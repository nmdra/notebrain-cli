package pdf2md

import (
	"slices"
	"sort"
)

type interval struct {
	Min float64
	Max float64
}

func mergeIntervals(intervals []interval, tolerance float64) []interval {
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].Min < intervals[j].Min })

	var merged []interval
	curr := intervals[0]
	for _, intv := range intervals[1:] {
		// Inflate slightly to tolerate microscopic overlaps/gaps
		if intv.Min <= curr.Max+tolerance {
			if intv.Max > curr.Max {
				curr.Max = intv.Max
			}
		} else {
			merged = append(merged, curr)
			curr = intv
		}
	}
	merged = append(merged, curr)
	return merged
}

// XYCut recursively clusters TextRects into reading blocks based on horizontal and vertical gaps.
func XYCut(rects []TextRect) [][]TextRect {
	if len(rects) == 0 {
		return nil
	}

	// 1. Try Y-cut (horizontal split)
	var yIntervals []interval
	for _, r := range rects {
		yIntervals = append(yIntervals, interval{Min: r.Bottom, Max: r.Top})
	}
	// Use a tolerance of 5.0 to merge typical line gaps so we don't cut between every line.
	yMerged := mergeIntervals(yIntervals, 5.0)

	if len(yMerged) > 1 {
		// Y-cut found. Split into blocks and recurse.
		// Since Y points UP, higher values are top of the page.
		// We want to process from top to bottom, so we iterate yMerged in reverse.
		var blocks [][]TextRect
		for _, intv := range slices.Backward(yMerged) {
			var subRects []TextRect
			for _, r := range rects {
				center := (r.Top + r.Bottom) / 2.0
				if center >= intv.Min-2.5 && center <= intv.Max+2.5 {
					subRects = append(subRects, r)
				}
			}
			if len(subRects) > 0 {
				blocks = append(blocks, XYCut(subRects)...)
			}
		}
		return blocks
	}

	// 2. Try X-cut (vertical split, e.g., columns)
	var xIntervals []interval
	for _, r := range rects {
		xIntervals = append(xIntervals, interval{Min: r.Left, Max: r.Right})
	}
	// Use a tolerance of 10.0 to merge words within the same column but keep gutters distinct.
	xMerged := mergeIntervals(xIntervals, 10.0)

	if len(xMerged) > 1 {
		// X-cut found. Split into columns and recurse.
		// X points RIGHT. We want to process left to right.
		var blocks [][]TextRect
		for i := range xMerged {
			intv := xMerged[i]
			var subRects []TextRect
			for _, r := range rects {
				center := (r.Left + r.Right) / 2.0
				if center >= intv.Min-5.0 && center <= intv.Max+5.0 {
					subRects = append(subRects, r)
				}
			}
			if len(subRects) > 0 {
				blocks = append(blocks, XYCut(subRects)...)
			}
		}
		return blocks
	}

	// 3. No cuts possible, return as a single block
	return [][]TextRect{rects}
}
