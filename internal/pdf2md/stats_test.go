package pdf2md

import (
	"testing"
)

func TestAnalyzeDocument(t *testing.T) {
	allPages := [][]Line{
		{
			{
				Rects:  []TextRect{{Text: "Header", FontSize: 20.1, FontName: "Arial"}},
				Top:    100.0,
				Bottom: 90.0,
			},
			{
				Rects:  []TextRect{{Text: "Body 1", FontSize: 12.2, FontName: "Times"}},
				Top:    85.0, // gap = 5.0
				Bottom: 75.0,
			},
			{
				Rects:  []TextRect{{Text: "Body 2", FontSize: 11.9, FontName: "Times"}},
				Top:    65.0, // gap = 10.0
				Bottom: 55.0,
			},
			{
				Rects:  []TextRect{{Text: "Body 3", FontSize: 12.0, FontName: "Times"}},
				Top:    50.0, // gap = 5.0
				Bottom: 40.0,
			},
		},
	}

	stats := AnalyzeDocument(allPages)

	// Expected mode for font size:
	// 20.1 rounded to nearest 0.5 = 20.0 (count 1)
	// 12.2 rounded = 12.0
	// 11.9 rounded = 12.0
	// 12.0 rounded = 12.0 (count 3)
	if stats.BodyFontSize != 12.0 {
		t.Errorf("expected BodyFontSize 12.0, got %v", stats.BodyFontSize)
	}

	if stats.BodyFontName != "Times" {
		t.Errorf("expected BodyFontName 'Times', got '%v'", stats.BodyFontName)
	}

	// Gaps: 5.0, 10.0, 5.0
	// Sorted: 5.0, 5.0, 10.0 -> median is 5.0
	if stats.MedianLineGap != 5.0 {
		t.Errorf("expected MedianLineGap 5.0, got %v", stats.MedianLineGap)
	}
}
