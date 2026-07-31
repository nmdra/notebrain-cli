package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/nmdra/notebrain-cli/v2/internal/llmparse"
	"github.com/nmdra/notebrain-cli/v2/internal/parser"
	"github.com/nmdra/notebrain-cli/v2/internal/pdfextract"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

const (
	fileTypeMD  = "md"
	fileTypePDF = "pdf"

	// chunkSchemaVersion is bumped whenever chunk content semantics change so
	// that already-ingested files are re-ingested even if their bytes are
	// unchanged (e.g. the chunk-overlap duplication fix, has_code metadata).
	chunkSchemaVersion = 3
)

// Embedder abstracts vector embedding so the pipeline can be tested with mocks.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// FailedFile records a file that failed during ingestion along with the failure reason.
type FailedFile struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
}

// Pipeline orchestrates the ingestion of markdown files into the ChromaDB store.
type Pipeline struct {
	store            *store.Store
	embedder         Embedder
	workers          int
	MinChunkWords    int // minimum word count to keep a chunk (filters junk)
	ChunkSize        int // maximum runes per chunk fed to Parse
	ChunkOverlap     int // overlap runes between sub-chunks when a section is split
	MaxEmbedTokens   int // max tokens for embed text (model sequence length)
	RespectExclude   bool
	SkipAttachments  bool
	EnablePDF        bool
	LLMModel         string
	LLMContextWindow int
	llmConverter     llmparse.Converter
	pdfBackend       pdfextract.PDFBackend
	ocrBackend       pdfextract.OCRBackend
	failedFilesMu    sync.Mutex
	failedFiles      []FailedFile
}

func (p *Pipeline) recordFailedFile(filePath, reason string) {
	p.failedFilesMu.Lock()
	defer p.failedFilesMu.Unlock()
	p.failedFiles = append(p.failedFiles, FailedFile{
		FilePath: filePath,
		Reason:   reason,
	})
}

// FailedFiles returns a copy of all file ingestion failures recorded during the run.
func (p *Pipeline) FailedFiles() []FailedFile {
	p.failedFilesMu.Lock()
	defer p.failedFilesMu.Unlock()
	copied := make([]FailedFile, len(p.failedFiles))
	copy(copied, p.failedFiles)
	return copied
}

// NewPipeline creates an ingestion pipeline with the given number of concurrent workers.
// Set ChunkSize, ChunkOverlap, and MinChunkWords on the returned Pipeline before calling Run.
func NewPipeline(s *store.Store, e Embedder, workers int) *Pipeline {
	if workers <= 0 {
		workers = 1
	}
	return &Pipeline{
		store:    s,
		embedder: e,
		workers:  workers,
		// Defaults matching config.Default() — callers should override from config.
		ChunkSize:        800,
		ChunkOverlap:     100,
		MinChunkWords:    10,
		MaxEmbedTokens:   256,
		RespectExclude:   false,
		SkipAttachments:  true,
		LLMContextWindow: 128000,
	}
}

