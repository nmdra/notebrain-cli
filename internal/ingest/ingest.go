package ingest

import (
	"context"
	"crypto/sha256"
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

	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
)

// Embedder abstracts vector embedding so the pipeline can be tested with mocks.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Pipeline orchestrates the ingestion of markdown files into the ChromaDB store.
type Pipeline struct {
	store           *store.Store
	embedder        Embedder
	workers         int
	MinChunkWords   int // minimum word count to keep a chunk (filters junk)
	ChunkSize       int // maximum runes per chunk fed to Parse
	ChunkOverlap    int // overlap runes between sub-chunks when a section is split
	MaxEmbedTokens  int // max tokens for embed text (model sequence length)
	RespectExclude  bool
	SkipAttachments bool
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
		ChunkSize:       800,
		ChunkOverlap:    100,
		MaxEmbedTokens:  256,
		RespectExclude:  true,
		SkipAttachments: true,
	}
}

// Run walks the vault directory, finds markdown files matching glob, and ingests
// them into the store with a TUI progress bar rendered to stdout.
func (p *Pipeline) Run(ctx context.Context, vaultPath string, glob string, stdin io.Reader, stdout io.Writer) error {
	files, err := p.collectFiles(vaultPath, glob)
	if err != nil {
		return err
	}

	totalFiles := len(files)
	slog.Info("scanning vault", "files_found", totalFiles, "vault_path", vaultPath)

	if totalFiles == 0 {
		slog.Info("no matching markdown files found", "vault_path", vaultPath, "glob", glob)
		return nil
	}

	hashes, _ := p.store.GetNoteHashes(ctx)
	if hashes == nil {
		hashes = make(map[string]string)
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
	for slug := range hashes {
		if _, ok := validSlugs[slug]; !ok {
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

			res, err := p.processFile(ctx, vaultPath, f, hashes)
			if err != nil {
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

	if firstErr == nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}

// collectFiles walks the vault directory and returns all .md files matching glob.
func (p *Pipeline) collectFiles(vaultPath, glob string) ([]string, error) {
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
		if filepath.Ext(path) == ".md" {
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

func (p *Pipeline) processFile(ctx context.Context, vaultPath string, filePath string, knownHashes map[string]string) (*store.BatchIngestData, error) {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	slug := parser.Slugify(relPath)

	hash := fmt.Sprintf("%x", sha256.Sum256(content))
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

	chunkRecords := make([]store.ChunkRecord, len(validChunks))
	for i, c := range validChunks {
		// Preamble fix: the top-of-note section before the first heading has no
		// HeadingPath. Use the note title so preamble chunks are semantically grounded.
		headingPath := c.HeadingPath
		if headingPath == "" {
			headingPath = title
		}

		embedContent := c.Text
		if isCodeOnlyChunk(c.Text) && c.RichText != "" {
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
			CodeBlocks:   c.CodeBlocks,
			HasTable:     c.HasTable,
			HasTask:      c.HasTask,
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
	if title != "" && headingPath != "" && headingPath != title {
		breadcrumb = title + " > " + headingPath
	} else if title != "" {
		breadcrumb = title
	} else if headingPath != "" {
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

		if bcTokens+tagTokens <= prefixBudget {
			if breadcrumb != "" {
				sb.WriteString(breadcrumb)
				sb.WriteByte('\n')
			}
			if tagLine != "" {
				sb.WriteString(tagLine)
				sb.WriteByte('\n')
			}
		} else if bcTokens <= prefixBudget {
			if breadcrumb != "" {
				sb.WriteString(breadcrumb)
				sb.WriteByte('\n')
			}
		} else if estimateTokens(title) <= prefixBudget && title != "" {
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
