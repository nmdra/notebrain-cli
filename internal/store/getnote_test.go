package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// multiChunkNote seeds a note with three distinct chunk texts.
func multiChunkNote(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	chunks := []store.ChunkRecord{
		{ID: "multi:0", NoteSlug: "multi", Title: "Multi Note", FilePath: "Multi.md", ChunkIndex: 0, Text: "chunk zero text", Tags: []string{"go"}, Embedding: []float32{1, 0, 0}},
		{ID: "multi:1", NoteSlug: "multi", Title: "Multi Note", FilePath: "Multi.md", ChunkIndex: 1, Text: "chunk one text", Embedding: []float32{1, 0, 0}},
		{ID: "multi:2", NoteSlug: "multi", Title: "Multi Note", FilePath: "Multi.md", ChunkIndex: 2, Text: "chunk two text", Embedding: []float32{1, 0, 0}},
	}
	seedChunks(t, ctx, st, chunks, nil)
}

func TestGetNoteMeta(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	multiChunkNote(t, ctx, st)

	meta, err := st.GetNoteMeta(ctx, "multi")
	if err != nil {
		t.Fatalf("GetNoteMeta: %v", err)
	}
	if meta.NoteSlug != "multi" || meta.Title != "Multi Note" || meta.FilePath != "Multi.md" {
		t.Errorf("meta header mismatch: %+v", meta)
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "go" {
		t.Errorf("meta tags = %v, want [go]", meta.Tags)
	}
	if meta.Chunks != 3 {
		t.Errorf("meta chunks = %d, want 3", meta.Chunks)
	}
	if meta.Text != "" {
		t.Errorf("meta must not carry text, got %q", meta.Text)
	}
}

func TestGetNoteMetaUnknownNote(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	_, err := st.GetNoteMeta(ctx, "no-such-note")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("GetNoteMeta on unknown note: got %v, want 'not found' error", err)
	}
}

func TestGetNoteHead(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	multiChunkNote(t, ctx, st)

	head, err := st.GetNoteHead(ctx, "multi", 2)
	if err != nil {
		t.Fatalf("GetNoteHead: %v", err)
	}
	if head.Chunks != 3 {
		t.Errorf("head chunks = %d, want total 3", head.Chunks)
	}
	if !strings.Contains(head.Text, "chunk zero text") || !strings.Contains(head.Text, "chunk one text") {
		t.Errorf("head text missing first chunks: %q", head.Text)
	}
	if strings.Contains(head.Text, "chunk two text") {
		t.Errorf("head text must not include chunks beyond the limit: %q", head.Text)
	}
}

func TestGetNoteHeadExceedsChunks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	multiChunkNote(t, ctx, st)

	head, err := st.GetNoteHead(ctx, "multi", 99)
	if err != nil {
		t.Fatalf("GetNoteHead: %v", err)
	}
	if head.Chunks != 3 || !strings.Contains(head.Text, "chunk two text") {
		t.Errorf("head beyond note size must return the full note: chunks=%d text=%q", head.Chunks, head.Text)
	}
}

func TestGetNoteUnchanged(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	multiChunkNote(t, ctx, st)

	note, err := st.GetNote(ctx, "multi")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	for _, want := range []string{"chunk zero text", "chunk one text", "chunk two text"} {
		if !strings.Contains(note.Text, want) {
			t.Errorf("GetNote text missing %q: %q", want, note.Text)
		}
	}
	if note.Chunks != 3 {
		t.Errorf("GetNote chunks = %d, want 3", note.Chunks)
	}
}
