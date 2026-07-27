package pdf2md

import "strings"

// Convert processes raw text rects from a PDF into a formatted Markdown string.
// It applies noise filtering, heading detection, and paragraph/list grouping.
func Convert(pages [][]TextRect) string {
	if len(pages) == 0 {
		return ""
	}

	// 1. Group raw rects into reading blocks via XY-cut, then into lines per page
	var pagesOfLines [][]Line
	for _, pageRects := range pages {
		blocks := XYCut(pageRects)
		var pageLines []Line
		for _, blockRects := range blocks {
			blockLines := BuildLines(blockRects, 3.0)
			pageLines = append(pageLines, blockLines...)
		}
		pagesOfLines = append(pagesOfLines, pageLines)
	}

	// 2. Analyze document to find baseline stats (body font size, line gaps)
	stats := AnalyzeDocument(pagesOfLines)

	// 3. Filter out running headers, footers, page numbers, etc.
	cleanedPages := FilterNoise(pagesOfLines, stats)

	// 4. Classify headings based on font size relative to body text
	ClassifyHeadings(cleanedPages, stats)

	// 5. Group lines into structural Markdown blocks (Headings, Paragraphs, Lists)
	blocks := GroupBlocks(cleanedPages, stats)

	// 6. Render blocks to Markdown
	var buf strings.Builder
	for _, block := range blocks {
		md := block.Markdown()
		if md != "" {
			buf.WriteString(md)
			buf.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(buf.String())
}
