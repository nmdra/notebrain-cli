package store_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestUpsertAndDeleteChunks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

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

	err := st.UpsertChunks(ctx, chunks)
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
	st := newTestStore(t)

	links := []string{"other-note"}

	err := st.UpsertLinks(ctx, "test-note", links)
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

func TestBatchIngest(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// 1. Initial Batch Ingest: Ingest Note A and Note B
	data := []store.BatchIngestData{
		{
			NoteSlug: "note-a",
			ChunkRecords: []store.ChunkRecord{
				{ID: "note-a:0", NoteSlug: "note-a", ChunkIndex: 0, Text: "chunk A0", Embedding: []float32{0.1}},
				{ID: "note-a:1", NoteSlug: "note-a", ChunkIndex: 1, Text: "chunk A1", Embedding: []float32{0.2}},
			},
			Links: []string{"note-b"},
		},
		{
			NoteSlug: "note-b",
			ChunkRecords: []store.ChunkRecord{
				{ID: "note-b:0", NoteSlug: "note-b", ChunkIndex: 0, Text: "chunk B0", Embedding: []float32{0.3}},
			},
			Links: []string{"note-c"},
		},
	}

	err := st.BatchIngest(ctx, data, nil)
	if err != nil {
		t.Fatalf("Initial BatchIngest failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 3 || stats["links"] != 2 {
		t.Errorf("Expected 3 chunks, 2 links after initial batch, got %v", stats)
	}

	// 2. Modify Note A (shrink to 1 chunk, update links), Delete Note B (as stale slug), and Add Note C
	data2 := []store.BatchIngestData{
		{
			NoteSlug: "note-a",
			ChunkRecords: []store.ChunkRecord{
				{ID: "note-a:0", NoteSlug: "note-a", ChunkIndex: 0, Text: "chunk A0 updated", Embedding: []float32{0.15}},
			},
			Links: []string{"note-c"},
		},
		{
			NoteSlug: "note-c",
			ChunkRecords: []store.ChunkRecord{
				{ID: "note-c:0", NoteSlug: "note-c", ChunkIndex: 0, Text: "chunk C0", Embedding: []float32{0.4}},
			},
			Links: []string{},
		},
	}

	err = st.BatchIngest(ctx, data2, []string{"note-b"})
	if err != nil {
		t.Fatalf("Second BatchIngest failed: %v", err)
	}

	stats, _ = st.Stats(ctx)
	// Expected:
	// - note-a: 1 chunk, 1 link (links: "note-c")
	// - note-b: deleted (0 chunks, 0 links)
	// - note-c: 1 chunk, 0 links
	// Total: chunks = 2 (note-a:0, note-c:0), links = 1 (note-a -> note-c)
	if stats["chunks"] != 2 || stats["links"] != 1 {
		t.Errorf("Expected 2 chunks, 1 link after updates/delete, got %v", stats)
	}
}

func BenchmarkUpsertLinks(b *testing.B) {
	ctx := context.Background()
	st := newTestStore(b)

	links := make([]string, 100)
	for i := range 100 {
		links[i] = "target-note-" + strconv.Itoa(i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = st.UpsertLinks(ctx, "bench-note", links)
	}
}
