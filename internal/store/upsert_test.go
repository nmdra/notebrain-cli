package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/nmdra/notebrain-cli/internal/store"
)

func TestUpsertAndDeleteChunks(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "test-note:0",
			NoteSlug:   "test-note",
			Title:      "Test Note",
			FilePath:   "test.md",
			ChunkIndex: 0,
			Text:       "hello world",
			Tags:       []string{"test", "go"},
			HasLinks:   true,
			ModifiedMs: time.Now().UnixMilli(),
			Embedding:  []float32{0.1, 0.2, 0.3},
		},
		{
			ID:         "test-note:1",
			NoteSlug:   "test-note",
			Title:      "Test Note",
			FilePath:   "test.md",
			ChunkIndex: 1,
			Text:       "more text here",
			Tags:       []string{"test"},
			HasLinks:   false,
			ModifiedMs: time.Now().UnixMilli(),
			Embedding:  []float32{0.4, 0.5, 0.6},
		},
	}

	err = st.UpsertChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 2 {
		t.Errorf("Expected 2 chunks, got %d", stats["chunks"])
	}

	// Delete
	err = st.DeleteNoteChunks(ctx, "test-note")
	if err != nil {
		t.Fatalf("DeleteNoteChunks failed: %v", err)
	}

	stats, _ = st.Stats(ctx)
	if stats["chunks"] != 0 {
		t.Errorf("Expected 0 chunks after delete, got %d", stats["chunks"])
	}
}

func TestUpsertLinks(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	links := []string{"other-note"}

	err = st.UpsertLinks(ctx, "test-note", links)
	if err != nil {
		t.Fatalf("UpsertLinks failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["links"] != 1 {
		t.Errorf("Expected 1 link, got %d", stats["links"])
	}

	// Upsert again should replace
	err = st.UpsertLinks(ctx, "test-note", links)
	if err != nil {
		t.Fatalf("UpsertLinks twice failed: %v", err)
	}
	stats, _ = st.Stats(ctx)
	if stats["links"] != 1 {
		t.Errorf("Expected 1 link after replacement, got %d", stats["links"])
	}

	// Delete links
	err = st.DeleteNoteLinks(ctx, "test-note")
	if err != nil {
		t.Fatalf("DeleteNoteLinks failed: %v", err)
	}
	stats, _ = st.Stats(ctx)
	if stats["links"] != 0 {
		t.Errorf("Expected 0 links after delete, got %d", stats["links"])
	}
}

func TestIngestNote(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{ID: "test-note:0", NoteSlug: "test-note", ChunkIndex: 0, Text: "chunk 0", Embedding: []float32{0.1}},
		{ID: "test-note:1", NoteSlug: "test-note", ChunkIndex: 1, Text: "chunk 1", Embedding: []float32{0.2}},
		{ID: "test-note:2", NoteSlug: "test-note", ChunkIndex: 2, Text: "chunk 2", Embedding: []float32{0.3}},
	}
	links := []string{"link-a", "link-b"}

	// Initial ingest
	err = st.IngestNote(ctx, "test-note", chunks, links)
	if err != nil {
		t.Fatalf("Initial IngestNote failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 3 || stats["links"] != 2 {
		t.Errorf("Expected 3 chunks, 2 links, got %v", stats)
	}

	// Re-ingest with fewer chunks and different links to trigger cleanup
	chunks2 := []store.ChunkRecord{
		{ID: "test-note:0", NoteSlug: "test-note", ChunkIndex: 0, Text: "chunk 0 updated", Embedding: []float32{0.1}},
	}
	links2 := []string{"link-c"}

	err = st.IngestNote(ctx, "test-note", chunks2, links2)
	if err != nil {
		t.Fatalf("Second IngestNote failed: %v", err)
	}

	stats, _ = st.Stats(ctx)
	if stats["chunks"] != 1 || stats["links"] != 1 {
		t.Errorf("Expected 1 chunk, 1 link after shrinking, got %v", stats)
	}
}
