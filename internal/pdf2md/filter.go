package pdf2md

import (
	"regexp"
	"strings"
)

var pageNumberPattern = regexp.MustCompile(`^(?i)(?:page\s*)?\d+(?:\s*of\s*\d+)?$`)

// FilterNoise removes headers, footers, page numbers, and meaningless rects.
func FilterNoise(pages [][]Line, stats DocumentStats) [][]Line {
	if len(pages) == 0 {
		return pages
	}

	// 1. Identify running headers and footers
	// We'll look at the first few and last few lines of each page.
	headerCounts := make(map[string]int)
	footerCounts := make(map[string]int)

	for _, page := range pages {
		if len(page) == 0 {
			continue
		}
		// Consider top 3 lines as potential headers
		for i := 0; i < len(page) && i < 3; i++ {
			text := strings.TrimSpace(page[i].FullText())
			if text != "" {
				headerCounts[text]++
			}
		}
		// Consider bottom 3 lines as potential footers
		for i := len(page) - 1; i >= 0 && i >= len(page)-3; i-- {
			text := strings.TrimSpace(page[i].FullText())
			if text != "" {
				footerCounts[text]++
			}
		}
	}

	threshold := max(int(float64(len(pages))*0.7),
		// Need at least 2 occurrences if few pages
		2)

	var cleanedPages [][]Line
	for _, page := range pages {
		var cleanedPage []Line
		for i, line := range page {
			text := strings.TrimSpace(line.FullText())
			if text == "" {
				continue
			}

			// Filter micro-text
			if line.MaxFontSize() > 0 && line.MaxFontSize() < 5.0 {
				continue
			}

			// Filter giant text (watermarks)
			if stats.BodyFontSize > 0 && line.MaxFontSize() > stats.BodyFontSize*3.0 {
				// Only filter if it's very short, as it might be a massive title otherwise
				if len(text) < 20 {
					continue
				}
			}

			isTopRegion := i < 3
			isBottomRegion := i >= len(page)-3

			// Filter running headers
			if isTopRegion && headerCounts[text] >= threshold {
				continue
			}

			// Filter running footers
			if isBottomRegion && footerCounts[text] >= threshold {
				continue
			}

			// Filter page numbers
			if (isTopRegion || isBottomRegion) && pageNumberPattern.MatchString(text) {
				continue
			}

			cleanedPage = append(cleanedPage, line)
		}
		cleanedPages = append(cleanedPages, cleanedPage)
	}

	return cleanedPages
}
