package ingest

import (
	"bytes"
	"context"
	"io"
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

	// Use an io.Pipe for stdin so Bubble Tea doesn't immediately exit due to EOF
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	var stdout bytes.Buffer
	err = p.Run(ctx, vaultDir, "", pr, &stdout)
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

func TestPipelineSyncDeleted(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// 1. Write note1.md and note2.md
	n1Path := filepath.Join(vaultDir, "note1.md")
	n2Path := filepath.Join(vaultDir, "note2.md")
	_ = os.WriteFile(n1Path, []byte("Note one [[note2]]"), 0644)
	_ = os.WriteFile(n2Path, []byte("Note two [[note1]]"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)

	// Ingest initially
	pr1, pw1 := io.Pipe()
	go func() { _ = pw1.Close() }()
	var stdout1 bytes.Buffer
	if err := p.Run(ctx, vaultDir, "", pr1, &stdout1); err != nil {
		t.Fatalf("First run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 2 || stats["links"] != 2 {
		t.Fatalf("Expected 2 chunks, 2 links initially, got %v", stats)
	}

	// 2. Delete note2.md on disk
	if err := os.Remove(n2Path); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	// Ingest again
	pr2, pw2 := io.Pipe()
	go func() { _ = pw2.Close() }()
	var stdout2 bytes.Buffer
	if err := p.Run(ctx, vaultDir, "", pr2, &stdout2); err != nil {
		t.Fatalf("Second run failed: %v", err)
	}

	// 3. Verify that note2 has been cleaned up (only 1 chunk and 1 link for note1 remain)
	stats, _ = st.Stats(ctx)
	if stats["chunks"] != 1 {
		t.Errorf("Expected 1 chunk remaining after sync, got %d", stats["chunks"])
	}
	if stats["links"] != 1 {
		t.Errorf("Expected 1 link remaining after sync, got %d", stats["links"])
	}
}
