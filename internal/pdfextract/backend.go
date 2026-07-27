package pdfextract

import (
	"context"

	"github.com/nmdra/notebrain-cli/v2/internal/pdf2md"
)

// PDFBackend is the interface for extracting text and rendering pages from PDF files.
type PDFBackend interface {
	// ExtractText extracts text from the PDF file, returning a slice where each element is a page's text.
	ExtractText(ctx context.Context, filePath string) ([]string, error)
	// ExtractStructured extracts text with font and position data.
	ExtractStructured(ctx context.Context, filePath string) ([][]pdf2md.TextRect, error)
	// RenderPage renders the specified page (1-indexed) to a temporary image file and returns the path.
	RenderPage(ctx context.Context, filePath string, pageNum int) (string, error)
	// Close releases any resources associated with the backend.
	Close() error
}

// OCRBackend is the interface for performing OCR on images.
type OCRBackend interface {
	// Available returns true if the OCR backend is installed and executable.
	Available() bool
	// ValidateLang checks if the configured language data is available.
	ValidateLang(ctx context.Context) error
	// OCRPage performs OCR on the specified image file and returns the extracted text.
	OCRPage(ctx context.Context, imagePath string) (string, error)
}
