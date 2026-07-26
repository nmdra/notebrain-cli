package pdfextract

import (
	"reflect"
	"strings"
	"testing"
)

func TestChunkPages(t *testing.T) {
	tests := []struct {
		name     string
		pages    []string
		minWords int
		maxRunes int
		overlap  int
		want     []PDFChunk
	}{
		{
			name:     "Single short page",
			pages:    []string{"This is page one."},
			minWords: 2,
			maxRunes: 800,
			overlap:  100,
			want: []PDFChunk{
				{PageNum: 1, Text: "This is page one."},
			},
		},
		{
			name:     "Multiple pages, some empty/sparse",
			pages:    []string{"Page one content here.", "  ", "Page three content here."},
			minWords: 2,
			maxRunes: 800,
			overlap:  100,
			want: []PDFChunk{
				{PageNum: 1, Text: "Page one content here."},
				{PageNum: 3, Text: "Page three content here."},
			},
		},
		{
			name:     "Long page needing split",
			pages:    []string{strings.Repeat("word ", 200)}, // 1000 chars
			minWords: 2,
			maxRunes: 500, // force split
			overlap:  50,
			// Since exact splitting depends on implementation, we'll just check len(chunks) > 1 and page num
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChunkPages(tt.pages, tt.minWords, tt.maxRunes, tt.overlap)

			if tt.name == "Long page needing split" {
				if len(got) < 2 {
					t.Errorf("Expected page to be split into multiple chunks, got %d", len(got))
				}
				for _, c := range got {
					if c.PageNum != 1 {
						t.Errorf("Expected all chunks to have PageNum 1, got %d", c.PageNum)
					}
					if len(c.Text) > tt.maxRunes {
						t.Errorf("Chunk exceeds maxRunes %d: len=%d", tt.maxRunes, len(c.Text))
					}
				}
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChunkPages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripHeadersFooters(t *testing.T) {
	pages := []string{
		"Header X\nContent 1\nFooter Y",
		"Header X\nContent 2\nFooter Y",
		"Header X\nContent 3\nFooter Y",
		"Header X\nContent 4\nFooter Y",
	}

	stripped := stripHeadersFooters(pages)

	for i, p := range stripped {
		if strings.Contains(p, "Header X") {
			t.Errorf("Page %d still contains header", i+1)
		}
		if strings.Contains(p, "Footer Y") {
			t.Errorf("Page %d still contains footer", i+1)
		}
		if !strings.Contains(p, "Content") {
			t.Errorf("Page %d missing content", i+1)
		}
	}
}

func TestStripHeadersFooters_NoRepeat(t *testing.T) {
	pages := []string{
		"Title Page\nContent 1",
		"Chapter 1\nContent 2",
		"Chapter 2\nContent 3",
	}

	stripped := stripHeadersFooters(pages)

	// Should not strip anything since there's no >70% repetition
	if !reflect.DeepEqual(stripped, pages) {
		t.Errorf("stripHeadersFooters() = %v, want %v", stripped, pages)
	}
}
