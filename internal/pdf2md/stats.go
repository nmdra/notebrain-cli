package pdf2md

import (
	"math"
	"sort"
	"strings"
)

// AnalyzeDocument computes document-wide font/layout statistics.
func AnalyzeDocument(allPages [][]Line) DocumentStats {
	var stats DocumentStats
	if len(allPages) == 0 {
		return stats
	}

	sizeCounts := make(map[float64]int)
	nameCounts := make(map[string]int)
	var allGaps []float64

	for _, page := range allPages {
		for i, line := range page {
			// Font size & name stats
			for _, rect := range line.Rects {
				if strings.TrimSpace(rect.Text) == "" {
					continue
				}
				// Round font size to nearest 0.5 to group similar sizes
				roundedSize := math.Round(rect.FontSize*2) / 2
				sizeCounts[roundedSize]++

				// We want the font name of the most common size, but for now we just
				// take the overall most common font name.
				nameCounts[rect.FontName]++
			}

			// Line gap stats (gap between this line and the next on the same page)
			if i+1 < len(page) {
				nextLine := page[i+1]
				gap := line.Bottom - nextLine.Top
				if gap > 0 { // Ignore overlapping lines or negative gaps
					allGaps = append(allGaps, gap)
				}
			}
		}
	}

	// Find body font size (mode). Tie-breaker: smaller font size is preferred for body text.
	maxCount := 0
	for size, count := range sizeCounts {
		if count > maxCount {
			maxCount = count
			stats.BodyFontSize = size
		} else if count == maxCount && size < stats.BodyFontSize {
			stats.BodyFontSize = size
		}
	}

	// Find body font name (mode)
	maxNameCount := 0
	for name, count := range nameCounts {
		if count > maxNameCount {
			maxNameCount = count
			stats.BodyFontName = name
		}
	}

	// Find median line gap
	if len(allGaps) > 0 {
		sort.Float64s(allGaps)
		mid := len(allGaps) / 2
		if len(allGaps)%2 == 0 {
			stats.MedianLineGap = (allGaps[mid-1] + allGaps[mid]) / 2.0
		} else {
			stats.MedianLineGap = allGaps[mid]
		}
	} else {
		// Fallback reasonable gap if no consecutive lines exist
		stats.MedianLineGap = stats.BodyFontSize * 0.2
	}

	return stats
}
