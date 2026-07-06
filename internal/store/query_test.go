package store_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
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
		res, err := st.SemanticSearch(ctx, qVec, 10, 1, nil, false)
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

func TestGetNoteHashes(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:          "note-hash-test:0",
			NoteSlug:    "note-hash-test",
			Title:       "Note Hash",
			FilePath:    "Note Hash.md",
			ChunkIndex:  0,
			ContentHash: "abcdef123456",
			Embedding:   []float32{1.0, 0.0, 0.0},
		},
	}
	_ = st.UpsertChunks(ctx, chunks)

	hashes, err := st.GetNoteHashes(ctx)
	if err != nil {
		t.Fatalf("GetNoteHashes failed: %v", err)
	}

	if val, ok := hashes["note-hash-test"]; !ok || val != "abcdef123456" {
		t.Errorf("GetNoteHashes returned unexpected or missing hash: %v", hashes)
	}
}

func TestTagSearch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	setupTestData(t, ctx, st)

	res, err := st.TagSearch(ctx, "vector", 10, nil, false)
	if err != nil {
		t.Fatalf("TagSearch failed: %v", err)
	}
	if len(res) == 0 || res[0].NoteSlug != "note-a" {
		t.Errorf("Expected note-a for tag 'vector', got %v", res)
	}
}

func TestGetNote(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	setupTestData(t, ctx, st)

	note, err := st.GetNote(ctx, "note-a")
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}
	if note.NoteSlug != "note-a" {
		t.Errorf("Expected NoteSlug note-a, got %s", note.NoteSlug)
	}
	if !strings.Contains(note.Text, "golang and chroma") {
		t.Errorf("Expected full text to contain golang and chroma, got %s", note.Text)
	}
}

func TestSemanticSearch_TopKDeduplication(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "multi:0",
			NoteSlug:   "multi",
			Title:      "Multi Note",
			FilePath:   "multi.md",
			ChunkIndex: 0,
			Text:       "chunk zero",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "multi:1",
			NoteSlug:   "multi",
			Title:      "Multi Note",
			FilePath:   "multi.md",
			ChunkIndex: 1,
			Text:       "chunk one",
			Embedding:  []float32{0.99, 0.0, 0.0},
		},
		{
			ID:         "multi:2",
			NoteSlug:   "multi",
			Title:      "Multi Note",
			FilePath:   "multi.md",
			ChunkIndex: 2,
			Text:       "chunk two",
			Embedding:  []float32{0.98, 0.0, 0.0},
		},
		{
			ID:         "multi:3",
			NoteSlug:   "multi",
			Title:      "Multi Note",
			FilePath:   "multi.md",
			ChunkIndex: 3,
			Text:       "chunk three",
			Embedding:  []float32{0.97, 0.0, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	// With topK=2, we should get exactly 2 chunks for note "multi".
	res, err := st.SemanticSearch(ctx, []float32{1.0, 0.0, 0.0}, 10, 2, nil, false)
	if err != nil {
		t.Fatalf("SemanticSearch failed: %v", err)
	}
	count := 0
	for _, r := range res {
		if r.NoteSlug == "multi" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("Expected exactly 2 chunks for note 'multi' with topK=2, got %d", count)
	}
}

func TestGetChunkWindow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "win:0",
			NoteSlug:   "win",
			Title:      "Window Note",
			FilePath:   "win.md",
			ChunkIndex: 0,
			Text:       "chunk zero",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "win:1",
			NoteSlug:   "win",
			Title:      "Window Note",
			FilePath:   "win.md",
			ChunkIndex: 1,
			Text:       "chunk one",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "win:2",
			NoteSlug:   "win",
			Title:      "Window Note",
			FilePath:   "win.md",
			ChunkIndex: 2,
			Text:       "chunk two",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "win:3",
			NoteSlug:   "win",
			Title:      "Window Note",
			FilePath:   "win.md",
			ChunkIndex: 3,
			Text:       "chunk three",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	win, err := st.GetChunkWindow(ctx, "win", 1, 1)
	if err != nil {
		t.Fatalf("GetChunkWindow failed: %v", err)
	}
	if win == nil || win.MatchedIndex != 1 {
		t.Fatalf("Expected window for matched index 1, got %v", win)
	}
	if len(win.Texts) != 3 {
		t.Fatalf("Expected 3 texts in window, got %d: %v", len(win.Texts), win.Texts)
	}
	if win.Texts[0] != "chunk zero" || win.Texts[1] != "chunk one" || win.Texts[2] != "chunk two" {
		t.Errorf("Unexpected window texts: %v", win.Texts)
	}

	res := []store.Result{
		{NoteSlug: "win", ChunkIndex: 1},
	}
	st.PopulateContext(ctx, res, 1)
	if len(res[0].Context) != 3 {
		t.Fatalf("Expected PopulateContext to fill 3 texts, got %d", len(res[0].Context))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	setupTestData(t, ctx, st)

	var wg sync.WaitGroup
	qVec := []float32{1.0, 0.0, 0.0}

	// Spawn 10 concurrent readers
	for range 10 {
		wg.Go(func() {
			for range 5 {
				_, _ = st.SemanticSearch(ctx, qVec, 5, 1, nil, false)
				_, _ = st.GetNote(ctx, "note-a")
				_, _ = st.GetNoteHashes(ctx)
			}
		})
	}

	// Spawn 2 concurrent writers
	for i := range 2 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 3 {
				slug := fmt.Sprintf("note-conc-%d-%d", id, j)
				chunks := []store.ChunkRecord{
					{
						ID:         slug + ":0",
						NoteSlug:   slug,
						Title:      "Conc Note",
						FilePath:   slug + ".md",
						ChunkIndex: 0,
						Text:       "concurrent text",
						Embedding:  []float32{0.5, 0.5, 0.0},
					},
				}
				_ = st.UpsertChunks(ctx, chunks)
				_ = st.UpsertLinks(ctx, slug, []string{"note-a"})
			}
		}(i)
	}

	wg.Wait()
}

