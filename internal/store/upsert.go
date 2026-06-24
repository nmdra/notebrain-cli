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

	// 1. Upsert chunks (replaces existing by ID, adds new)
	if err := s.upsertChunks(ctx, chunks); err != nil {
		return err
	}
	// 2. Clean up any stale chunks (if new version has fewer chunks)
	if err := s.cleanupNoteChunks(ctx, noteSlug, len(chunks)); err != nil {
		return err
	}
	// 3. Sync links (upserts new/existing, deletes stale)
	return s.upsertLinks(ctx, noteSlug, links)
}

type BatchIngestData struct {
	NoteSlug     string
	ChunkRecords []ChunkRecord
	Links        []string
}

// BatchIngest atomically replaces all chunks and links for a batch of notes, and deletes stale notes.
// It uses a single mutex lock to serialize operations on the store.
func (s *Store) BatchIngest(ctx context.Context, data []BatchIngestData, staleSlugs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Gather all slugs we need to delete from the index
	var slugsToClean []string
	slugsToClean = append(slugsToClean, staleSlugs...)
	for _, d := range data {
		slugsToClean = append(slugsToClean, d.NoteSlug)
	}

	if len(slugsToClean) > 0 {
		// 2. Fetch and delete existing chunks and links for these slugs in batches of 100
		const batchSize = 100
		for i := 0; i < len(slugsToClean); i += batchSize {
			end := i + batchSize
			if end > len(slugsToClean) {
				end = len(slugsToClean)
			}
			batchSlugs := slugsToClean[i:end]

			var filters []chroma.WhereClause
			for _, slug := range batchSlugs {
				filters = append(filters, chroma.EqString("note_slug", slug))
			}

			var whereFilter chroma.WhereFilter
			if len(filters) == 1 {
				whereFilter = filters[0]
			} else {
				whereFilter = chroma.Or(filters...)
			}

			// Fetch existing chunk IDs
			resChunks, err := s.chunks.Get(ctx,
				chroma.WithWhere(whereFilter),
				chroma.WithInclude(chroma.IncludeMetadatas),
			)
			if err != nil {
				return fmt.Errorf("fetch chunk IDs for cleanup: %w", err)
			}

			// Delete existing chunks
			if len(resChunks.GetIDs()) > 0 {
				if err := s.chunks.Delete(ctx, chroma.WithIDs(resChunks.GetIDs()...)); err != nil {
					return fmt.Errorf("delete chunks batch: %w", err)
				}
			}

			// Fetch existing links IDs (links metadata uses source_slug instead of note_slug)
			var linksFilters []chroma.WhereClause
			for _, slug := range batchSlugs {
				linksFilters = append(linksFilters, chroma.EqString("source_slug", slug))
			}
			var linksWhereFilter chroma.WhereFilter
			if len(linksFilters) == 1 {
				linksWhereFilter = linksFilters[0]
			} else {
				linksWhereFilter = chroma.Or(linksFilters...)
			}

			resLinks, err := s.links.Get(ctx,
				chroma.WithWhere(linksWhereFilter),
				chroma.WithInclude(chroma.IncludeMetadatas),
			)
			if err != nil {
				return fmt.Errorf("fetch links IDs for cleanup: %w", err)
			}

			// Delete existing links
			if len(resLinks.GetIDs()) > 0 {
				if err := s.links.Delete(ctx, chroma.WithIDs(resLinks.GetIDs()...)); err != nil {
					return fmt.Errorf("delete links batch: %w", err)
				}
			}
		}
	}

	// 3. Batch upsert new chunks
	var allChunks []ChunkRecord
	for _, d := range data {
		allChunks = append(allChunks, d.ChunkRecords...)
	}
	if len(allChunks) > 0 {
		if err := s.upsertChunks(ctx, allChunks); err != nil {
			return fmt.Errorf("batch upsert chunks: %w", err)
		}
	}

	// 4. Batch upsert new links
	for _, d := range data {
		if len(d.Links) > 0 {
			if err := s.upsertLinks(ctx, d.NoteSlug, d.Links); err != nil {
				return fmt.Errorf("batch upsert links for %s: %w", d.NoteSlug, err)
			}
		}
	}

	return nil
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

	const batchSize = 2000
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		ids := make([]chroma.DocumentID, len(batch))
		texts := make([]string, len(batch))
		embs := make([]embeddings.Embedding, len(batch))
		metas := make([]chroma.DocumentMetadata, len(batch))

		for j, c := range batch {
			ids[j] = chroma.DocumentID(c.ID)
			texts[j] = c.Text
			embs[j] = embeddings.NewEmbeddingFromFloat32(c.Embedding)
			metaMap := buildChunkMeta(c)
			metas[j], _ = chroma.NewDocumentMetadataFromMap(metaMap)
		}

		err := s.chunks.Upsert(ctx,
			chroma.WithIDs(ids...),
			chroma.WithTexts(texts...),
			chroma.WithEmbeddings(embs...),
			chroma.WithMetadatas(metas...),
		)
		if err != nil {
			return fmt.Errorf("upsert chunk batch [%d:%d]: %w", i, end, err)
		}
	}

	return nil
}

