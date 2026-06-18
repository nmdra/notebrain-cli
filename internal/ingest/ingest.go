package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/obsidian"
	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
)

type Pipeline struct {
	store    *store.Store
	embedder embedder.Embedder
	workers  int
}

func NewPipeline(s *store.Store, e embedder.Embedder, workers int) *Pipeline {
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
				if err := p.ingestFile(ctx, vaultPath, path); err != nil {
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

func (p *Pipeline) ingestFile(ctx context.Context, vaultPath string, filePath string) error {
	relPath, err := filepath.Rel(vaultPath, filePath)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	slug := parser.Slugify(relPath)
	title := parser.TitleFromPath(relPath)

	text := parser.StripFrontmatter(string(content))
	links := parser.ExtractLinks(text)
	tags := parser.ExtractTags(text, "") // frontmatter parsing skipped for now

	parsedChunks := parser.ChunkText(text, 300, 50)
	if len(parsedChunks) == 0 {
		parsedChunks = []parser.Chunk{{Index: 0, Text: " "}}
	}

	chunkRecords := make([]store.ChunkRecord, len(parsedChunks))
	for i, c := range parsedChunks {
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
			ID:         fmt.Sprintf("%s:%d", slug, i),
			NoteSlug:   slug,
			Title:      title,
			FilePath:   relPath,
			ChunkIndex: c.Index,
			Text:       c.Text,
			Tags:       tags,
			HasLinks:   len(links) > 0,
			ModifiedMs: modTime.UnixMilli(),
			Embedding:  emb,
		}
	}

	linkRecords := make([]obsidian.LinkRecord, len(links))
	for i, l := range links {
		linkRecords[i] = obsidian.LinkRecord{
			Path:        l.Target,
			DisplayText: l.DisplayText,
		}
	}

	if err := p.store.DeleteNoteChunks(ctx, slug); err != nil {
		return err
	}
	if err := p.store.UpsertChunks(ctx, chunkRecords); err != nil {
		return err
	}
	if err := p.store.UpsertLinks(ctx, slug, linkRecords); err != nil {
		return err
	}

	fmt.Printf("Ingested %s (%d chunks)\n", relPath, len(parsedChunks))
	return nil
}
