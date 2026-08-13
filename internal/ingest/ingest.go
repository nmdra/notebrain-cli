package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/nmdra/notebrain-cli/v2/internal/llmparse"
	"github.com/nmdra/notebrain-cli/v2/internal/parser"
	"github.com/nmdra/notebrain-cli/v2/internal/pdfextract"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

const (
	// chunkSchemaVersion is bumped whenever chunk content semantics change so
	// that already-ingested files are re-ingested even if their bytes are
	// unchanged (e.g. the chunk-overlap duplication fix, has_code metadata).
	// Version 4 covers the embedding-model hash salt: switching models must
	// invalidate stored hashes to avoid dimension mismatches.
	// Version 5 covers the attachment-embed marker rendering (chunk text
	// changes from raw filenames to [image]/[image: alt] markers).
	chunkSchemaVersion = 5
)

// Embedder abstracts vector embedding so the pipeline can be tested with mocks.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	// Model identifies the embedding model so content hashes can be salted.
	Model() string
}

// Store abstracts the vector-store methods the pipeline uses so it can be
// tested with fakes.
type Store interface {
	BatchIngest(ctx context.Context, data []store.BatchIngestData, staleSlugs []string) error
	GetNoteMetadata(ctx context.Context) (map[string]store.NoteMeta, error)
}

// FailedFile records a file that failed during ingestion along with the failure reason.
type FailedFile struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
}