// DeleteNoteChunks removes all chunks belonging to a note (before re-ingest).
func (s *Store) DeleteNoteChunks(ctx context.Context, noteSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chunks.Delete(ctx,
		chroma.WithWhere(chroma.EqString("note_slug", noteSlug)),
	)
}

// DeleteNoteLinks removes all outgoing links for a note.
func (s *Store) DeleteNoteLinks(ctx context.Context, noteSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.links.Delete(ctx,
		chroma.WithWhere(chroma.EqString("source_slug", noteSlug)),
	)
}

// cleanupNoteChunks deletes chunks for a note with an index >= validCount.
// This removes stale chunks when a note shrinks, without dropping and recreating
// the valid ones (which triggers an hnswlib race condition/bug).
func (s *Store) cleanupNoteChunks(ctx context.Context, noteSlug string, validCount int) error {
	return s.chunks.Delete(ctx,
		chroma.WithWhere(chroma.And(
			chroma.EqString("note_slug", noteSlug),
			chroma.GteInt("chunk_index", validCount),
		)),
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
	// Deduplicate links
	var uniqueLinks []string
	seenSlugs := make(map[string]bool)
	targetSlugMap := make(map[string]string) // original -> slug

	for _, l := range links {
		targetSlug := parser.Slugify(l)
		if targetSlug == "" || seenSlugs[targetSlug] {
			continue
		}
		seenSlugs[targetSlug] = true
		uniqueLinks = append(uniqueLinks, l)
		targetSlugMap[l] = targetSlug
	}

	// 1. Fetch existing links for this source_slug
	existing, err := s.links.Get(ctx,
		chroma.WithWhere(chroma.EqString("source_slug", noteSlug)),
		chroma.WithInclude(chroma.IncludeMetadatas), // minimal fetch
	)
	if err != nil {
		return fmt.Errorf("fetch existing links for %s: %w", noteSlug, err)
	}

	// 2. Identify stale links to delete
	var staleIDs []chroma.DocumentID
	for _, docID := range existing.GetIDs() {
		// docID format: source_slug + "→" + target_slug
		parts := strings.Split(string(docID), "→")
		if len(parts) == 2 {
			tgtSlug := parts[1]
			if !seenSlugs[tgtSlug] {
				staleIDs = append(staleIDs, docID)
			}
		}
	}

	if len(staleIDs) > 0 {
		if err := s.links.Delete(ctx, chroma.WithIDs(staleIDs...)); err != nil {
			return fmt.Errorf("delete stale links for %s: %w", noteSlug, err)
		}
	}

	if len(uniqueLinks) == 0 {
		return nil
	}

	ids := make([]chroma.DocumentID, len(uniqueLinks))
	texts := make([]string, len(uniqueLinks)) // placeholder text — Chroma requires non-empty
	metas := make([]chroma.DocumentMetadata, len(uniqueLinks))

	for i, l := range uniqueLinks {
		targetSlug := targetSlugMap[l]
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

	// nb_links has no embedding function — pass zero-length embeddings or
	// create the collection with no embedding function.
	// Simplest: store links with a dummy 16-dim embedding.
	embs := make([]embeddings.Embedding, len(uniqueLinks))
	// Add dummy 16-dimensional embeddings to bypass Chroma dimension checks (required).
	// Using 16 distinct dimensions in L2 space avoids HNSW pathologically failing or corrupting
	// on identical/degenerate vector spaces.
	for i := range uniqueLinks {
		vec := make([]float32, 16)
		for j := 0; j < 16; j++ {
			vec[j] = rand.Float32()
		}
		embs[i] = embeddings.NewEmbeddingFromFloat32(vec)
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
