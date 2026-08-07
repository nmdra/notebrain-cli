package store_test

import (
	"context"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestListTags(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	tags, err := st.ListTags(ctx, 0)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	want := []store.TagCount{
		{Tag: "go", Count: 2},
		{Tag: "vector", Count: 1},
	}
	if len(tags) != len(want) {
		t.Fatalf("ListTags = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("ListTags[%d] = %+v, want %+v", i, tags[i], want[i])
		}
	}

	limited, err := st.ListTags(ctx, 1)
	if err != nil {
		t.Fatalf("ListTags limit: %v", err)
	}
	if len(limited) != 1 || limited[0].Tag != "go" || limited[0].Count != 2 {
		t.Errorf("ListTags(limit=1) = %v, want [{go 2}] (highest count first)", limited)
	}
}

func TestListTagsEmpty(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	tags, err := st.ListTags(ctx, 0)
	if err != nil {
		t.Fatalf("ListTags on empty store: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("ListTags on empty store = %v, want none", tags)
	}
}

func TestListTagsCountsNotesNotChunks(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// note-a has two chunks both tagged "go"; it must count once.
	chunks := []store.ChunkRecord{
		{ID: "multi:0", NoteSlug: "multi", Title: "Multi", FilePath: "Multi.md", ChunkIndex: 0, Text: "c0", Tags: []string{"go"}, Embedding: []float32{1, 0, 0}},
		{ID: "multi:1", NoteSlug: "multi", Title: "Multi", FilePath: "Multi.md", ChunkIndex: 1, Text: "c1", Tags: []string{"go"}, Embedding: []float32{1, 0, 0}},
	}
	seedChunks(t, ctx, st, chunks, nil)

	tags, err := st.ListTags(ctx, 0)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Tag != "go" || tags[0].Count != 1 {
		t.Errorf("ListTags = %v, want single [{go 1}] (note counted once)", tags)
	}
}

func TestSuggestTags(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	// Extend with more tags so the ranking is meaningful.
	chunks := []store.ChunkRecord{
		{ID: "extra:0", NoteSlug: "extra", Title: "Extra", FilePath: "Extra.md", ChunkIndex: 0, Tags: []string{"golang", "kubernetes"}, Embedding: []float32{0, 0, 1}},
	}
	seedChunks(t, ctx, st, chunks, nil)

	tests := []struct {
		name  string
		input string
		limit int
		want  []string
	}{
		{"close typo resolves to exact tag", "gol", 3, []string{"go", "golang", "vector"}},
		{"distant input falls back to closest", "zzzzz", 1, []string{"go"}},
		{"exact match excluded", "golang", 3, []string{"go", "vector", "kubernetes"}},
		{"limit respected", "gol", 1, []string{"go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.SuggestTags(ctx, tt.input, tt.limit)
			if err != nil {
				t.Fatalf("SuggestTags: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("SuggestTags(%q, %d) = %v, want %v", tt.input, tt.limit, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("SuggestTags(%q)[%d] = %q, want %q (all: %v)", tt.input, i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestSuggestTagsEmptyStore(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	got, err := st.SuggestTags(ctx, "anything", 3)
	if err != nil {
		t.Fatalf("SuggestTags on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("SuggestTags on empty store = %v, want none", got)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"a", "a", 0},
		{"kitten", "sitting", 3},
		{"golang", "go", 4},
		{"GO", "go", 2},
	}
	for _, tt := range tests {
		if got := store.Levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
