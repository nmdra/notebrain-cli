package implementation_webassembly

import (
	"github.com/google/uuid"
	"github.com/klippa-app/go-pdfium/references"
)

func (p *PdfiumImplementation) registerPageObjectMark(pageObjectMark *uint64) *PageObjectMarkHandle {
	ref := uuid.New()
	handle := &PageObjectMarkHandle{
		handle:    pageObjectMark,
		nativeRef: references.FPDF_PAGEOBJECTMARK(ref.String()),
	}

	p.pageObjectMarkRefs[handle.nativeRef] = handle

	return handle
}
