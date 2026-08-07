package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"

	"github.com/nmdra/notebrain-cli/v2/internal/ingest"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestResolveQueries(t *testing.T) {
	tests := []struct {
		name    string
		queries []string
		want    []string
	}{
		{
			name:    "single query",
			queries: []string{"redis"},
			want:    []string{"redis"},
		},
		{
			name:    "multi positional queries",
			queries: []string{"redis", "message broker"},
			want:    []string{"redis", "message broker"},
		},
		{
			name:    "deduplication and whitespace trimming",
			queries: []string{"  redis ", "redis", "message broker"},
			want:    []string{"redis", "message broker"},
		},
		{
			name:    "empty strings ignored",
			queries: []string{"", "  "},
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveQueries(tt.queries)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolveQueries() = %v, want %v", got, tt.want)
			}
		})
	}
}

// searchTestEmbedder returns deterministic unit vectors, one per note.
type searchTestEmbedder struct{}

func (searchTestEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}

func (searchTestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 384)
	}
	return out, nil
}

func (searchTestEmbedder) Close() error { return nil }

func (searchTestEmbedder) Model() string { return "search-test" }

// newSearchTestStore ingests three notes into a temp store and returns the
// store plus the path to its chroma dir for globals.
func newSearchTestStore(t *testing.T, ctx context.Context) (*store.Store, string) {
	t.Helper()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	vaultDir := t.TempDir()
	files := map[string]string{
		"zeta-note.md":  "Zeta note content about redis queues.",
		"alpha.md":      "Alpha note content about redis queues.",
		"beta.md":       "Beta note content about redis queues.",
		"omega-note.md": "Omega note content about redis queues.",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(vaultDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	p := ingest.NewPipeline(st, searchTestEmbedder{}, 2)
	p.MinChunkWords = 0
	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("pipeline run: %v", err)
	}
	return st, dbDir
}

func TestResolveExcludes(t *testing.T) {
	ctx := context.Background()
	st, _ := newSearchTestStore(t, ctx)

	slugOf := func(input string) string {
		slug, err := st.ResolveNoteSlug(ctx, input)
		if err != nil {
			t.Fatalf("resolve %q: %v", input, err)
		}
		return slug
	}

	t.Run("empty and whitespace values ignored", func(t *testing.T) {
		c := &SearchCmd{ExcludeNotes: []string{"", "   ", "zeta-note.md"}}
		got, err := c.resolveExcludes(ctx, st)
		if err != nil {
			t.Fatalf("resolveExcludes: %v", err)
		}
		want := []string{slugOf("zeta-note.md")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("resolveExcludes() = %v, want %v", got, want)
		}
	})

	t.Run("deduplicates resolved slugs across title and slug forms", func(t *testing.T) {
		slug := slugOf("alpha.md")
		c := &SearchCmd{ExcludeNotes: []string{slug, "alpha.md", "Alpha"}}
		got, err := c.resolveExcludes(ctx, st)
		if err != nil {
			t.Fatalf("resolveExcludes: %v", err)
		}
		if !reflect.DeepEqual(got, []string{slug}) {
			t.Errorf("resolveExcludes() = %v, want %v", got, []string{slug})
		}
	})

	t.Run("unknown note skipped with no error", func(t *testing.T) {
		c := &SearchCmd{ExcludeNotes: []string{"no-such-note", "zeta-note.md"}}
		got, err := c.resolveExcludes(ctx, st)
		if err != nil {
			t.Fatalf("resolveExcludes: %v", err)
		}
		if !reflect.DeepEqual(got, []string{slugOf("zeta-note.md")}) {
			t.Errorf("resolveExcludes() = %v", got)
		}
	})

	t.Run("ambiguous value is a usage error", func(t *testing.T) {
		// Both zeta-note.md and omega-note.md end with "note", so the
		// suffix match is ambiguous and resolution must fail.
		c := &SearchCmd{ExcludeNotes: []string{"note"}}
		_, err := c.resolveExcludes(ctx, st)
		var usageErr *UsageError
		if !errors.As(err, &usageErr) {
			t.Fatalf("expected UsageError, got %v", err)
		}
		if !strings.Contains(err.Error(), "multiple") {
			t.Errorf("expected ambiguity message, got %q", err)
		}
	})

	t.Run("no excludes returns nil", func(t *testing.T) {
		c := &SearchCmd{}
		got, err := c.resolveExcludes(ctx, st)
		if err != nil {
			t.Fatalf("resolveExcludes: %v", err)
		}
		if got != nil {
			t.Errorf("resolveExcludes() = %v, want nil", got)
		}
	})
}

func TestBuildWhereFilter_ExcludeNotes(t *testing.T) {
	// filterJSON renders a WhereFilter the same way it reaches the embedded
	// engine: {"field": {"$op": ...}} or {"$and": [...]}.
	filterJSON := func(t *testing.T, w chroma.WhereFilter) map[string]any {
		t.Helper()
		b, err := json.Marshal(w)
		if err != nil {
			t.Fatalf("marshal filter: %v", err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal filter %s: %v", b, err)
		}
		return m
	}

	t.Run("no excludes leaves filter unchanged", func(t *testing.T) {
		if got := (&store.SearchFilter{IncludePDF: true}).Build(); got != nil {
			t.Errorf("expected nil filter, got %v", got)
		}
		if got := (&store.SearchFilter{}).Build(); got == nil {
			t.Error("expected file_type filter, got nil")
		}
	})

	t.Run("excludes add a NinString clause", func(t *testing.T) {
		m := filterJSON(t, (&store.SearchFilter{IncludePDF: true, Exclude: []string{"alpha", "beta"}}).Build())
		nin, ok := m["note_slug"].(map[string]any)["$nin"].([]any)
		if !ok {
			t.Fatalf("expected $nin clause on note_slug, got %v", m)
		}
		if len(nin) != 2 {
			t.Errorf("expected 2 excluded slugs, got %v", nin)
		}
	})

	t.Run("excludes combine with tag and section filters", func(t *testing.T) {
		m := filterJSON(t, (&store.SearchFilter{
			Section:     "Architecture > Components",
			Tag:         "kubernetes",
			ResolveTags: true,
			Exclude:     []string{"alpha"},
		}).Build())
		and, ok := m["$and"].([]any)
		if !ok {
			t.Fatalf("expected AND-combined filter, got %v", m)
		}
		// section + file_type md + tag OR + $nin
		if len(and) != 4 {
			t.Errorf("expected 4 AND clauses, got %d: %v", len(and), m)
		}
	})
}
