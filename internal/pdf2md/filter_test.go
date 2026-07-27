package pdf2md

import (
	"fmt"
	"strings"
	"testing"
)

func TestFilterNoise(t *testing.T) {
	// Create 4 pages with the same header and footer
	var pages [][]Line
	for i := range 4 {
		pages = append(pages, []Line{
			{Rects: []TextRect{{Text: "Annual Report 2024", FontSize: 12.0}}},
			{Rects: []TextRect{{Text: fmt.Sprintf("Body text on page %d", i+1), FontSize: 12.0}}},
			{Rects: []TextRect{{Text: fmt.Sprintf("Page %d", i+14), FontSize: 12.0}}},
			{Rects: []TextRect{{Text: "Confidential", FontSize: 12.0}}}, // Footer
		})
	}

	// Make page 1 different slightly to test threshold
	pages[0][0].Rects[0].Text = "Different Header"

	stats := DocumentStats{BodyFontSize: 12.0}
	cleaned := FilterNoise(pages, stats)

	if len(cleaned) != 4 {
		t.Fatalf("expected 4 cleaned pages, got %d", len(cleaned))
	}

	for i, page := range cleaned {
		if i == 0 {
			// Page 0 has "Different Header" which didn't meet the threshold, so it stays
			if len(page) != 2 {
				t.Errorf("expected 2 lines on page 0, got %d", len(page))
			} else {
				if page[0].FullText() != "Different Header" {
					t.Errorf("expected 'Different Header', got %s", page[0].FullText())
				}
				if !strings.HasPrefix(page[1].FullText(), "Body text on page") {
					t.Errorf("expected body text, got %s", page[1].FullText())
				}
			}
		} else {
			// Other pages should only have the body text (header and footer and page number removed)
			if len(page) != 1 {
				t.Errorf("expected 1 line on page %d, got %d", i, len(page))
			} else if !strings.HasPrefix(page[0].FullText(), "Body text on page") {
				t.Errorf("expected body text, got %s", page[0].FullText())
			}
		}
	}
}

func TestFilterNoise_GiantText(t *testing.T) {
	pages := [][]Line{
		{
			{Rects: []TextRect{{Text: "DRAFT", FontSize: 40.0}}},
			{Rects: []TextRect{{Text: "Body text", FontSize: 10.0}}},
		},
	}

	stats := DocumentStats{BodyFontSize: 10.0}
	cleaned := FilterNoise(pages, stats)

	if len(cleaned) != 1 || len(cleaned[0]) != 1 {
		t.Fatalf("expected giant text to be filtered")
	}
	if cleaned[0][0].FullText() != "Body text" {
		t.Errorf("expected 'Body text', got %s", cleaned[0][0].FullText())
	}
}

func TestFilterNoise_MicroText(t *testing.T) {
	pages := [][]Line{
		{
			{Rects: []TextRect{{Text: "Body text", FontSize: 10.0}}},
			{Rects: []TextRect{{Text: "tiny copyright", FontSize: 4.5}}},
		},
	}

	stats := DocumentStats{BodyFontSize: 10.0}
	cleaned := FilterNoise(pages, stats)

	if len(cleaned) != 1 || len(cleaned[0]) != 1 {
		t.Fatalf("expected micro text to be filtered")
	}
	if cleaned[0][0].FullText() != "Body text" {
		t.Errorf("expected 'Body text', got %s", cleaned[0][0].FullText())
	}
}
