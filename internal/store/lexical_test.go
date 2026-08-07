package store_test

import (
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

func TestLexicalSearch(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{"chunk text hit", "golang", 10, []string{"note-a"}},
		{"tag hit", "vector", 10, []string{"note-a"}},
		{"title and path hit", "note", 10, []string{"note-a", "note-b"}},
		{"multi-token all on one note", "golang chroma", 10, []string{"note-a"}},
		{"limit caps results", "note", 1, []string{"note-a"}},
		{"no match", "zzz", 10, nil},
		{"empty query", "", 10, nil},
		{"case insensitive", "GOLANG", 10, []string{"note-a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.LexicalSearch(ctx, tt.query, tt.limit, nil)
			if err != nil {
				t.Fatalf("LexicalSearch(%q): %v", tt.query, err)
			}
			slugs := make([]string, 0, len(got))
			for _, r := range got {
				slugs = append(slugs, r.NoteSlug)
				if r.Lexical != true {
					t.Errorf("result %s missing Lexical marker", r.NoteSlug)
				}
			}
			if len(slugs) != len(tt.want) {
				t.Fatalf("LexicalSearch(%q) = %v, want %v", tt.query, slugs, tt.want)
			}
			for i := range tt.want {
				if slugs[i] != tt.want[i] {
					t.Errorf("LexicalSearch(%q)[%d] = %q, want %q (all: %v)", tt.query, i, slugs[i], tt.want[i], slugs)
				}
			}
		})
	}
}

func TestLexicalSearchRespectsWhereFilter(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	got, err := st.LexicalSearch(ctx, "note", 10, chroma.NinString("note_slug", "note-b"))
	if err != nil {
		t.Fatalf("LexicalSearch: %v", err)
	}
	if len(got) != 1 || got[0].NoteSlug != "note-a" {
		t.Errorf("LexicalSearch with exclude = %v, want [note-a]", got)
	}
}

func TestLexicalSearchLeanRows(t *testing.T) {
	ctx, st, _ := setupStoreTest(t)

	got, err := st.LexicalSearch(ctx, "golang", 10, nil)
	if err != nil {
		t.Fatalf("LexicalSearch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	r := got[0]
	if r.NoteSlug != "note-a" || r.Title != "Note A" {
		t.Errorf("unexpected row: %+v", r)
	}
	if r.Score != 0 {
		t.Errorf("lexical rows must have Score 0, got %v", r.Score)
	}
	if r.Text != "" || len(r.Context) != 0 {
		t.Errorf("lexical rows must be lean (no text/context), got text=%q context=%v", r.Text, r.Context)
	}
}