// Pipeline orchestrates the ingestion of markdown files into the ChromaDB store.
type Pipeline struct {
	store            Store
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
func NewPipeline(s Store, e Embedder, workers int) *Pipeline {
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
func (p *Pipeline) Run(ctx context.Context, vaultPath string, glob string) error {
	skipPDF := false
	if p.EnablePDF {
		if p.LLMModel == "" {
			slog.Warn("PDF ingestion requested via --with-pdf, but no --llm-model was provided. Skipping PDF ingestion (previously ingested PDFs will be preserved). (hint: use --llm-model)")
			skipPDF = true
		} else {
			slog.Info("initializing PDF extractor backend", "pool_size", p.workers)
			pb, err := pdfextract.NewPDFiumBackend(p.workers)
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
			if skipPDF && meta.FileType == store.FileTypePDF {
				continue
			}
			staleSlugs = append(staleSlugs, slug)
		}
	}

	progressCh := make(chan ProgressUpdate, p.workers*2)
	errCh := make(chan error, totalFiles+1) // +1 for batch ingest error

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var uiWg sync.WaitGroup
	uiWg.Go(func() {
		RunProgress(totalFiles, progressCh)
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
			// Count every file (including failures) so progress can reach
			// totalFiles even when some files error out.
			defer func() {
				n := completed.Add(1)
				progressCh <- ProgressUpdate{
					Done:    int(n),
					Total:   totalFiles,
					Current: filepath.Base(f),
				}
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
		}(file)
	}

	// Wait for all workers to finish, then signal the UI to quit
	workerWg.Wait()
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

	// maxReportedWorkerErrors caps how many worker errors are joined into the
	// returned error, so a failing vault does not produce an unbounded error.
	const maxReportedWorkerErrors = 20

	var errs []error
	for e := range errCh {
		if len(errs) < maxReportedWorkerErrors {
			errs = append(errs, e)
		}
		slog.Error("ingestion worker error", "error", e)
	}

	failed := p.FailedFiles()
	if len(failed) > 0 {
		slog.Warn("ingestion finished with failed files", "failed_count", len(failed))
		for _, ff := range failed {
			slog.Warn("failed file summary", "file", ff.FilePath, "reason", ff.Reason)
		}
	}

	if len(errs) == 0 && ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.Join(errs...)
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
				matched, err := doublestar.Match(glob, rel)
				if err != nil {
					return fmt.Errorf("invalid glob %q: %w", glob, err)
				}
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

// fileHash returns a content hash that also covers the chunking parameters,
// the embedding model, and the current chunk schema version. Changing any of
// them invalidates previously stored hashes so unchanged files are still
// re-ingested (e.g. switching embedding models must not leave stale vectors).
func fileHash(content []byte, chunkSize, chunkOverlap int, embedModel string) string {
	h := sha256.New()
	h.Write(content)
	_, _ = fmt.Fprintf(h, "\x00schema=%d\x00size=%d\x00overlap=%d\x00model=%s\x00", chunkSchemaVersion, chunkSize, chunkOverlap, embedModel)
	return hex.EncodeToString(h.Sum(nil))
}

func (p *Pipeline) processFile(ctx context.Context, vaultPath string, filePath string, knownHashes map[string]string) (*store.BatchIngestData, error) {
	if strings.ToLower(filepath.Ext(filePath)) == ".pdf" {
		return p.processPdfFile(ctx, vaultPath, filePath, knownHashes)
	}

	relPath, slug, title, content, hash, changed, err := p.noteIdentity(vaultPath, filePath, knownHashes)
	if err != nil || !changed {
		return nil, err
	}

	// Frontmatter title overrides the filename-derived title for markdown
	// notes only; PDF titles come from the file name (see processPdfFile).
	return p.buildIngestData(ctx, relPath, slug, title, hash, store.FileTypeMD, string(content), true)
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
	relPath, slug, title, _, hash, changed, err := p.noteIdentity(vaultPath, filePath, knownHashes)
	if err != nil || !changed {
		return nil, err
	}

	pages, err := p.pdfBackend.ExtractText(ctx, filePath)
	if err != nil {
		slog.Warn("pdf extract text failed, skipping PDF", "file", relPath, "err", err)
		p.recordFailedFile(relPath, fmt.Sprintf("PDF text extraction failed: %v", err))
		return nil, nil
	}
	slog.Debug("sending PDF to LLM for markdown conversion", "file", relPath, "pages", len(pages))
	markdown, err := p.llmConverter.Convert(ctx, pages)
	if err != nil {
		slog.Warn("LLM conversion failed, skipping PDF", "file", relPath, "err", err)
		p.recordFailedFile(relPath, fmt.Sprintf("LLM conversion failed: %v", err))
		return nil, nil
	}

	// Empty LLM output is usually a transient conversion failure. Preserve the
	// previously indexed PDF instead of deleting it from the index; an empty
	// batch would remove its chunks permanently (see buildChunkRecords).
	return p.buildIngestData(ctx, relPath, slug, title, hash, store.FileTypePDF, markdown, false)
}

// noteIdentity reads, identifies, and hashes a note file. changed=false means
// the note is already indexed with identical content, so the caller skips it
// without reading further or re-embedding.
func (p *Pipeline) noteIdentity(vaultPath, filePath string, knownHashes map[string]string) (relPath, slug, title string, content []byte, hash string, changed bool, err error) {
	relPath, err = filepath.Rel(vaultPath, filePath)
	if err != nil {
		return "", "", "", nil, "", false, err
	}

	content, err = os.ReadFile(filePath)
	if err != nil {
		return "", "", "", nil, "", false, err
	}

	slug = parser.Slugify(relPath)
	hash = fileHash(content, p.ChunkSize, p.ChunkOverlap, p.embedder.Model())
	if knownHashes[slug] == hash {
		return "", "", "", nil, "", false, nil // Skip unchanged
	}

	return relPath, slug, parser.TitleFromPath(relPath), content, hash, true, nil
}

// buildIngestData parses markdown and assembles batch data for an identified
// note. Returns nil when no chunk survives filtering: the caller must skip the
// note entirely, since replacing it with an empty batch would delete its
// previously indexed chunks, and because the content hash is unchanged that
// deletion would be permanent (later ingests would skip the file as
// unmodified). When frontmatterTitle is true, a frontmatter "title" overrides
// the filename-derived title.
func (p *Pipeline) buildIngestData(ctx context.Context, relPath, slug, title, hash, fileType, markdown string, frontmatterTitle bool) (*store.BatchIngestData, error) {
	astRes := parser.Parse(markdown, slug, p.ChunkSize, p.ChunkOverlap, p.SkipAttachments)

	if frontmatterTitle {
		if ft, ok := astRes.Frontmatter["title"].(string); ok && ft != "" {
			title = ft
		}
	}

	records, err := p.buildChunkRecords(ctx, &astRes, slug, title, relPath, hash, fileType)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	return &store.BatchIngestData{
		NoteSlug:     slug,
		ChunkRecords: records,
		Links:        astRes.Links,
	}, nil
}

// buildChunkRecords filters parsed chunks below the minimum word threshold,
// embeds the survivors, and assembles store records tagged with fileType.
//
// When nothing survives (note had no chunks, or every chunk was filtered),
// it returns an empty slice: the caller must skip the note entirely, since
// replacing it with an empty batch would delete its previously indexed
// chunks, and because the content hash is unchanged that deletion would be
// permanent (later ingests would skip the file as unmodified).
func (p *Pipeline) buildChunkRecords(ctx context.Context, astRes *parser.Result, slug, title, relPath, hash, fileType string) ([]store.ChunkRecord, error) {
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
			HeadingPath:  c.HeadingPath,
			HeadingLevel: c.Level,
			HasTask:      c.HasTask,
			HasCode:      c.HasCode,
			FileType:     fileType,
			ContentHash:  hash,
			Embedding:    emb,
		}
	}

	return chunkRecords, nil
}
