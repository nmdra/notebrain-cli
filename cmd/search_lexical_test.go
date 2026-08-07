package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestSearchLexicalFallbackOnEmptySemantic(t *testing.T) {
	fs := &fakeStore{lexical: []store.Result{
		{NoteSlug: "lecture-note", Title: "Lecture 5", FilePath: "Lecture 5.md", Lexical: true},
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatText}, fs, searchTestEmbedder{}, []string{"Lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, "Lexical Search") || !strings.Contains(out, "Lecture 5") {
		t.Errorf("expected lexical fallback output, got:\n%s", out)
	}
}

func TestSearchNoFallbackWhenSemanticHits(t *testing.T) {
	fs := &fakeStore{semantic: []store.Result{
		{NoteSlug: "sem-note", Title: "Semantic Note", Score: 0.85},
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatText}, fs, searchTestEmbedder{}, []string{"lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, "Semantic Search") || strings.Contains(out, "Lexical") {
		t.Errorf("expected semantic output without fallback, got:\n%s", out)
	}
}

func TestSearchLexicalFallbackWhenAllBelowMinScore(t *testing.T) {
	// Weak semantic rows that --min-score would drop must still trigger the
	// keyword fallback (the "Lecture" scenario under config min-score).
	fs := &fakeStore{
		semantic: []store.Result{{NoteSlug: "weak", Title: "Weak Match", Score: 0.3}},
		lexical:  []store.Result{{NoteSlug: "lecture-note", Title: "Lecture 5", FilePath: "Lecture 5.md", Lexical: true}},
	}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{ChunkDisplayFlags: ChunkDisplayFlags{MinScore: 0.4}}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatText}, fs, searchTestEmbedder{}, []string{"Lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, "Lexical Search") || !strings.Contains(out, "Lecture 5") {
		t.Errorf("expected lexical fallback for all-below-min-score results:\n%s", out)
	}
}

func TestSearchNoFallbackWhenAboveMinScore(t *testing.T) {
	fs := &fakeStore{semantic: []store.Result{
		{NoteSlug: "sem-note", Title: "Semantic Note", Score: 0.85},
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{ChunkDisplayFlags: ChunkDisplayFlags{MinScore: 0.4}}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatText}, fs, searchTestEmbedder{}, []string{"lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, "Semantic Search") || strings.Contains(out, "Lexical") {
		t.Errorf("expected semantic output without fallback, got:\n%s", out)
	}
}

func TestSearchLexicalJSONMarker(t *testing.T) {
	fs := &fakeStore{lexical: []store.Result{
		{NoteSlug: "lecture-note", Title: "Lecture 5", Lexical: true},
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatJSON}, fs, searchTestEmbedder{}, []string{"Lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, `"lexical": true`) {
		t.Errorf("expected lexical marker in JSON, got:\n%s", out)
	}
}

func TestSearchLexicalSkipsMinScoreFilter(t *testing.T) {
	fs := &fakeStore{lexical: []store.Result{
		{NoteSlug: "lecture-note", Title: "Lecture 5", Score: 0, Lexical: true},
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&SearchCmd{}).runStatic(context.Background(), &Globals{Ctx: context.Background(), Format: formatText}, fs, searchTestEmbedder{}, []string{"Lecture"}, nil, nil); err != nil {
			t.Errorf("runStatic: %v", err)
		}
	})
	if !strings.Contains(out, "Lecture 5") {
		t.Errorf("lexical row (Score 0) must survive min-score filtering:\n%s", out)
	}
}
