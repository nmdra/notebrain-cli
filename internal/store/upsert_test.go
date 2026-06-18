package store_test

import (
	"context"
	"github.com/nmdra/notebrain-cli/internal/parser"
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

	links := []parser.Link{
		{
			Target:      "Other Note.md",
			DisplayText: "Other",
		},
	}

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
}
