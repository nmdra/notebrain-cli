package pdfextract

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTesseractBackend_Available(t *testing.T) {
	// Create a mock tesseract binary
	tempDir := t.TempDir()
	mockBin := filepath.Join(tempDir, "mock-tesseract")

	// Create a dummy executable
	if err := os.WriteFile(mockBin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to create mock binary: %v", err)
	}

	backend := NewTesseractBackend(mockBin, "eng")
	if !backend.Available() {
		t.Errorf("expected mock tesseract to be available")
	}

	// Test with non-existent binary
	badBackend := NewTesseractBackend(filepath.Join(tempDir, "does-not-exist"), "eng")
	if badBackend.Available() {
		t.Errorf("expected non-existent tesseract to be unavailable")
	}
}

func TestTesseractBackend_ValidateLang(t *testing.T) {
	// Create a mock tesseract that outputs specific languages
	tempDir := t.TempDir()
	mockBin := filepath.Join(tempDir, "mock-tesseract")

	// We need a script that prints to stdout when given --list-langs
	script := `#!/bin/sh
if [ "$1" = "--list-langs" ]; then
	echo "List of available languages (3):"
	echo "eng"
	echo "osd"
	echo "fra"
	exit 0
fi
exit 1
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock binary: %v", err)
	}

	tests := []struct {
		name    string
		lang    string
		wantErr bool
	}{
		{"Single existing lang", "eng", false},
		{"Multiple existing langs", "eng+fra", false},
		{"Missing lang", "deu", true},
		{"One missing, one existing", "eng+deu", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewTesseractBackend(mockBin, tt.lang)
			err := backend.ValidateLang(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLang() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTesseractBackend_OCRPage(t *testing.T) {
	tempDir := t.TempDir()
	mockBin := filepath.Join(tempDir, "mock-tesseract")

	// Script that simulates OCR output
	script := `#!/bin/sh
# $1 is image, $2 is stdout
if [ "$2" = "stdout" ]; then
	echo "Mocked OCR text"
	exit 0
fi
exit 1
`
	if err := os.WriteFile(mockBin, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock binary: %v", err)
	}

	// Create a dummy image file
	dummyImg := filepath.Join(tempDir, "dummy.png")
	if err := os.WriteFile(dummyImg, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("failed to create dummy image: %v", err)
	}

	backend := NewTesseractBackend(mockBin, "eng")
	text, err := backend.OCRPage(context.Background(), dummyImg)
	if err != nil {
		t.Fatalf("OCRPage() unexpected error: %v", err)
	}

	if text != "Mocked OCR text\n" {
		t.Errorf("OCRPage() got = %q, want %q", text, "Mocked OCR text\n")
	}
}
