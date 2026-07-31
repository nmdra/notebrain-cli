package pdfextract

import (
	"context"
	"fmt"
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
