package store

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/nmdra/notebrain-cli/internal/parser"
)

type ChunkRecord struct {
	ID           string // "<slug>:<index>"
	NoteSlug     string
	Title        string
	FilePath     string
	ChunkIndex   int
	Text         string
	Tags         []string
	HasLinks     bool
	HeadingPath  string
	HeadingLevel int
	CodeBlocks   int
	HasTable     bool
	HasTask      bool
	ModifiedMs   int64
	ContentHash  string
	Embedding    []float32
}

// IngestNote atomically replaces all chunks and links for a single note.
// It holds the store mutex for the entire Delete→UpsertChunks→UpsertLinks
// sequence to prevent concurrent hnswlib modifications that corrupt the
// HNSW graph (assertion: inbound_connections_num[i] > 0).
func (s *Store) IngestNote(ctx context.Context, noteSlug string, chunks []ChunkRecord, links []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.deleteNoteChunks(ctx, noteSlug); err != nil {
		return err
	}
	if err := s.upsertChunks(ctx, chunks); err != nil {
		return err
	}
	return s.upsertLinks(ctx, noteSlug, links)
}

// UpsertChunks stores a batch of chunks (upsert = insert or replace by ID).
// Call DeleteNoteChunks first to cleanly re-ingest a note.
func (s *Store) UpsertChunks(ctx context.Context, chunks []ChunkRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertChunks(ctx, chunks)
}

// upsertChunks is the unlocked implementation. Caller must hold s.mu.
func (s *Store) upsertChunks(ctx context.Context, chunks []ChunkRecord) error {
	if len(chunks) == 0 {
		return nil
	}

	ids := make([]chroma.DocumentID, len(chunks))
	texts := make([]string, len(chunks))
	embs := make([]embeddings.Embedding, len(chunks))
	metas := make([]chroma.DocumentMetadata, len(chunks))

	for i, c := range chunks {
		ids[i] = chroma.DocumentID(c.ID)
		texts[i] = c.Text
		embs[i] = embeddings.NewEmbeddingFromFloat32(c.Embedding)
		metaMap := buildChunkMeta(c)
		metas[i], _ = chroma.NewDocumentMetadataFromMap(metaMap)
	}

	return s.chunks.Upsert(ctx,
		chroma.WithIDs(ids...),
		chroma.WithTexts(texts...),
		chroma.WithEmbeddings(embs...),
		chroma.WithMetadatas(metas...),
	)
}

// DeleteNoteChunks removes all chunks belonging to a note (before re-ingest).
func (s *Store) DeleteNoteChunks(ctx context.Context, noteSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteNoteChunks(ctx, noteSlug)
}

// deleteNoteChunks is the unlocked implementation. Caller must hold s.mu.
func (s *Store) deleteNoteChunks(ctx context.Context, noteSlug string) error {
	return s.chunks.Delete(ctx,
		chroma.WithWhere(chroma.EqString("note_slug", noteSlug)),
	)
}

// UpsertLinks replaces all outgoing links for a note.
func (s *Store) UpsertLinks(ctx context.Context, noteSlug string, links []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertLinks(ctx, noteSlug, links)
}

// upsertLinks is the unlocked implementation. Caller must hold s.mu.
func (s *Store) upsertLinks(ctx context.Context, noteSlug string, links []string) error {
	// Delete old outgoing links for this note
	if err := s.links.Delete(ctx,
		chroma.WithWhere(chroma.EqString("source_slug", noteSlug)),
	); err != nil {
		return fmt.Errorf("delete old links for %s: %w", noteSlug, err)
	}

	var uniqueLinks []string
	seenSlugs := make(map[string]bool)
	for _, l := range links {
		targetSlug := parser.Slugify(l)
		if targetSlug == "" || seenSlugs[targetSlug] {
			continue
		}
		seenSlugs[targetSlug] = true
		uniqueLinks = append(uniqueLinks, l)
	}

	if len(uniqueLinks) == 0 {
		return nil
	}

	ids := make([]chroma.DocumentID, len(uniqueLinks))
	texts := make([]string, len(uniqueLinks)) // placeholder text — Chroma requires non-empty
	metas := make([]chroma.DocumentMetadata, len(uniqueLinks))

	for i, l := range uniqueLinks {
		targetSlug := parser.Slugify(l) // derive slug from resolved path
		ids[i] = chroma.DocumentID(noteSlug + "→" + targetSlug)
		texts[i] = l
		if texts[i] == "" {
			texts[i] = "-"
		}
		metaMap := map[string]interface{}{
			"source_slug":  noteSlug,
			"target_slug":  targetSlug,
			"target_path":  l,
			"display_text": l,
		}
		metas[i], _ = chroma.NewDocumentMetadataFromMap(metaMap)
	}

	// sb_links has no embedding function — pass zero-length embeddings or
	// create the collection with no embedding function.
	// Simplest: store links with a dummy 1-dim embedding.
	embs := make([]embeddings.Embedding, len(uniqueLinks))
	// Add dummy 1-dimensional embeddings to bypass Chroma dimension checks (required)
	// We use random values to prevent HNSW pathologically failing on identical vectors
	for i := range uniqueLinks {
		embs[i] = embeddings.NewEmbeddingFromFloat32([]float32{rand.Float32()})
	}

	return s.links.Upsert(ctx,
		chroma.WithIDs(ids...),
		chroma.WithTexts(texts...),
		chroma.WithEmbeddings(embs...),
		chroma.WithMetadatas(metas...),
	)
}

// ─── Metadata helpers ────────────────────────────────────────────

func buildChunkMeta(c ChunkRecord) map[string]interface{} {
	meta := map[string]interface{}{
		"note_slug":     c.NoteSlug,
		"title":         c.Title,
		"file_path":     c.FilePath,
		"chunk_index":   c.ChunkIndex,
		"word_count":    len(strings.Fields(c.Text)),
		"has_links":     c.HasLinks,
		"heading_path":  c.HeadingPath,
		"heading_level": c.HeadingLevel,
		"has_table":     c.HasTable,
		"has_task":      c.HasTask,
		"code_blocks":   c.CodeBlocks,
		"has_code":      c.CodeBlocks > 0,
		"modified_ms":   int(c.ModifiedMs),
		"content_hash":  c.ContentHash,
		"tag_count":     len(c.Tags),
	}
	// Encode tags as tag_0, tag_1, tag_2, ...
	for i, tag := range c.Tags {
		meta["tag_"+strconv.Itoa(i)] = tag
	}
	return meta
}
