package pdfextract

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPDFiumBackend_ExtractText(t *testing.T) {
	backend, err := NewPDFiumBackend(1)
	if err != nil {
		t.Fatalf("Failed to initialize PDFium backend: %v", err)
	}
	defer backend.Close()

	testPDF := filepath.Join("testdata", "hello.pdf")

	pages, err := backend.ExtractText(context.Background(), testPDF)
	if err != nil {
		t.Fatalf("ExtractText() failed: %v", err)
	}

	if len(pages) != 1 {
		t.Errorf("ExtractText() returned %d pages, want 1", len(pages))
	}

	if len(pages) > 0 && pages[0] != "Hello World" {
		t.Errorf("ExtractText() got text %q, want %q", pages[0], "Hello World")
	}
}
