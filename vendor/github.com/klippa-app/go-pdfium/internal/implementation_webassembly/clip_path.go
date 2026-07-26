package implementation_webassembly

import (
	"github.com/google/uuid"
	"github.com/klippa-app/go-pdfium/references"
)

func (p *PdfiumImplementation) registerClipPath(clipPath *uint64) *ClipPathHandle {
	ref := uuid.New()
	handle := &ClipPathHandle{
		handle:    clipPath,
		nativeRef: references.FPDF_CLIPPATH(ref.String()),
	}

	p.clipPathRefs[handle.nativeRef] = handle

	return handle
}
