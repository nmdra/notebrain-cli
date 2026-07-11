package store_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
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

func setupStoreTest(t *testing.T) (context.Context, *store.Store, []float32) {
	ctx := context.Background()
	st := newTestStore(t)
	setupTestData(t, ctx, st)
	return ctx, st, []float32{1.0, 0.0, 0.0}
}

func TestSemanticSearch(t *testing.T) {
	ctx, st, qVec := setupStoreTest(t)
	res, err := st.SemanticSearch(ctx, qVec, 10, 1, nil, false)
	if err != nil {
		t.Fatalf("SemanticSearch failed: %v", err)
	}
	if len(res) == 0 || res[0].NoteSlug != "note-a" {
		t.Errorf("Expected note-a to be best match, got %v", res)
	}
}

func TestBacklinks(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)
	res, err := st.Backlinks(ctx, "note-b")
	if err != nil {
		t.Fatalf("Backlinks failed: %v", err)
	}
	if len(res) == 0 || res[0].NoteSlug != "note-a" {
		t.Errorf("Expected note-a to backlink to note-b, got %v", res)
	}
}

func TestConnections(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)
	res, err := st.Connections(ctx, "note-a", 1)
	if err != nil {
		t.Fatalf("Connections failed: %v", err)
	}
	if len(res) == 0 || res[0].NoteSlug != "note-b" {
		t.Errorf("Expected note-b to be connected to note-a, got %v", res)
	}
}

func TestHiddenConnections(t *testing.T) {
	ctx, st, qVec := setupStoreTest(t)
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
}

func TestHiddenConnectionsDeep(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)
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

	hidden, seedChunks, err := st.HiddenConnectionsDeep(ctx, "note-a", 10, 3, false)
	if err != nil {
		t.Fatalf("HiddenConnectionsDeep failed: %v", err)
	}
	if len(hidden) == 0 || hidden[0].NoteSlug != "note-c" {
		t.Errorf("Expected note-c to be deep hidden connection to note-a, got %v", hidden)
	}
	if len(seedChunks) == 0 {
		t.Errorf("Expected non-empty seedChunks, got %v", seedChunks)
	}
	if len(hidden) > 0 && len(hidden[0].MatchedQueries) == 0 {
		t.Errorf("Expected MatchedQueries to be populated on result, got %v", hidden[0].MatchedQueries)
	}

	_, _, err = st.HiddenConnectionsDeep(ctx, "non-existent-note", 10, 3, false)
	if err == nil {
		t.Errorf("Expected error for non-existent note in deep hidden check, got nil")
	}
}

func TestMultiSemanticSearch_ThresholdFiltering(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)
	chunks := []store.ChunkRecord{
		{
			ID:         "note-x:0",
			NoteSlug:   "note-x",
			Title:      "Note X",
			FilePath:   "Note X.md",
			ChunkIndex: 0,
			Text:       "target vector alignment",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
	}
	_ = st.UpsertChunks(ctx, chunks)

	queryVecs := [][]float32{
		{1.0, 0.0, 0.0},
		{0.4, 0.9165, 0.0},
	}
	queries := []string{"§ StrongMatch", "§ WeakMatch"}

	res, err := st.MultiSemanticSearch(ctx, queryVecs, queries, 10, 3, nil, false)
	if err != nil {
		t.Fatalf("MultiSemanticSearch failed: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("Expected results from MultiSemanticSearch, got 0")
	}
	for _, r := range res {
		if r.NoteSlug == "note-x" {
			for _, mq := range r.MatchedQueries {
				if mq == "§ WeakMatch" {
					t.Errorf("Expected § WeakMatch to be filtered out by threshold, but got MatchedQueries=%v", r.MatchedQueries)
				}
			}
			foundStrong := false
			for _, mq := range r.MatchedQueries {
				if mq == "§ StrongMatch" {
					foundStrong = true
				}
			}
			if !foundStrong {
				t.Errorf("Expected § StrongMatch in MatchedQueries, got %v", r.MatchedQueries)
			}
		}
	}
}

func TestSharedTags(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)
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
}

