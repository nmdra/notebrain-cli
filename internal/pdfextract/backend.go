package pdfextract

import (
	"context"
)

// PDFBackend is the interface for extracting text from PDF files.
type PDFBackend interface {
	// ExtractText extracts text from the PDF file, returning a slice where each element is a page's text.
	ExtractText(ctx context.Context, filePath string) ([]string, error)
	// Close releases any resources associated with the backend.
	Close() error
}
