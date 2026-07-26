package pdfextract

import (
	"context"
	"log/slog"
	"os"
	"unicode"
)

const minTextDensity = 50 // Minimum characters to consider a page "text-dense"

// Extract processes a PDF file and returns its extracted text.
// If an OCRBackend is provided and a page is sparse, it will fall back to OCR.
func Extract(ctx context.Context, pdfBackend PDFBackend, ocrBackend OCRBackend, filePath string) ([]string, error) {
	pages, err := pdfBackend.ExtractText(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if ocrBackend != nil {
		for i, pageText := range pages {
			if textDensity(pageText) < minTextDensity {
				img, renderErr := pdfBackend.RenderPage(ctx, filePath, i+1)
				if renderErr != nil {
					slog.Warn("pdf: failed to render page for OCR, skipping",
						"file", filePath, "page", i+1, "err", renderErr)
					continue
				}

				ocrText, ocrErr := runOCRWithCleanup(ctx, ocrBackend, img)

				if ocrErr != nil {
					slog.Warn("pdf: OCR failed for page, using sparse text",
						"file", filePath, "page", i+1, "err", ocrErr)
					continue // keep original sparse pageText
				}

				if len(ocrText) > len(pageText) {
					pages[i] = ocrText
					slog.Debug("pdf: OCR extracted text for sparse page",
						"file", filePath, "page", i+1, "chars", len(ocrText))
				}
			}
		}
	} else {
		for i, pageText := range pages {
			if textDensity(pageText) < minTextDensity {
				slog.Debug("pdf: sparse page detected, OCR not enabled",
					"file", filePath, "page", i+1, "chars", len(pageText))
			}
		}
	}

	return pages, nil
}

// textDensity returns the number of non-whitespace characters in a string.
func textDensity(text string) int {
	var count int
	for _, r := range text {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

func runOCRWithCleanup(ctx context.Context, ocrBackend OCRBackend, img string) (string, error) {
	defer os.Remove(img)
	return ocrBackend.OCRPage(ctx, img)
}
