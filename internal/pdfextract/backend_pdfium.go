package pdfextract

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// PDFiumBackend implements PDFBackend using go-pdfium via WASM.
type PDFiumBackend struct {
	pool pdfium.Pool
}

// NewPDFiumBackend initializes a new PDFium WebAssembly backend.
func NewPDFiumBackend() (*PDFiumBackend, error) {
	// Initialize the WebAssembly pool
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  1,
		MaxTotal: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("could not init PDFium WASM pool: %w", err)
	}

	return &PDFiumBackend{
		pool: pool,
	}, nil
}

// ExtractText returns the text of each page in the PDF document as a slice of strings.
func (b *PDFiumBackend) ExtractText(_ context.Context, filePath string) ([]string, error) {
	instance, err := b.pool.GetInstance(time.Second * 30)
	if err != nil {
		return nil, fmt.Errorf("could not get PDFium instance: %w", err)
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	pageCountReq, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{
		Document: doc.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	pages := make([]string, pageCountReq.PageCount)

	for i := 0; i < pageCountReq.PageCount; i++ {
		textReq, err := instance.GetPageText(&requests.GetPageText{
			Page: requests.Page{
				ByIndex: &requests.PageByIndex{
					Document: doc.Document,
					Index:    i,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to extract text from page %d: %w", i+1, err)
		}
		pages[i] = textReq.Text
	}

	return pages, nil
}

// RenderPage renders the specified page of a PDF document to a JPEG image file in a temporary directory.
func (b *PDFiumBackend) RenderPage(_ context.Context, filePath string, pageNum int) (string, error) {
	instance, err := b.pool.GetInstance(time.Second * 30)
	if err != nil {
		return "", fmt.Errorf("could not get PDFium instance: %w", err)
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &filePath,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	// Render page to image
	renderReq, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{
				Document: doc.Document,
				Index:    pageNum - 1,
			},
		},
		DPI: 300,
	})
	if err != nil {
		return "", fmt.Errorf("failed to render page %d: %w", pageNum, err)
	}

	// Create temp file for the image
	f, err := os.CreateTemp("", "notebrain-ocr-*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for image: %w", err)
	}
	defer f.Close()

	if renderReq.Result.Image != nil {
		if err := png.Encode(f, renderReq.Result.Image); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", fmt.Errorf("failed to encode png: %w", err)
		}
	} else {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("render returned nil image")
	}

	return f.Name(), nil
}

// Close releases the PDFium instance and pool.
func (b *PDFiumBackend) Close() error {
	var errs []error
	if b.pool != nil {
		if err := b.pool.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing PDFium: %v", errs)
	}
	return nil
}
