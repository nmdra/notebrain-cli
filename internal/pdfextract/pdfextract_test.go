package pdfextract

import (
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/pdf2md"
)

type mockPDFBackend struct {
	pages []string
}

func (m *mockPDFBackend) ExtractText(ctx context.Context, filePath string) ([]string, error) {
	return m.pages, nil
}

func (m *mockPDFBackend) ExtractStructured(ctx context.Context, filePath string) ([][]pdf2md.TextRect, error) {
	return nil, nil
}

func (m *mockPDFBackend) RenderPage(ctx context.Context, filePath string, pageNum int) (string, error) {
	return "mock-image.png", nil
}

func (m *mockPDFBackend) Close() error { return nil }

type mockOCRBackend struct {
	available bool
	text      string
}

func (m *mockOCRBackend) Available() bool                        { return m.available }
func (m *mockOCRBackend) ValidateLang(ctx context.Context) error { return nil }
func (m *mockOCRBackend) OCRPage(ctx context.Context, imagePath string) (string, error) {
	return m.text, nil
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name       string
		pdfPages   []string
		ocrEnabled bool
		ocrText    string
		want       []string
	}{
		{
			name:       "dense text page, no OCR needed",
			pdfPages:   []string{strings.Repeat("a", 100)},
			ocrEnabled: true,
			ocrText:    "ocr output",
			want:       []string{strings.Repeat("a", 100)}, // OCR shouldn't be called
		},
		{
			name:       "sparse text page, OCR disabled",
			pdfPages:   []string{"short"},
			ocrEnabled: false,
			ocrText:    "ocr output",
			want:       []string{"short"}, // OCR is disabled, return sparse
		},
		{
			name:       "sparse text page, OCR enabled",
			pdfPages:   []string{"short"},
			ocrEnabled: true,
			ocrText:    "long ocr output that replaces sparse text",
			want:       []string{"long ocr output that replaces sparse text"}, // OCR should replace
		},
		{
			name:       "sparse text page, OCR returns less text",
			pdfPages:   []string{"short"},
			ocrEnabled: true,
			ocrText:    "bad",
			want:       []string{"short"}, // OCR text is shorter, keep original
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdfBackend := &mockPDFBackend{pages: tt.pdfPages}
			var ocrBackend OCRBackend
			if tt.ocrEnabled {
				ocrBackend = &mockOCRBackend{available: true, text: tt.ocrText}
			}

			pages, err := Extract(context.Background(), pdfBackend, ocrBackend, "dummy.pdf")
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			if len(pages) != len(tt.want) {
				t.Fatalf("Extract() got %d pages, want %d", len(pages), len(tt.want))
			}

			for i := range pages {
				if pages[i] != tt.want[i] {
					t.Errorf("Extract() page %d = %q, want %q", i, pages[i], tt.want[i])
				}
			}
		})
	}
}
