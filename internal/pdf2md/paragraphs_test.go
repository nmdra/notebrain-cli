package pdf2md

import (
	"reflect"
	"testing"
)

func TestGroupBlocks(t *testing.T) {
	stats := DocumentStats{MedianLineGap: 5.0}

	pages := [][]Line{
		{
			{Rects: []TextRect{{Text: "Heading"}}, HeadingLevel: 1},
			{Rects: []TextRect{{Text: "This is a paragraph."}}, Top: 100.0, Bottom: 90.0},
			{Rects: []TextRect{{Text: "It continues on the next line."}}, Top: 87.0, Bottom: 77.0},     // gap=3.0 (< 1.8*5)
			{Rects: []TextRect{{Text: "This is a new paragraph."}}, Top: 60.0, Bottom: 50.0},           // gap=17.0 (> 1.8*5)
			{Rects: []TextRect{{Text: "This paragraph flows onto the next"}}, Top: 25.0, Bottom: 15.0}, // no punctuation, huge gap
		},
		{
			{Rects: []TextRect{{Text: "page."}}, Top: 100.0, Bottom: 90.0}, // lowercase, merges with previous page
			{Rects: []TextRect{{Text: "Subheading"}}, HeadingLevel: 2},
			{Rects: []TextRect{{Text: "Final text."}}, Top: 80.0, Bottom: 70.0},
		},
	}

	blocks := GroupBlocks(pages, stats)

	wantBlocks := []Block{
		HeadingBlock{Level: 1, Text: "Heading"},
		ParagraphBlock{Text: "This is a paragraph. It continues on the next line."},
		ParagraphBlock{Text: "This is a new paragraph."},
		ParagraphBlock{Text: "This paragraph flows onto the next page."},
		HeadingBlock{Level: 2, Text: "Subheading"},
		ParagraphBlock{Text: "Final text."},
	}

	if len(blocks) != len(wantBlocks) {
		t.Errorf("expected %d blocks, got %d", len(wantBlocks), len(blocks))
		for i, b := range blocks {
			t.Logf("Block %d: %#v", i, b)
		}
		t.FailNow()
	}

	for i, b := range blocks {
		if !reflect.DeepEqual(b, wantBlocks[i]) {
			t.Errorf("block %d: got %#v, want %#v", i, b, wantBlocks[i])
		}
	}
}

func TestGroupBlocks_Lists(t *testing.T) {
	stats := DocumentStats{MedianLineGap: 5.0}

	pages := [][]Line{
		{
			{Rects: []TextRect{{Text: "Before list."}}, Top: 100.0, Bottom: 90.0},
			{Rects: []TextRect{{Text: "- Item 1"}}, Top: 80.0, Bottom: 70.0},
			{Rects: []TextRect{{Text: "continuation of item 1"}}, Top: 67.0, Bottom: 57.0}, // small gap, part of item 1
			{Rects: []TextRect{{Text: "• Item 2"}}, Top: 50.0, Bottom: 40.0},
			{Rects: []TextRect{{Text: "After list."}}, Top: 20.0, Bottom: 10.0}, // large gap, breaks list
		},
	}

	blocks := GroupBlocks(pages, stats)

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	if _, ok := blocks[1].(ListBlock); !ok {
		t.Fatalf("expected blocks[1] to be ListBlock, got %T", blocks[1])
	}
	list := blocks[1].(ListBlock)
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list.Items))
	}
	if list.Items[0] != "- Item 1 continuation of item 1" {
		t.Errorf("item 0 mismatch: %s", list.Items[0])
	}
	if list.Items[1] != "• Item 2" {
		t.Errorf("item 1 mismatch: %s", list.Items[1])
	}
}
