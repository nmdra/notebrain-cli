package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type Pipeline struct {
	store    *store.Store
	embedder Embedder
	workers  int
}

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

func (p *Pipeline) Run(ctx context.Context, vaultPath string, glob string) error {
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
		return fmt.Errorf("walk vault: %w", err)
	}

	fmt.Printf("Found %d markdown files to ingest\n", len(files))

	hashes, _ := p.store.GetNoteHashes(ctx)
	if hashes == nil {
		hashes = make(map[string]string)
	}

	var wg sync.WaitGroup
	fileCh := make(chan string, len(files))
	for _, f := range files {
		fileCh <- f
	}
	close(fileCh)

	errCh := make(chan error, len(files))

	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileCh {
				if err := p.ingestFile(ctx, vaultPath, path, hashes); err != nil {
					errCh <- fmt.Errorf("file %s: %w", path, err)
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	var firstErr error
	for e := range errCh {
		if firstErr == nil {
			firstErr = e
		}
		fmt.Printf("Error: %v\n", e)
	}

	return firstErr
}

func (p *Pipeline) ingestFile(ctx context.Context, vaultPath string, filePath string, knownHashes map[string]string) error {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	slug := parser.Slugify(relPath)

	// Compute hash and check if unchanged
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	if knownHashes[slug] == hash {
		fmt.Printf("Skipped %s (unchanged)\n", relPath)
		return nil
	}

	title := parser.TitleFromPath(relPath)

	// Use goldmark to parse AST, extract frontmatter, tags, links, and text chunks
	astRes := parser.ParseAST(string(content), slug, 300)

	// If frontmatter provides a title, prefer it
	if ft, ok := astRes.Frontmatter["title"].(string); ok && ft != "" {
		title = ft
	}

	if len(astRes.Chunks) == 0 {
		// create a dummy chunk for empty files
		astRes.Chunks = []parser.ASTChunk{{NoteSlug: slug, Index: 0, Text: " "}}
	}

	chunkRecords := make([]store.ChunkRecord, len(astRes.Chunks))
	for i, c := range astRes.Chunks {
		emb, err := p.embedder.Embed(ctx, c.Text)
		if err != nil {
			return err
		}
		info, _ := os.Stat(filePath)
		modTime := time.Now()
		if info != nil {
			modTime = info.ModTime()
		}

		chunkRecords[i] = store.ChunkRecord{
			ID:           fmt.Sprintf("%s:%d", slug, i),
			NoteSlug:     slug,
			Title:        title,
			FilePath:     relPath,
			ChunkIndex:   c.Index,
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

	if err := p.store.DeleteNoteChunks(ctx, slug); err != nil {
		return err
	}
	if err := p.store.UpsertChunks(ctx, chunkRecords); err != nil {
		return err
	}
	if err := p.store.UpsertLinks(ctx, slug, astRes.Links); err != nil {
		return err
	}

	fmt.Printf("Ingested %s (%d chunks)\n", relPath, len(astRes.Chunks))
	return nil
}
