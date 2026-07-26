package pdfextract

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFiumBackend_ExtractText(t *testing.T) {
	backend, err := NewPDFiumBackend()
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

func TestPDFiumBackend_RenderPage(t *testing.T) {
	backend, err := NewPDFiumBackend()
	if err != nil {
		t.Fatalf("Failed to initialize PDFium backend: %v", err)
	}
	defer backend.Close()

	testPDF := filepath.Join("testdata", "hello.pdf")

	imgPath, err := backend.RenderPage(context.Background(), testPDF, 1)
	if err != nil {
		t.Fatalf("RenderPage() failed: %v", err)
	}
	defer os.Remove(imgPath)

	info, err := os.Stat(imgPath)
	if err != nil {
		t.Fatalf("Failed to stat rendered image: %v", err)
	}

	if info.Size() == 0 {
		t.Errorf("Rendered image size is 0")
	}
}
