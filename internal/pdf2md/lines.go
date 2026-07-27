package pdf2md

import (
	"math"
	"sort"
)

// BuildLines groups TextRects into visual lines based on vertical position.
// Rects within `tolerance` points of the same Top are grouped together.
func BuildLines(rects []TextRect, tolerance float64) []Line {
	if len(rects) == 0 {
		return nil
	}

	// 1. Sort all rects primarily by Top (top-to-bottom), then by Left (left-to-right)
	sortedRects := make([]TextRect, len(rects))
	copy(sortedRects, rects)
	sort.Slice(sortedRects, func(i, j int) bool {
		if math.Abs(sortedRects[i].Top-sortedRects[j].Top) > tolerance {
			return sortedRects[i].Top > sortedRects[j].Top
		}
		return sortedRects[i].Left < sortedRects[j].Left
	})

	var lines []Line
	var currentLine []TextRect

	// Helper to finalize the current line
	finalizeLine := func() {
		if len(currentLine) == 0 {
			return
		}
		// Sort within line strictly by Left
		sort.Slice(currentLine, func(i, j int) bool {
			return currentLine[i].Left < currentLine[j].Left
		})

		// Find line Top/Bottom bounds
		maxTop := currentLine[0].Top
		minBottom := currentLine[0].Bottom
		for _, r := range currentLine {
			if r.Top > maxTop {
				maxTop = r.Top
			}
			if r.Bottom < minBottom {
				minBottom = r.Bottom
			}
		}

		lines = append(lines, Line{
			Rects:   currentLine,
			Top:     maxTop,
			Bottom:  minBottom,
			PageNum: currentLine[0].PageNum,
		})
		currentLine = nil
	}

	for _, r := range sortedRects {
		if len(currentLine) == 0 {
			currentLine = append(currentLine, r)
			continue
		}

		// Check if rect belongs to the current line
		// Use the first rect in currentLine as the anchor for the line's Top
		anchorTop := currentLine[0].Top
		if math.Abs(r.Top-anchorTop) <= tolerance {
			currentLine = append(currentLine, r)
		} else {
			finalizeLine()
			currentLine = append(currentLine, r)
		}
	}
	finalizeLine()

	return lines
}
