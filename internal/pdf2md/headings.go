package pdf2md

import "strings"

// ClassifyHeadings evaluates lines and sets their HeadingLevel based on font size and weight.
func ClassifyHeadings(pages [][]Line, stats DocumentStats) {
	if stats.BodyFontSize == 0 {
		return // Cannot classify without a baseline
	}

	for i := range pages {
		for j := range pages[i] {
			line := &pages[i][j] //nolint:gosec // safe due to range
			text := strings.TrimSpace(line.FullText())
			if text == "" {
				continue
			}

			maxSize := line.MaxFontSize()
			ratio := maxSize / stats.BodyFontSize

			switch {
			case ratio >= 1.8:
				line.HeadingLevel = 1
			case ratio >= 1.4:
				line.HeadingLevel = 2
			case ratio >= 1.15:
				line.HeadingLevel = 3
			case ratio >= 0.95 && ratio <= 1.05 && line.IsBold() && len(text) < 80:
				// Bold body-size text that is short is often a subheading
				// We must ensure it's not just a bold sentence in a paragraph.
				// A simple heuristic is length.
				line.HeadingLevel = 3
			}
		}
	}
}
