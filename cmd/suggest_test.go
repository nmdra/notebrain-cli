package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/ingest"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type suggestTestEmbedder struct{}

func (suggestTestEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}

func (suggestTestEmbedder) Model() string { return "suggest-test" }

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// seededSuggestStore ingests two notes into a temp store and closes it,
// leaving the DB ready for SuggestNotesCmd to open on its own.
func seededSuggestStore(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	files := map[string]string{
		"zeta-note.md": "Zeta note content with several meaningful words here.",
		"alpha.md":     "Alpha note content with several meaningful words here.",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(vaultDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	p := ingest.NewPipeline(st, suggestTestEmbedder{}, 2)
	p.MinChunkWords = 0
	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}
	return dbDir
}

func TestSuggestNotesListsSlugsSorted(t *testing.T) {
	dbDir := seededSuggestStore(t)
	got := captureStdout(t, func() {
		globals := &Globals{Ctx: context.Background(), ChromaPath: dbDir}
		if err := (&SuggestNotesCmd{}).Run(globals); err != nil {
			t.Fatalf("suggest-notes failed: %v", err)
		}
	})
	want := []string{"alpha", "zeta-note"}
	gotSlugs := strings.Fields(got)
	if strings.Join(gotSlugs, ",") != strings.Join(want, ",") {
		t.Errorf("got slugs %v, want %v", gotSlugs, want)
	}
}

func TestSuggestNotesEmptyStore(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	got := captureStdout(t, func() {
		globals := &Globals{Ctx: ctx, ChromaPath: dbDir}
		if err := (&SuggestNotesCmd{}).Run(globals); err != nil {
			t.Fatalf("suggest-notes on empty store failed: %v", err)
		}
	})
	if got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}
