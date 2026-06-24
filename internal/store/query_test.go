package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/nmdra/notebrain-cli/internal/store"
)

func setupTestData(t *testing.T, ctx context.Context, st *store.Store) {
	chunks := []store.ChunkRecord{
		{
			ID:         "note-a:0",
			NoteSlug:   "note-a",
			Title:      "Note A",
			FilePath:   "Note A.md",
			ChunkIndex: 0,
			Text:       "text about golang and chroma",
			Tags:       []string{"go", "vector"},
			HasLinks:   true,
			ModifiedMs: time.Now().UnixMilli(),
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "note-b:0",
			NoteSlug:   "note-b",
			Title:      "Note B",
			FilePath:   "Note B.md",
			ChunkIndex: 0,
			Text:       "some other text",
			Tags:       []string{"go"},
			HasLinks:   false,
			ModifiedMs: time.Now().UnixMilli(),
			Embedding:  []float32{0.0, 1.0, 0.0},
		},
	}
	err := st.UpsertChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("setup chunks: %v", err)
	}

	links := []string{"note-b"}
	if err := st.UpsertLinks(ctx, "note-a", links); err != nil {
		t.Fatalf("setup links: %v", err)
	}
}

func TestQueries(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	setupTestData(t, ctx, st)

	qVec := []float32{1.0, 0.0, 0.0}

	t.Run("SemanticSearch", func(t *testing.T) {
		res, err := st.SemanticSearch(ctx, qVec, 10, nil, false)
		if err != nil {
			t.Fatalf("SemanticSearch failed: %v", err)
		}
		if len(res) == 0 || res[0].NoteSlug != "note-a" {
			t.Errorf("Expected note-a to be best match, got %v", res)
		}
	})

	t.Run("Backlinks", func(t *testing.T) {
		res, err := st.Backlinks(ctx, "note-b")
		if err != nil {
			t.Fatalf("Backlinks failed: %v", err)
		}
		if len(res) == 0 || res[0].NoteSlug != "note-a" {
			t.Errorf("Expected note-a to backlink to note-b, got %v", res)
		}
	})

	t.Run("Connections", func(t *testing.T) {
		res, err := st.Connections(ctx, "note-a", 1)
		if err != nil {
			t.Fatalf("Connections failed: %v", err)
		}
		if len(res) == 0 || res[0].NoteSlug != "note-b" {
			t.Errorf("Expected note-b to be connected to note-a, got %v", res)
		}
	})

	t.Run("HiddenConnections", func(t *testing.T) {
		// query that matches note-a, but note-a is linked from note-a (self).
		// note-b is linked. Let's find note-c that is similar but not linked.
		chunks := []store.ChunkRecord{
			{
				ID:         "note-c:0",
				NoteSlug:   "note-c",
				Title:      "Note C",
				FilePath:   "Note C.md",
				ChunkIndex: 0,
				Text:       "text about golang",
				Tags:       []string{"go"},
				HasLinks:   false,
				Embedding:  []float32{0.9, 0.0, 0.0},
			},
		}
		_ = st.UpsertChunks(ctx, chunks)

		hidden, err := st.HiddenConnections(ctx, qVec, "note-a", 10, false)
		if err != nil {
			t.Fatalf("HiddenConnections failed: %v", err)
		}
		if len(hidden) == 0 || hidden[0].NoteSlug != "note-c" {
			t.Errorf("Expected note-c to be hidden connection to note-a, got %v", hidden)
		}
	})

	t.Run("SharedTags", func(t *testing.T) {
		res, err := st.SharedTags(ctx, "note-a", 1)
		if err != nil {
			t.Fatalf("SharedTags failed: %v", err)
		}
		if len(res) == 0 {
			t.Fatalf("Expected shared tags, got none")
		}
		found := false
		for _, r := range res {
			if r.NoteSlug == "note-b" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected note-b to share tags with note-a")
		}
	})

	t.Run("GraphBoostedSearch", func(t *testing.T) {
		// Querying near note-a, with seed note-b and boost
		boosted, err := st.GraphBoostedSearch(ctx, qVec, "note-b", 0.5, 10, false)
		if err != nil {
			t.Fatalf("GraphBoostedSearch failed: %v", err)
		}
		if len(boosted) == 0 {
			t.Fatalf("Expected results, got none")
		}

		found := false
		for _, r := range boosted {
			if r.NoteSlug == "note-b" {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected note-b in boosted results")
		}
	})
}
