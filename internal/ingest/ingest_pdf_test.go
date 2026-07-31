package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockPDFBackend struct{}

func (m *mockPDFBackend) ExtractText(ctx context.Context, filePath string) ([]string, error) {
	return []string{"This is a test PDF page with enough words to be extracted properly."}, nil
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
	if chunk.FileType != fileTypePDF {
		t.Errorf("Expected FileType 'pdf', got %q", chunk.FileType)
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

	chunkSize, chunkOverlap := 800, 100
	importHash := fileHash([]byte("%PDF-1.4 dummy"), chunkSize, chunkOverlap)

	p := &Pipeline{
		embedder:     &mockEmbedder{},
		pdfBackend:   &mockPDFBackend{},
		llmConverter: &mockLLMConverter{},
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
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

type mockFailingLLMConverter struct{}

func (m *mockFailingLLMConverter) Convert(ctx context.Context, pages []string) (string, error) {
	return "", fmt.Errorf("simulated LLM conversion error")
}
func (m *mockFailingLLMConverter) Name() string { return "mock-fail" }

type mockEmptyLLMConverter struct{}

func (m *mockEmptyLLMConverter) Convert(ctx context.Context, pages []string) (string, error) {
	return "", nil
}
func (m *mockEmptyLLMConverter) Name() string { return "mock-empty" }

// TestProcessPdfFile_EmptyMarkdownSkips verifies that an empty LLM conversion
// result does not delete the previously indexed PDF.
func TestProcessPdfFile_EmptyMarkdownSkips(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "dummy.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		embedder:     &mockEmbedder{},
		pdfBackend:   &mockPDFBackend{},
		llmConverter: &mockEmptyLLMConverter{},
		ChunkSize:    800,
		ChunkOverlap: 100,
	}

	res, err := p.processPdfFile(context.Background(), dir, pdfPath, make(map[string]string))
	if err != nil {
		t.Fatalf("processPdfFile failed: %v", err)
	}
	if res != nil {
		t.Fatal("expected nil result for empty LLM conversion (note should be preserved)")
	}
}

func TestProcessPdfFile_FailureRecording(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "broken.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		embedder:     &mockEmbedder{},
		pdfBackend:   &mockPDFBackend{},
		llmConverter: &mockFailingLLMConverter{},
	}

	res, err := p.processPdfFile(context.Background(), dir, pdfPath, make(map[string]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Fatal("expected nil result on failure")
	}

	failed := p.FailedFiles()
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed file record, got %d", len(failed))
	}

	if failed[0].FilePath != "broken.pdf" {
		t.Errorf("expected FilePath 'broken.pdf', got %q", failed[0].FilePath)
	}
	if !strings.Contains(failed[0].Reason, "simulated LLM conversion error") {
		t.Errorf("expected reason to contain 'simulated LLM conversion error', got %q", failed[0].Reason)
	}
}
