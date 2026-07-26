package implementation_webassembly

import (
	"github.com/google/uuid"
	"github.com/klippa-app/go-pdfium/references"
)

func (p *PdfiumImplementation) registerPathSegment(pathSegment *uint64) *PathSegmentHandle {
	ref := uuid.New()
	handle := &PathSegmentHandle{
		handle:    pathSegment,
		nativeRef: references.FPDF_PATHSEGMENT(ref.String()),
	}

	p.pathSegmentRefs[handle.nativeRef] = handle

	return handle
}
