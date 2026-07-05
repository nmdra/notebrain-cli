package ingest

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
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

func TestPipelineMinChunkWords(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// note1 has 2 words (should be skipped if MinChunkWords = 5)
	// note2 has 8 words (should be kept if MinChunkWords = 5)
	_ = os.WriteFile(filepath.Join(vaultDir, "note1.md"), []byte("short note"), 0644)
	_ = os.WriteFile(filepath.Join(vaultDir, "note2.md"), []byte("this is a longer note containing several words"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.MinChunkWords = 5

	pr, pw := io.Pipe()
	go func() { _ = pw.Close() }()
	var stdout bytes.Buffer
	if err := p.Run(ctx, vaultDir, "", pr, &stdout); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 1 {
		t.Errorf("Expected 1 chunk (only note2), got %d", stats["chunks"])
	}
}

func TestPipelineRespectExclude(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	obsidianDir := filepath.Join(vaultDir, ".obsidian")
	_ = os.MkdirAll(obsidianDir, 0755)
	appJSON := []byte(`{"userIgnoreFilters": ["Archive"], "attachmentFolderPath": "99.Storage-Shed/Attachments"}`)
	_ = os.WriteFile(filepath.Join(obsidianDir, "app.json"), appJSON, 0644)

	_ = os.WriteFile(filepath.Join(vaultDir, "active.md"), []byte("Active note content"), 0644)
	_ = os.MkdirAll(filepath.Join(vaultDir, "Archive"), 0755)
	_ = os.WriteFile(filepath.Join(vaultDir, "Archive", "old.md"), []byte("Old note content"), 0644)
	_ = os.MkdirAll(filepath.Join(vaultDir, "99.Storage-Shed", "Attachments"), 0755)
	_ = os.WriteFile(filepath.Join(vaultDir, "99.Storage-Shed", "Attachments", "attachment.md"), []byte("Attachment note"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.RespectExclude = true

	pr, pw := io.Pipe()
	go func() { _ = pw.Close() }()
	var stdout bytes.Buffer
	if err := p.Run(ctx, vaultDir, "", pr, &stdout); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 1 {
		t.Errorf("Expected 1 chunk (only active.md), got %d", stats["chunks"])
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"short", "hello world"},
		{"medium", strings.Repeat("word ", 50)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got < 0 {
				t.Errorf("estimateTokens(%q) = %d, want >= 0", tt.text, got)
			}
			if tt.text != "" && got < 1 {
				t.Errorf("estimateTokens(%q) = %d, want >= 1 for non-empty string", tt.text, got)
			}
		})
	}
}

func TestBuildEmbedText_TruncationGuard(t *testing.T) {
	longTitle := strings.Repeat("Architecture ", 20) // ~260 chars
	longHeading := strings.Repeat("SubSection > ", 15)
	tags := []string{"tag1", "tag2", "tag3", "tag4", "tag5"}
	body := "This is the chunk body that must be preserved."

	tests := []struct {
		name      string
		title     string
		heading   string
		tags      []string
		body      string
		maxTokens int
		wantTitle bool
		wantTags  bool
	}{
		{"normal fit", "My Note", "Section A", tags, body, 256, true, true},
		{"long title drops tags", longTitle, "Sec", tags, body, 80, true, false},
		{"very long prefix drops all", longTitle, longHeading, tags, body, 60, false, false},
		{"empty prefix", "", "", nil, body, 256, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEmbedText(tt.title, tt.heading, tt.tags, tt.body, tt.maxTokens)
			if !strings.Contains(result, tt.body) {
				t.Errorf("body text missing from embed text: got %q", result)
			}
			if tt.wantTags && !strings.Contains(result, "[tags:") {
				t.Errorf("expected tags in embed text: got %q", result)
			}
			if !tt.wantTags && strings.Contains(result, "[tags:") {
				t.Errorf("expected tags to be trimmed: got %q", result)
			}
		})
	}
}

func TestPipeline_CodeOnlyNoteIngest(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	codeNote := "---\ntitle: Code Snippet\n---\n# Helper Function\n\n```go\nfunc add(a, b int) int {\n    return a + b\n}\n```\n"
	_ = os.WriteFile(filepath.Join(vaultDir, "code.md"), []byte(codeNote), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	pr, pw := io.Pipe()
	go func() { _ = pw.Close() }()
	var stdout bytes.Buffer
	if err := p.Run(ctx, vaultDir, "", pr, &stdout); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats["chunks"] != 1 {
		t.Errorf("Expected 1 chunk for code-only note, got %d", stats["chunks"])
	}
}

func BenchmarkEstimateTokens(b *testing.B) {
	text := strings.Repeat("This is a test sentence for token estimation in NoteBrain. ", 50)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = estimateTokens(text)
	}
}

func BenchmarkBuildEmbedText(b *testing.B) {
	title := "System Architecture and Data Flow"
	heading := "Internal Components > Ingestion Pipeline"
	tags := []string{"architecture", "golang", "chromadb", "embeddings"}
	body := strings.Repeat("The ingestion pipeline tokenizes markdown notes and stores them into ChromaDB vectors. ", 10)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = buildEmbedText(title, heading, tags, body, 256)
	}
}
