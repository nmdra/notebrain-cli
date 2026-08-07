package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestResolveNoteSlugStaleSlugNotPreferred(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// A stale note whose note_slug equals the slugified form of the input
	// title must NOT win over the real note's exact title match. The stale
	// note's path differs from the input, so it only matches via the
	// slugified-guess suffix — which must lose to a real title match.
	chunks := []store.ChunkRecord{
		{
			ID:         "stale:0",
			NoteSlug:   "1-java-ee-ead",
			Title:      "Old Java EE (stale)",
			FilePath:   "Old Java EE (stale).md",
			ChunkIndex: 0,
			Embedding:  []float32{1, 0, 0},
		},
		{
			ID:         "real:0",
			NoteSlug:   "04archives-real",
			Title:      "1. Java EE (EAD)",
			FilePath:   "04 Archives/1. Java EE (EAD).md",
			ChunkIndex: 0,
			Embedding:  []float32{0, 1, 0},
		},
	}
	seedChunks(t, ctx, st, chunks, nil)

	got, err := st.ResolveNoteSlug(ctx, "1. Java EE (EAD)")
	if err != nil {
		t.Fatalf("ResolveNoteSlug: %v", err)
	}
	if got != "04archives-real" {
		t.Errorf("ResolveNoteSlug(%q) = %q, want %q (stale slug must not win)", "1. Java EE (EAD)", got, "04archives-real")
	}
}

func TestResolveNoteSlugMissingReturnsError(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	_, err := st.ResolveNoteSlug(ctx, "no-such-note")
	if err == nil {
		t.Fatal("ResolveNoteSlug on unknown note: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("ResolveNoteSlug error = %q, want 'not found' hint", err.Error())
	}
}