// Run walks the vault directory, finds markdown files matching glob, and ingests
// them into the store with structured logging for progress.
func (p *Pipeline) Run(ctx context.Context, vaultPath string, glob string, _ io.Reader, _ io.Writer) error {
	skipPDF := false
	if p.EnablePDF {
		if p.LLMModel == "" {
			slog.Warn("PDF ingestion requested via --enable-pdf, but no --llm-model was provided. Skipping PDF ingestion (previously ingested PDFs will be preserved). (hint: use --llm-model)")
			skipPDF = true
		} else {
			slog.Info("initializing PDF extractor backend")
			pb, err := pdfextract.NewPDFiumBackend()
			if err != nil {
				return fmt.Errorf("failed to initialize PDF backend: %w", err)
			}
			p.pdfBackend = pb
			defer pb.Close()

			converter, err := llmparse.New(p.LLMModel, p.LLMContextWindow)
			if err != nil {
				if errors.Is(err, llmparse.ErrNoAPIKey) {
					slog.Warn("PDF ingestion enabled, but no valid API key found in environment. Skipping PDF ingestion (previously ingested PDFs will be preserved).", "err", err)
					skipPDF = true
				} else {
					return fmt.Errorf("LLM PDF parser: %w", err)
				}
			}

			if !skipPDF {
				p.llmConverter = converter
				slog.Info("LLM PDF parser enabled", "model", p.LLMModel, "backend", converter.Name())

				// Auto-detect OCR support when PDF ingestion is enabled
				ob := pdfextract.NewTesseractBackend("tesseract", "eng")
				if ob.Available() {
					if err := ob.ValidateLang(ctx); err != nil {
						slog.Warn("tesseract found but language validation failed, skipping OCR", "err", err)
					} else {
						p.ocrBackend = ob
						slog.Info("OCR backend auto-detected (tesseract)")
					}
				} else {
					slog.Debug("tesseract not found in PATH, OCR unavailable for scanned PDFs")
				}
			}
		}
	}

	files, err := p.collectFiles(vaultPath, glob, skipPDF)
	if err != nil {
		return err
	}

	totalFiles := len(files)
	slog.Info("scanning vault", "files_found", totalFiles, "vault_path", vaultPath)

	if totalFiles == 0 {
		slog.Info("no matching markdown files found", "vault_path", vaultPath, "glob", glob)
		return nil
	}

	hashes, err := p.store.GetNoteMetadata(ctx)
	if err != nil {
		slog.Warn("could not fetch existing note hashes, proceeding with full check", "err", err)
	}
	if hashes == nil {
		hashes = make(map[string]store.NoteMeta)
	}

	simpleHashes := make(map[string]string, len(hashes))
	for k, v := range hashes {
		simpleHashes[k] = v.Hash
	}

	// Identify notes that are in the database but no longer exist on disk
	validSlugs := make(map[string]struct{}, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(vaultPath, file)
		if err == nil {
			validSlugs[parser.Slugify(rel)] = struct{}{}
		}
	}

	staleSlugs := make([]string, 0, len(hashes))
	for slug, meta := range hashes {
		if _, ok := validSlugs[slug]; !ok {
			if skipPDF && meta.FileType == fileTypePDF {
				continue
			}
			staleSlugs = append(staleSlugs, slug)
		}
	}

	progressCh := make(chan ProgressUpdate, p.workers*2)
	errCh := make(chan error, totalFiles+1) // +1 for batch ingest error

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var done atomic.Int32

	var uiWg sync.WaitGroup
	uiWg.Go(func() {
		RunProgress(totalFiles, progressCh)
		if done.Load() == 0 {
			cancel() // Cancel workers if progress loop exits early
		}
	})

	// Atomic counter for monotonically increasing progress
	var completed atomic.Int64

	var workerWg sync.WaitGroup
	sem := make(chan struct{}, p.workers)

	var mu sync.Mutex
	ingestResults := make([]store.BatchIngestData, 0, len(files))

fileLoop:
	for _, file := range files {
		// Check for context cancellation before spawning new work
		select {
		case <-ctx.Done():
			break fileLoop
		case sem <- struct{}{}:
		}

		if ctx.Err() != nil {
			<-sem
			break fileLoop
		}

		workerWg.Add(1)
		go func(f string) {
			defer func() {
				<-sem
				workerWg.Done()
			}()

			res, err := p.processFile(ctx, vaultPath, f, simpleHashes)
			if err != nil {
				rel, _ := filepath.Rel(vaultPath, f)
				p.recordFailedFile(rel, err.Error())
				errCh <- fmt.Errorf("file %s: %w", f, err)
				return
			}

			if res != nil {
				mu.Lock()
				ingestResults = append(ingestResults, *res)
				mu.Unlock()
			}

			n := completed.Add(1)
			progressCh <- ProgressUpdate{
				Done:    int(n),
				Total:   totalFiles,
				Current: filepath.Base(f),
			}
		}(file)
	}

	// Wait for all workers to finish, then signal the UI to quit
	workerWg.Wait()
	done.Store(1)
	close(progressCh)
	uiWg.Wait()

	// Perform batch database updates
	if len(ingestResults) > 0 || len(staleSlugs) > 0 {
		slog.Info("syncing database: applying batch updates", "notes_updated", len(ingestResults), "stale_removed", len(staleSlugs))
		if err := p.store.BatchIngest(ctx, ingestResults, staleSlugs); err != nil {
			errCh <- fmt.Errorf("batch ingest: %w", err)
		}
	}

	close(errCh)

	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
		slog.Error("ingestion worker error", "err", e)
	}

	failed := p.FailedFiles()
	if len(failed) > 0 {
		slog.Warn("ingestion finished with failed files", "failed_count", len(failed))
		for _, ff := range failed {
			slog.Warn("failed file summary", "file", ff.FilePath, "reason", ff.Reason)
		}
	}

	if firstErr == nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}

