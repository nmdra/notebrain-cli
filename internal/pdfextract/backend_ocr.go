package pdfextract

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TesseractBackend implements OCRBackend using the tesseract CLI tool.
type TesseractBackend struct {
	BinPath string
	Lang    string
}

// NewTesseractBackend creates a new TesseractBackend.
func NewTesseractBackend(binPath, lang string) *TesseractBackend {
	return &TesseractBackend{
		BinPath: binPath,
		Lang:    lang,
	}
}

// Available returns true if the tesseract binary is executable.
func (b *TesseractBackend) Available() bool {
	_, err := exec.LookPath(b.BinPath)
	return err == nil
}

// ValidateLang checks that the configured language data is available.
func (b *TesseractBackend) ValidateLang(ctx context.Context) error {
	//nolint:gosec // BinPath is controlled internally
	cmd := exec.CommandContext(ctx, b.BinPath, "--list-langs")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tesseract --list-langs failed: %w", err)
	}

	// Check each requested language is in the output
	for lang := range strings.SplitSeq(b.Lang, "+") {
		if !strings.Contains(string(out), lang) {
			return fmt.Errorf(
				"language %q not found in tesseract data (available: %s)",
				lang, strings.TrimSpace(string(out)),
			)
		}
	}
	return nil
}

// OCRPage performs OCR on the specified image file and returns the extracted text.
func (b *TesseractBackend) OCRPage(ctx context.Context, imagePath string) (string, error) {
	// tesseract <image> stdout -l <lang>
	//nolint:gosec // Inputs are controlled by the pipeline
	cmd := exec.CommandContext(ctx, b.BinPath, imagePath, "stdout", "-l", b.Lang)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tesseract OCR failed: %w", err)
	}
	return string(out), nil
}
