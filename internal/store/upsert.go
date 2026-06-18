package store

import (
	"context"
	"fmt"
	"strconv"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/nmdra/notebrain-cli/internal/obsidian"
	"github.com/nmdra/notebrain-cli/internal/parser"
)

// ChunkRecord holds everything needed to store one chunk.
type ChunkRecord struct {
	ID         string // "<slug>:<index>"
	NoteSlug   string
	Title      string
	FilePath   string
	ChunkIndex int
	Text       string
	Tags       []string
	HasLinks   bool
	ModifiedMs int64
	Embedding  []float32
}

// UpsertChunks stores a batch of chunks (upsert = insert or replace by ID).
// Call DeleteNoteChunks first to cleanly re-ingest a note.
func (s *Store) UpsertChunks(ctx context.Context, chunks []ChunkRecord) error {
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
	return s.chunks.Delete(ctx,
		chroma.WithWhere(chroma.EqString("note_slug", noteSlug)),
	)
}

// UpsertLinks replaces all outgoing links for a note.
func (s *Store) UpsertLinks(ctx context.Context, noteSlug string, links []obsidian.LinkRecord) error {
	// Delete old outgoing links for this note
	if err := s.links.Delete(ctx,
		chroma.WithWhere(chroma.EqString("source_slug", noteSlug)),
	); err != nil {
		return fmt.Errorf("delete old links for %s: %w", noteSlug, err)
	}

	if len(links) == 0 {
		return nil
	}

	ids := make([]chroma.DocumentID, len(links))
	texts := make([]string, len(links)) // placeholder text — Chroma requires non-empty
	metas := make([]chroma.DocumentMetadata, len(links))

	for i, l := range links {
		targetSlug := parser.Slugify(l.Path) // derive slug from resolved path
		ids[i] = chroma.DocumentID(noteSlug + "→" + targetSlug)
		texts[i] = l.DisplayText
		if texts[i] == "" {
			texts[i] = "-"
		}
		metaMap := map[string]interface{}{
			"source_slug":  noteSlug,
			"target_slug":  targetSlug,
			"target_path":  l.Path,
			"display_text": l.DisplayText,
		}
		metas[i], _ = chroma.NewDocumentMetadataFromMap(metaMap)
	}

	// sb_links has no embedding function — pass zero-length embeddings or
	// create the collection with no embedding function.
	// Simplest: store links with a dummy 1-dim embedding.
	dummyEmbs := make([]embeddings.Embedding, len(links))
	for i := range dummyEmbs {
		dummyEmbs[i] = embeddings.NewEmbeddingFromFloat32([]float32{0})
	}

	return s.links.Upsert(ctx,
		chroma.WithIDs(ids...),
		chroma.WithTexts(texts...),
		chroma.WithEmbeddings(dummyEmbs...),
		chroma.WithMetadatas(metas...),
	)
}

// ─── Metadata helpers ────────────────────────────────────────────

func buildChunkMeta(c ChunkRecord) map[string]interface{} {
	meta := map[string]interface{}{
		"note_slug":   c.NoteSlug,
		"title":       c.Title,
		"file_path":   c.FilePath,
		"chunk_index": c.ChunkIndex,
		"word_count":  len(splitWords(c.Text)),
		"has_links":   c.HasLinks,
		"modified_ms": int(c.ModifiedMs),
		"tag_count":   len(c.Tags),
	}
	// Encode tags as tag_0, tag_1, tag_2, ...
	for i, tag := range c.Tags {
		meta["tag_"+strconv.Itoa(i)] = tag
	}
	return meta
}

func splitWords(s string) []string {
	// simple whitespace split for word count
	var words []string
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			inWord = false
		} else if !inWord {
			inWord = true
			words = append(words, "")
		}
	}
	return words
}