// collectFiles walks the vault directory and returns all .md files matching glob.
func (p *Pipeline) collectFiles(vaultPath, glob string, skipPDF bool) ([]string, error) {
	var excluded []string
	if p.RespectExclude {
		excluded = LoadExcludedPaths(vaultPath)
	}
	var files []string
	err := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(vaultPath, path)
		if err != nil {
			return err
		}

		if rel != "." {
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if IsExcluded(rel, excluded) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || (p.EnablePDF && !skipPDF && ext == ".pdf") {
			if glob != "" {
				matched, _ := filepath.Match(glob, rel)
				if !matched {
					return nil
				}
			}
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk vault: %w", err)
	}
	return files, nil
}

// fileHash returns a content hash that also covers the chunking parameters and
// the current chunk schema version. Changing any of them invalidates previously
// stored hashes so unchanged files are still re-ingested.
func fileHash(content []byte, chunkSize, chunkOverlap int) string {
	h := sha256.New()
	h.Write(content)
	_, _ = fmt.Fprintf(h, "\x00schema=%d\x00size=%d\x00overlap=%d\x00", chunkSchemaVersion, chunkSize, chunkOverlap)
	return hex.EncodeToString(h.Sum(nil))
}

func (p *Pipeline) processFile(ctx context.Context, vaultPath string, filePath string, knownHashes map[string]string) (*store.BatchIngestData, error) {
	if strings.ToLower(filepath.Ext(filePath)) == ".pdf" {
		return p.processPdfFile(ctx, vaultPath, filePath, knownHashes)
	}

	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	slug := parser.Slugify(relPath)

	hash := fileHash(content, p.ChunkSize, p.ChunkOverlap)
	if knownHashes[slug] == hash {
		return nil, nil
	}

	title := parser.TitleFromPath(relPath)
	astRes := parser.Parse(string(content), slug, p.ChunkSize, p.ChunkOverlap, p.SkipAttachments)

	if ft, ok := astRes.Frontmatter["title"].(string); ok && ft != "" {
		title = ft
	}

	if len(astRes.Chunks) == 0 {
		astRes.Chunks = []parser.Chunk{{NoteSlug: slug, Index: 0, Text: " "}}
	}

	// Stat the file once, outside the chunk loop
	info, _ := os.Stat(filePath)
	modTime := time.Now()
	if info != nil {
		modTime = info.ModTime()
	}

	// Filter chunks: discard those below the minimum word threshold.
	// For code-only chunks (where Text is only placeholders), check word count
	// against RichText so code notes are preserved.
	validChunks := make([]parser.Chunk, 0, len(astRes.Chunks))
	for _, c := range astRes.Chunks {
		storedText := c.RichText
		if storedText == "" {
			storedText = c.Text
		}
		if len(strings.Fields(storedText)) < p.MinChunkWords {
			continue
		}
		// If c.RichText is empty and c.Text is just code placeholders with no prose, skip.
		if c.RichText == "" && isCodeOnlyChunk(c.Text) {
			continue
		}
		validChunks = append(validChunks, c)
	}

	// If every chunk was filtered out, skip the note entirely. Replacing it
	// with an empty batch would delete its previously indexed chunks, and
	// since the content hash is unchanged the deletion would be permanent
	// (later ingests would skip the file as unmodified).
	if len(validChunks) == 0 {
		return nil, nil
	}

	chunkRecords := make([]store.ChunkRecord, len(validChunks))
	for i, c := range validChunks {
		// Preamble fix: the top-of-note section before the first heading has no
		// HeadingPath. Use the note title so preamble chunks are semantically grounded.
		headingPath := c.HeadingPath
		if headingPath == "" {
			headingPath = title
		}

		// Embed from the overlap-full variant so chunk-boundary context
		// survives embedding; store the overlap-free display text.
		embedContent := c.EmbedText
		if isCodeOnlyChunk(c.EmbedText) && c.RichText != "" {
			embedContent = c.RichText
		}

		storedText := c.RichText
		if storedText == "" {
			storedText = c.Text
		}

		// Contextual augmentation: prepend title + heading path + tags before
		// embedding. The storedText is stored in ChromaDB for display/retrieval.
		embedText := buildEmbedText(title, headingPath, astRes.Tags, embedContent, p.MaxEmbedTokens)
		emb, err := p.embedder.Embed(ctx, embedText)
		if err != nil {
			return nil, err
		}

		chunkRecords[i] = store.ChunkRecord{
			ID:           fmt.Sprintf("%s:%d", slug, i),
			NoteSlug:     slug,
			Title:        title,
			FilePath:     relPath,
			ChunkIndex:   i,
			Text:         storedText,
			Tags:         astRes.Tags,
			HasLinks:     len(astRes.Links) > 0,
			HeadingPath:  c.HeadingPath,
			HeadingLevel: c.Level,
			HasTask:      c.HasTask,
			HasCode:      c.HasCode,
			FileType:     fileTypeMD,
			ModifiedMs:   modTime.UnixMilli(),
			ContentHash:  hash,
			Embedding:    emb,
		}
	}

	return &store.BatchIngestData{
		NoteSlug:     slug,
		ChunkRecords: chunkRecords,
		Links:        astRes.Links,
	}, nil
}

// estimateTokens returns a conservative rough token count for English/mixed text.
// Based on empirical ratio: ~4 runes per token for MiniLM tokenizer.
func estimateTokens(text string) int {
	return (utf8.RuneCountInString(text) + 3) / 4
}

// buildEmbedText constructs contextual embedding text with a token budget guard.
// Priority: chunk content > title > heading path > tags.
// If the full text would exceed maxTokens, the prefix is progressively trimmed.
func buildEmbedText(title, headingPath string, tags []string, chunkText string, maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = 256
	}

	bodyTokens := estimateTokens(chunkText)

	breadcrumb := ""
	switch {
	case title != "" && headingPath != "" && headingPath != title:
		breadcrumb = title + " > " + headingPath
	case title != "":
		breadcrumb = title
	case headingPath != "":
		breadcrumb = headingPath
	}

	tagLine := ""
	if len(tags) > 0 {
		tagLine = "[tags: " + strings.Join(tags, ", ") + "]"
	}

	prefixBudget := maxTokens - bodyTokens - 2

	var sb strings.Builder
	sb.Grow(len(title) + len(headingPath) + len(chunkText) + 64)

	if prefixBudget > 0 {
		bcTokens := estimateTokens(breadcrumb)
		tagTokens := estimateTokens(tagLine)

		switch {
		case bcTokens+tagTokens <= prefixBudget:
			if breadcrumb != "" {
				sb.WriteString(breadcrumb)
				sb.WriteByte('\n')
			}
			if tagLine != "" {
				sb.WriteString(tagLine)
				sb.WriteByte('\n')
			}
		case bcTokens <= prefixBudget:
			if breadcrumb != "" {
				sb.WriteString(breadcrumb)
				sb.WriteByte('\n')
			}
		case estimateTokens(title) <= prefixBudget && title != "":
			sb.WriteString(title)
			sb.WriteByte('\n')
		}
	}

	if sb.Len() > 0 {
		sb.WriteByte('\n')
	}
	sb.WriteString(chunkText)
	return sb.String()
}

