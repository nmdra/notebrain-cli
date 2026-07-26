package implementation_webassembly

import (
	"github.com/google/uuid"
	"github.com/klippa-app/go-pdfium/references"
)

func (p *PdfiumImplementation) registerBitmap(bitmap *uint64) *BitmapHandle {
	ref := uuid.New()
	handle := &BitmapHandle{
		handle:    bitmap,
		nativeRef: references.FPDF_BITMAP(ref.String()),
	}

	p.bitmapRefs[handle.nativeRef] = handle

	return handle
}
