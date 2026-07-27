package pdf2md

import (
	"reflect"
	"testing"
)

func TestBuildLines(t *testing.T) {
	tests := []struct {
		name      string
		rects     []TextRect
		tolerance float64
		wantLines int
		wantText  []string
	}{
		{
			name: "single line",
			rects: []TextRect{
				{Text: "Hello", Top: 100.0, Bottom: 90.0, Left: 10.0, PageNum: 1},
				{Text: "World", Top: 100.5, Bottom: 90.5, Left: 50.0, PageNum: 1},
			},
			tolerance: 1.0,
			wantLines: 1,
			wantText:  []string{"Hello World"},
		},
		{
			name: "two lines",
			rects: []TextRect{
				{Text: "World", Top: 100.0, Bottom: 90.0, Left: 50.0, PageNum: 1}, // mixed order
				{Text: "Line 2", Top: 85.0, Bottom: 75.0, Left: 10.0, PageNum: 1},
				{Text: "Hello", Top: 100.5, Bottom: 90.5, Left: 10.0, PageNum: 1},
			},
			tolerance: 1.0,
			wantLines: 2,
			wantText:  []string{"Hello World", "Line 2"},
		},
		{
			name: "superscript/subscript out of tolerance",
			rects: []TextRect{
				{Text: "E=mc", Top: 100.0, Bottom: 90.0, Left: 10.0, PageNum: 1},
				{Text: "2", Top: 105.0, Bottom: 95.0, Left: 40.0, PageNum: 1}, // 105.0 vs 100.0 > tolerance 2.0
			},
			tolerance: 2.0,
			wantLines: 2,
			wantText:  []string{"2", "E=mc"}, // "2" comes first because Top is 105.0
		},
		{
			name: "superscript within tolerance",
			rects: []TextRect{
				{Text: "E=mc", Top: 100.0, Bottom: 90.0, Left: 10.0, PageNum: 1},
				{Text: "2", Top: 101.5, Bottom: 95.0, Left: 40.0, PageNum: 1}, // 101.5 - 100.0 = 1.5 <= 2.0
			},
			tolerance: 2.0,
			wantLines: 1,
			wantText:  []string{"E=mc 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := BuildLines(tt.rects, tt.tolerance)
			if len(lines) != tt.wantLines {
				t.Fatalf("BuildLines() got %v lines, want %v", len(lines), tt.wantLines)
			}
			var gotText []string
			for _, l := range lines {
				gotText = append(gotText, l.FullText())
			}
			if !reflect.DeepEqual(gotText, tt.wantText) {
				t.Errorf("BuildLines() got text %v, want %v", gotText, tt.wantText)
			}
		})
	}
}