func TestConnections_PhantomAndAttachment(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "src-note:0",
			NoteSlug:   "src-note",
			Title:      "Source Note",
			FilePath:   "Source Note.md",
			ChunkIndex: 0,
			Text:       "source text",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "real-target:0",
			NoteSlug:   "real-target",
			Title:      "Real Target",
			FilePath:   "Real Target.md",
			ChunkIndex: 0,
			Text:       "real target text",
			Embedding:  []float32{0.0, 1.0, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	// Link to a real note, a phantom (uncreated) note, and an image attachment.
	st.SkipAttachments = false
	links := []string{"real-target", "Phantom Note", "image.webp"}
	if err := st.UpsertLinks(ctx, "src-note", links); err != nil {
		t.Fatalf("UpsertLinks failed: %v", err)
	}

	// Case 1: SkipAttachments = true (default)
	st.SkipAttachments = true
	res, err := st.Connections(ctx, "src-note", 1)
	if err != nil {
		t.Fatalf("Connections failed: %v", err)
	}

	if len(res) != 2 {
		t.Fatalf("Expected 2 connections when SkipAttachments=true, got %d: %v", len(res), res)
	}

	resMap := make(map[string]store.Result)
	for _, r := range res {
		resMap[r.NoteSlug] = r
	}

	realRes, ok := resMap["real-target"]
	if !ok || realRes.IsPhantom {
		t.Errorf("Expected real-target in results with IsPhantom=false, got %v", realRes)
	}

	phantomRes, ok := resMap["phantom-note"]
	if !ok || !phantomRes.IsPhantom {
		t.Errorf("Expected phantom-note in results with IsPhantom=true, got %v", phantomRes)
	}

	if _, ok := resMap["imagewebp"]; ok {
		t.Errorf("Did not expect image.webp in results when SkipAttachments=true")
	}

	// Case 2: SkipAttachments = false
	st.SkipAttachments = false
	resNoSkip, err := st.Connections(ctx, "src-note", 1)
	if err != nil {
		t.Fatalf("Connections failed: %v", err)
	}
	if len(resNoSkip) != 3 {
		t.Fatalf("Expected 3 connections when SkipAttachments=false, got %d: %v", len(resNoSkip), resNoSkip)
	}
}

func TestBacklinks_Attachment(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "src-note:0",
			NoteSlug:   "src-note",
			Title:      "Source Note",
			FilePath:   "Source Note.md",
			ChunkIndex: 0,
			Text:       "source text",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
	}
	_ = st.UpsertChunks(ctx, chunks)
	st.SkipAttachments = false
	_ = st.UpsertLinks(ctx, "src-note", []string{"diagram.canvas"})

	st.SkipAttachments = true
	res, err := st.Backlinks(ctx, "diagramcanvas")
	if err != nil {
		t.Fatalf("Backlinks failed: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("Expected 0 backlinks for attachment when SkipAttachments=true, got %v", res)
	}

	st.SkipAttachments = false
	resNoSkip, err := st.Backlinks(ctx, "diagramcanvas")
	if err != nil {
		t.Fatalf("Backlinks failed: %v", err)
	}
	if len(resNoSkip) != 1 || resNoSkip[0].NoteSlug != "src-note" {
		t.Errorf("Expected 1 backlink when SkipAttachments=false, got %v", resNoSkip)
	}
}

func TestMultiSemanticSearch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []store.ChunkRecord{
		{
			ID:         "note-a:0",
			NoteSlug:   "note-a",
			Title:      "Note A",
			FilePath:   "Note A.md",
			ChunkIndex: 0,
			Text:       "redis text",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "note-b:0",
			NoteSlug:   "note-b",
			Title:      "Note B",
			FilePath:   "Note B.md",
			ChunkIndex: 0,
			Text:       "broker text",
			Embedding:  []float32{0.0, 1.0, 0.0},
		},
		{
			ID:         "note-c:0",
			NoteSlug:   "note-c",
			Title:      "Note C",
			FilePath:   "Note C.md",
			ChunkIndex: 0,
			Text:       "redis and broker text",
			Embedding:  []float32{0.707, 0.707, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	queryVecs := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
	}
	queries := []string{"redis", "broker"}

	res, err := st.MultiSemanticSearch(ctx, queryVecs, queries, 10, 3, nil, false)
	if err != nil {
		t.Fatalf("MultiSemanticSearch failed: %v", err)
	}

	if len(res) < 3 {
		t.Fatalf("Expected at least 3 results, got %d", len(res))
	}

	// Chunk C should be ranked first because it matched both queries
	if res[0].NoteSlug != "note-c" {
		t.Errorf("Expected note-c first (multi-hit boost), got %s", res[0].NoteSlug)
	}
	if len(res[0].MatchedQueries) != 2 {
		t.Errorf("Expected 2 matched queries for note-c, got %v", res[0].MatchedQueries)
	}
}