// codeOnlyPattern matches chunk text that consists entirely of [code:X] or [code]
// placeholder tokens emitted by the parser for fenced code blocks.
// Such chunks carry no prose signal and produce noisy embeddings.
var codeOnlyPattern = regexp.MustCompile(`^(\[code(:[^\]]+)?\]\s*)+$`)

// isCodeOnlyChunk returns true when the entire chunk text is one or more
// [code:X] placeholder tokens with no prose content.
func isCodeOnlyChunk(text string) bool {
	return codeOnlyPattern.MatchString(strings.TrimSpace(text))
}

func (p *Pipeline) processPdfFile(ctx context.Context, vaultPath string, filePath string, knownHashes map[string]string) (*store.BatchIngestData, error) {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	slug := parser.Slugify(relPath)
	hash := fileHash(content, p.ChunkSize, p.ChunkOverlap)
	if knownHashes[slug] == hash {
		return nil, nil // Skip unchanged
	}

	title := parser.TitleFromPath(relPath)

	info, _ := os.Stat(filePath)
	modTime := time.Now()
	if info != nil {
		modTime = info.ModTime()
	}

	var markdown string

	pages, err := p.pdfBackend.ExtractText(ctx, filePath)
	if err != nil {
		slog.Warn("pdf extract text failed, skipping PDF", "file", relPath, "err", err)
		p.recordFailedFile(relPath, fmt.Sprintf("PDF text extraction failed: %v", err))
		return nil, nil
	}
	slog.Debug("sending PDF to LLM for markdown conversion", "file", relPath, "pages", len(pages))
	markdown, err = p.llmConverter.Convert(ctx, pages)
	if err != nil {
		slog.Warn("LLM conversion failed, skipping PDF", "file", relPath, "err", err)
		p.recordFailedFile(relPath, fmt.Sprintf("LLM conversion failed: %v", err))
		return nil, nil
	}

	astRes := parser.Parse(markdown, slug, p.ChunkSize, p.ChunkOverlap, p.SkipAttachments)

	// Empty LLM output is usually a transient conversion failure. Preserve the
	// previously indexed PDF instead of deleting it from the index.
	if len(astRes.Chunks) == 0 {
		return nil, nil
	}

	validChunks := make([]parser.Chunk, 0, len(astRes.Chunks))
	for _, c := range astRes.Chunks {
		storedText := c.RichText
		if storedText == "" {
			storedText = c.Text
		}
		if len(strings.Fields(storedText)) < p.MinChunkWords {
			continue
		}
		if c.RichText == "" && isCodeOnlyChunk(c.Text) {
			continue
		}
		validChunks = append(validChunks, c)
	}

	// Same preservation rule as markdown notes: an empty batch would delete
	// the PDF's previously indexed chunks with no way to restore them.
	if len(validChunks) == 0 {
		return nil, nil
	}

	chunkRecords := make([]store.ChunkRecord, len(validChunks))
	for i, c := range validChunks {
		headingPath := c.HeadingPath
		if headingPath == "" {
			headingPath = title
		}

		embedContent := c.EmbedText
		if isCodeOnlyChunk(c.EmbedText) && c.RichText != "" {
			embedContent = c.RichText
		}

		storedText := c.RichText
		if storedText == "" {
			storedText = c.Text
		}

		embedText := buildEmbedText(title, headingPath, astRes.Tags, embedContent, p.MaxEmbedTokens)
		emb, err := p.embedder.Embed(ctx, embedText)
		if err != nil {
			return nil, err
		}

		chunkRecords[i] = store.ChunkRecord{
			ID:           fmt.Sprintf("%s:%d", slug, i),
			NoteSlug:     slug,
			Title:        title,
			FilePath:     relPath,
			ChunkIndex:   i,
			Text:         storedText,
			Tags:         astRes.Tags,
			HasLinks:     len(astRes.Links) > 0,
			HeadingPath:  c.HeadingPath,
			HeadingLevel: c.Level,
			HasTask:      c.HasTask,
			HasCode:      c.HasCode,
			FileType:     fileTypePDF,
			ModifiedMs:   modTime.UnixMilli(),
			ContentHash:  hash,
			Embedding:    emb,
		}
	}

	return &store.BatchIngestData{
		NoteSlug:     slug,
		ChunkRecords: chunkRecords,
		Links:        astRes.Links,
	}, nil
}
