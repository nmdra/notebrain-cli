package ingest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmdra/notebrain-cli/internal/store"
)

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0, 0.0, 0.0}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1.0, 0.0, 0.0}
	}
	return out, nil
}

func (m *mockEmbedder) Close() error { return nil }

func TestPipelineRun(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// Create some markdown files
	files := map[string]string{
		"note1.md":          "This is a note with a [[note2]] link and a #tag.",
		"note2.md":          "Another note. [[note1|backlink]]",
		"ignore.txt":        "Should be ignored",
		".hidden/hidden.md": "Should be ignored",
		"dir/nested.md":     "A nested markdown file",
	}

	for name, content := range files {
		path := filepath.Join(vaultDir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, []byte(content), 0644)
	}

	p := NewPipeline(st, &mockEmbedder{}, 2)

	var stdin, stdout bytes.Buffer
	err = p.Run(ctx, vaultDir, "", &stdin, &stdout)
	if err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 3 {
		t.Errorf("Expected 3 chunks (note1, note2, nested), got %d", stats["chunks"])
	}
	if stats["links"] != 2 {
		t.Errorf("Expected 2 links, got %d", stats["links"])
	}
}
