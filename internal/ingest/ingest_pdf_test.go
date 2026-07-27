package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type mockPDFBackend struct{}

func (m *mockPDFBackend) ExtractText(ctx context.Context, filePath string) ([]string, error) {
	return []string{"This is a test PDF page with enough words to be extracted properly."}, nil
}

func (m *mockPDFBackend) RenderPage(ctx context.Context, filePath string, pageNum int) (string, error) {
	return "", nil
}

func (m *mockPDFBackend) Close() error { return nil }

type mockLLMConverter struct{}

func (m *mockLLMConverter) Convert(ctx context.Context, pages []string) (string, error) {
	return "# Dummy PDF Page\n\nThis is a test PDF page with enough words to be extracted properly.", nil
}
func (m *mockLLMConverter) Name() string { return "mock" }

func TestProcessPdfFile(t *testing.T) {
	// Create a temp dummy PDF
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "dummy.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		embedder:      &mockEmbedder{},
		pdfBackend:    &mockPDFBackend{},
		llmConverter:  &mockLLMConverter{},
		MinChunkWords: 2,
		ChunkSize:     800,
		ChunkOverlap:  100,
	}

	knownHashes := make(map[string]string)
	res, err := p.processPdfFile(context.Background(), dir, pdfPath, knownHashes)
	if err != nil {
		t.Fatalf("processPdfFile failed: %v", err)
	}

	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if res.NoteSlug != "dummy-pdf" {
		t.Errorf("expected slug 'dummy-pdf', got %q", res.NoteSlug)
	}

	if len(res.ChunkRecords) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(res.ChunkRecords))
	}

	chunk := res.ChunkRecords[0]
	if chunk.FileType != "pdf" {
		t.Errorf("expected FileType 'pdf', got %q", chunk.FileType)
	}
	if chunk.HeadingPath != "Dummy PDF Page" {
		t.Errorf("expected HeadingPath 'Dummy PDF Page', got %q", chunk.HeadingPath)
	}
}

func TestProcessPdfFile_SkipUnchanged(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "dummy.pdf")
	content := []byte("%PDF-1.4 dummy")
	if err := os.WriteFile(pdfPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	importHashBytes := []byte("%PDF-1.4 dummy")
	importHash := fmt.Sprintf("%x", sha256.Sum256(importHashBytes))

	p := &Pipeline{
		embedder:     &mockEmbedder{},
		pdfBackend:   &mockPDFBackend{},
		llmConverter: &mockLLMConverter{},
	}

	knownHashes := map[string]string{
		"dummy-pdf": importHash,
	}

	res, err := p.processPdfFile(context.Background(), dir, pdfPath, knownHashes)
	if err != nil {
		t.Fatalf("processPdfFile failed: %v", err)
	}
	if res != nil {
		t.Fatal("expected nil result for unchanged file")
	}
}