func TestGraphBoostedSearch(t *testing.T) {
	ctx, st, qVec := setupStoreTest(t)
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
}

func TestResolveNoteSlug(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"exact match", "note-a", "note-a", false},
		{"from title", "Note A", "note-a", false},
		{"from filename", "Note A.md", "note-a", false},
		{"from suffix", "e-a", "note-a", false},
		{"note-b exact", "note-b", "note-b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.ResolveNoteSlug(ctx, tt.input)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("ResolveNoteSlug(%q) = %q, err: %v (want %q, wantErr: %v)", tt.input, got, err, tt.want, tt.wantErr)
			}
		})
	}

	ambigChunks := []store.ChunkRecord{
		{ID: "ambig-1:0", NoteSlug: "ambig-1", Title: "Ambig Note", FilePath: "dir1/Ambig Note.md", ChunkIndex: 0, Embedding: []float32{1.0, 0.0, 0.0}},
		{ID: "ambig-2:0", NoteSlug: "ambig-2", Title: "Ambig Note", FilePath: "dir2/Ambig Note.md", ChunkIndex: 0, Embedding: []float32{1.0, 0.0, 0.0}},
	}
	_ = st.UpsertChunks(ctx, ambigChunks)

	_, err := st.ResolveNoteSlug(ctx, "Ambig Note")
	if err == nil || !strings.Contains(err.Error(), "matches multiple indexed notes") {
		t.Errorf("Expected multiple match error for Ambig Note, got %v", err)
	}
}

