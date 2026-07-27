package pdf2md

import (
	"math"
	"sort"
	"strings"
)

// BuildLines groups TextRects into visual lines based on vertical position.
func BuildLines(rects []TextRect, tolerance float64) []Line {
	if len(rects) == 0 {
		return nil
	}

	// To sort stably by line without violating transitivity, we quantize the vertical center
	sortedRects := make([]TextRect, len(rects))
	copy(sortedRects, rects)
	sort.Slice(sortedRects, func(i, j int) bool {
		avgHeight := (sortedRects[i].Height() + sortedRects[j].Height()) / 2.0
		if avgHeight == 0 {
			avgHeight = 10.0
		}
		// Quantize center to chunks of ~40% of line height
		// We use math.Round to put similar centers into the same bucket
		bucketSize := avgHeight * 0.4
		cI := math.Round(((sortedRects[i].Top + sortedRects[i].Bottom) / 2.0) / bucketSize)
		cJ := math.Round(((sortedRects[j].Top + sortedRects[j].Bottom) / 2.0) / bucketSize)

		if cI != cJ {
			return cI < cJ // smaller Y (Top) comes first (top-to-bottom)
		}
		return sortedRects[i].Left < sortedRects[j].Left
	})

	var lines []Line
	var currentLine []TextRect

	finalizeLine := func() {
		if len(currentLine) == 0 {
			return
		}
		// Sort within line strictly by Left
		sort.Slice(currentLine, func(i, j int) bool {
			return currentLine[i].Left < currentLine[j].Left
		})

		// Deduplicate overlapping rects (e.g., faux bold effects)
		var dedup []TextRect
		for _, r := range currentLine {
			if len(dedup) == 0 {
				dedup = append(dedup, r)
				continue
			}
			lastIdx := len(dedup) - 1
			last := &dedup[lastIdx]

			overlap := math.Min(r.Right, last.Right) - math.Max(r.Left, last.Left)
			if overlap > 0 {
				lt := strings.TrimSpace(last.Text)
				rt := strings.TrimSpace(r.Text)
				if lt != "" && rt != "" {
					if lt == rt {
						continue
					}
					if strings.Contains(rt, lt) {
						*last = r
						continue
					}
					if strings.Contains(lt, rt) {
						continue
					}
				}
			}
			dedup = append(dedup, r)
		}
		currentLine = dedup

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

		// Use the vertical center of the current line to decide if this belongs
		anchorCenter := 0.0
		for _, cr := range currentLine {
			anchorCenter += (cr.Top + cr.Bottom) / 2.0
		}
		anchorCenter /= float64(len(currentLine))

		rCenter := (r.Top + r.Bottom) / 2.0

		if math.Abs(rCenter-anchorCenter) <= r.Height()*0.4 {
			currentLine = append(currentLine, r)
		} else {
			finalizeLine()
			currentLine = append(currentLine, r)
		}
	}
	finalizeLine()

	return lines
}
