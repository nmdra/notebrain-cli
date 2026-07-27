package pdf2md

import "testing"

func TestClassifyHeadings(t *testing.T) {
	stats := DocumentStats{BodyFontSize: 11.0}

	pages := [][]Line{
		{
			{Rects: []TextRect{{Text: "Introduction", FontSize: 22.0, FontWeight: 700}}},
			{Rects: []TextRect{{Text: "Methods", FontSize: 16.0, FontWeight: 700}}},
			{Rects: []TextRect{{Text: "Data Collection", FontSize: 13.0, FontWeight: 700}}},
			{Rects: []TextRect{{Text: "Short Bold Sub", FontSize: 11.0, FontWeight: 700}}},
			{Rects: []TextRect{{Text: "Normal body text goes here...", FontSize: 11.0, FontWeight: 400}}},
			{Rects: []TextRect{{Text: "This is a very long bold line that contains more than eighty characters and should not be classified as a heading because it is just an emphasized paragraph", FontSize: 11.0, FontWeight: 700}}},
		},
	}

	ClassifyHeadings(pages, stats)

	page := pages[0]

	if page[0].HeadingLevel != 1 {
		t.Errorf("expected H1, got %d", page[0].HeadingLevel)
	}
	if page[1].HeadingLevel != 2 {
		t.Errorf("expected H2, got %d", page[1].HeadingLevel)
	}
	if page[2].HeadingLevel != 3 {
		t.Errorf("expected H3, got %d", page[2].HeadingLevel)
	}
	if page[3].HeadingLevel != 3 {
		t.Errorf("expected H3 for short bold, got %d", page[3].HeadingLevel)
	}
	if page[4].HeadingLevel != 0 {
		t.Errorf("expected body (0), got %d", page[4].HeadingLevel)
	}
	if page[5].HeadingLevel != 0 {
		t.Errorf("expected body (0) for long bold line, got %d", page[5].HeadingLevel)
	}
}