func TestMultiSemanticSearch_WithText(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	chunks := []store.ChunkRecord{
		{
			ID:         "multi-text:0",
			NoteSlug:   "multi-text",
			Title:      "Multi Text Note",
			FilePath:   "multi-text.md",
			ChunkIndex: 0,
			Text:       "this is the document text of multi-text chunk zero",
			Embedding:  []float32{1.0, 0.0, 0.0},
		},
		{
			ID:         "multi-text-2:0",
			NoteSlug:   "multi-text-2",
			Title:      "Multi Text Note 2",
			FilePath:   "multi-text-2.md",
			ChunkIndex: 0,
			Text:       "this is the document text of multi-text-2 chunk zero",
			Embedding:  []float32{0.9, 0.1, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	queryVecs := [][]float32{
		{1.0, 0.0, 0.0},
		{0.9, 0.1, 0.0},
	}
	queries := []string{"query 1", "query 2"}

	res, err := st.MultiSemanticSearch(ctx, queryVecs, queries, 10, 2, nil, true)
	if err != nil {
		t.Fatalf("MultiSemanticSearch with text failed: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("Expected results, got none")
	}
	for _, r := range res {
		if r.Text == "" {
			t.Errorf("Expected populated Text for note %s, got empty", r.NoteSlug)
		}
	}
}

func TestCombineWhereFilters(t *testing.T) {
	var nilFilter chroma.WhereFilter
	wc1 := chroma.EqString("note_slug", "note-a")
	wc2 := chroma.EqInt("chunk_index", 0)

	if got := store.CombineWhereFilters(nilFilter, wc1); got != wc1 {
		t.Errorf("Expected wc1 when f1 is nil")
	}
	if got := store.CombineWhereFilters(wc2, nilFilter); got != wc2 {
		t.Errorf("Expected wc2 when f2 is nil")
	}
	if got := store.CombineWhereFilters(wc1, wc2); got == nil {
		t.Errorf("Expected combined clause when both non-nil")
	}
}

func TestTagWhereClause(t *testing.T) {
	wc := store.TagWhereClause("golang")
	if wc == nil {
		t.Errorf("Expected non-nil WhereClause")
	}
}

func TestGetNoteHashes(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

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
	st := newTestStore(t)

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
	st := newTestStore(t)

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

func TestGetNote_WithHeadings(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	chunks := []store.ChunkRecord{
		{
			ID:           "heading-note:0",
			NoteSlug:     "heading-note",
			Title:        "Heading Note",
			FilePath:     "Heading Note.md",
			ChunkIndex:   0,
			Text:         "This is the overview.",
			HeadingPath:  "Overview",
			HeadingLevel: 1,
			ModifiedMs:   time.Now().UnixMilli(),
			Embedding:    []float32{1.0, 0.0, 0.0},
		},
		{
			ID:           "heading-note:1",
			NoteSlug:     "heading-note",
			Title:        "Heading Note",
			FilePath:     "Heading Note.md",
			ChunkIndex:   1,
			Text:         "Second paragraph of overview.",
			HeadingPath:  "Overview",
			HeadingLevel: 1,
			ModifiedMs:   time.Now().UnixMilli(),
			Embedding:    []float32{1.0, 0.0, 0.0},
		},
		{
			ID:           "heading-note:2",
			NoteSlug:     "heading-note",
			Title:        "Heading Note",
			FilePath:     "Heading Note.md",
			ChunkIndex:   2,
			Text:         "Architectural details.",
			HeadingPath:  "Overview > Architecture",
			HeadingLevel: 2,
			ModifiedMs:   time.Now().UnixMilli(),
			Embedding:    []float32{1.0, 0.0, 0.0},
		},
		{
			ID:           "heading-note:3",
			NoteSlug:     "heading-note",
			Title:        "Heading Note",
			FilePath:     "Heading Note.md",
			ChunkIndex:   3,
			Text:         "Component A details.",
			HeadingPath:  "Overview > Architecture > Component A",
			HeadingLevel: 3,
			ModifiedMs:   time.Now().UnixMilli(),
			Embedding:    []float32{1.0, 0.0, 0.0},
		},
	}
	if err := st.UpsertChunks(ctx, chunks); err != nil {
		t.Fatalf("UpsertChunks failed: %v", err)
	}

	note, err := st.GetNote(ctx, "heading-note")
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}

	expected := `# Overview

This is the overview.

Second paragraph of overview.

## Architecture

Architectural details.

### Component A

Component A details.`

	if note.Text != expected {
		t.Errorf("GetNote text mismatch.\nExpected:\n%s\nGot:\n%s", expected, note.Text)
	}
}

func TestSemanticSearch_TopKDeduplication(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

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
	st := newTestStore(t)

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
	st := newTestStore(t)

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
	st := newTestStore(t)

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
	st := newTestStore(t)

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
	st := newTestStore(t)

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

func TestMultiSemanticSearch_EmptyAndEdgeCases(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	res, err := st.MultiSemanticSearch(ctx, nil, nil, 10, 3, nil, false)
	if err != nil {
		t.Fatalf("MultiSemanticSearch empty failed: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("Expected 0 results for empty queries, got %d", len(res))
	}
}

func TestStoreOpen_WithSkipAttachments(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir(), store.WithSkipAttachments(true))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	links := []string{"target-note", "image.png", "doc.pdf"}
	if err := st.UpsertLinks(ctx, "source-note", links); err != nil {
		t.Fatalf("UpsertLinks failed: %v", err)
	}

	backlinks, err := st.Backlinks(ctx, "target-note")
	if err != nil {
		t.Fatalf("Backlinks target-note failed: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].NoteSlug != "source-note" {
		t.Errorf("Expected backlink for target-note, got %v", backlinks)
	}

	// Because image.png was skipped during UpsertLinks due to WithSkipAttachments(true), Backlinks for image.png should be 0
	imgBacklinks, err := st.Backlinks(ctx, "image.png")
	if err != nil {
		t.Fatalf("Backlinks image.png failed: %v", err)
	}
	if len(imgBacklinks) != 0 {
		t.Errorf("Expected 0 backlinks for image.png when SkipAttachments=true, got %v", imgBacklinks)
	}
}
