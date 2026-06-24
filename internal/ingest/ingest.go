package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/nmdra/notebrain-cli/internal/tui"
)

// Embedder abstracts vector embedding so the pipeline can be tested with mocks.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Pipeline orchestrates the ingestion of markdown files into the ChromaDB store.
type Pipeline struct {
	store         *store.Store
	embedder      Embedder
	workers       int
	MinChunkWords int
}

// NewPipeline creates an ingestion pipeline with the given number of concurrent workers.
func NewPipeline(s *store.Store, e Embedder, workers int) *Pipeline {
	if workers <= 0 {
		workers = 1
	}
	return &Pipeline{
		store:    s,
		embedder: e,
		workers:  workers,
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
	_, _ = fmt.Fprintf(stdout, "Found %d markdown files to ingest\n", totalFiles)

	if totalFiles == 0 {
		return nil
	}

	hashes, _ := p.store.GetNoteHashes(ctx)
	if hashes == nil {
		hashes = make(map[string]string)
	}

	// Identify notes that are in the database but no longer exist on disk
	validSlugs := make(map[string]bool)
	for _, file := range files {
		rel, err := filepath.Rel(vaultPath, file)
		if err == nil {
			validSlugs[parser.Slugify(rel)] = true
		}
	}

	var staleSlugs []string
	for slug := range hashes {
		if !validSlugs[slug] {
			staleSlugs = append(staleSlugs, slug)
		}
	}

	// +1 for the TUI goroutine's potential error
	progressCh := make(chan tui.ProgressUpdate, p.workers*2)
	errCh := make(chan error, totalFiles+2) // +2 for TUI and batch ingest error

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var done int32

	// Launch UI program in background
	pUI := tea.NewProgram(
		tui.NewProgressModel(totalFiles, progressCh),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
	)

	var uiWg sync.WaitGroup
	uiWg.Add(1)
	go func() {
		defer uiWg.Done()
		if _, uiErr := pUI.Run(); uiErr != nil {
			errCh <- fmt.Errorf("progress UI error: %w", uiErr)
		}
		if atomic.LoadInt32(&done) == 0 {
			cancel() // Cancel workers if UI exits early (e.g. ctrl+c)
		}
	}()

	// Atomic counter for monotonically increasing progress
	var completed int64

	var workerWg sync.WaitGroup
	sem := make(chan struct{}, p.workers)

	var mu sync.Mutex
	var ingestResults []store.BatchIngestData

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

			n := atomic.AddInt64(&completed, 1)
			progressCh <- tui.ProgressUpdate{
				Done:    int(n),
				Total:   totalFiles,
				Current: filepath.Base(f),
			}
		}(file)
	}

	// Wait for all workers to finish, then signal the UI to quit
	workerWg.Wait()
	atomic.StoreInt32(&done, 1)
	close(progressCh)
	uiWg.Wait()

	// Perform batch database updates
	if len(ingestResults) > 0 || len(staleSlugs) > 0 {
		_, _ = fmt.Fprintln(stdout, "\nSyncing database: applying batch updates...")
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
		_, _ = fmt.Fprintf(stdout, "Error: %v\n", e)
	}

	if firstErr == nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}

// collectFiles walks the vault directory and returns all .md files matching glob.
func (p *Pipeline) collectFiles(vaultPath, glob string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".md" {
			if glob != "" {
				rel, _ := filepath.Rel(vaultPath, path)
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
	astRes := parser.ParseAST(string(content), slug, 1000)

	if ft, ok := astRes.Frontmatter["title"].(string); ok && ft != "" {
		title = ft
	}

	if len(astRes.Chunks) == 0 {
		astRes.Chunks = []parser.ASTChunk{{NoteSlug: slug, Index: 0, Text: " "}}
	}

	// Stat the file once, outside the chunk loop
	info, _ := os.Stat(filePath)
	modTime := time.Now()
	if info != nil {
		modTime = info.ModTime()
	}

	// Filter chunks by minimum word count
	var validChunks []parser.ASTChunk
	for _, c := range astRes.Chunks {
		if c.WordCount >= p.MinChunkWords {
			validChunks = append(validChunks, c)
		}
	}

	chunkRecords := make([]store.ChunkRecord, len(validChunks))
	for i, c := range validChunks {
		emb, err := p.embedder.Embed(ctx, c.Text)
		if err != nil {
			return nil, err
		}

		chunkRecords[i] = store.ChunkRecord{
			ID:           fmt.Sprintf("%s:%d", slug, i),
			NoteSlug:     slug,
			Title:        title,
			FilePath:     relPath,
			ChunkIndex:   i,
			Text:         c.Text,
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
