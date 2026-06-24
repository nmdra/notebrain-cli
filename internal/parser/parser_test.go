// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package parser

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple two words", input: "My Note", want: "my-note"},
		{name: "strip .md extension", input: "My Note.md", want: "my-note"},
		{name: "remove punctuation", input: "Hello World!", want: "hello-world"},
		{name: "already slugified", input: "already-slugified", want: "already-slugified"},
		{name: "trim spaces", input: "  spaces  ", want: "spaces"},
		{name: "collapse multiple spaces", input: "Multiple   Spaces", want: "multiple-spaces"},
		{name: "empty string", input: "", want: ""},
		{name: "only special chars", input: "!!!", want: ""},
		{name: "mixed case and numbers", input: "Note 42 Test", want: "note-42-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTitleFromPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "My Note.md", want: "My Note"},
		{name: "with directory", input: "Folder/My Note.md", want: "My Note"},
		{name: "no extension", input: "My Note", want: "My Note"},
		{name: "empty", input: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TitleFromPath(tt.input); got != tt.want {
				t.Errorf("TitleFromPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAST(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		slug       string
		maxRunes   int
		wantChunks int
		wantTags   []string
		wantLinks  []string
		wantTitle  string
		checkText  func(*testing.T, []ASTChunk)
	}{
		{
			name:     "basic parsing with frontmatter and wikilinks",
			slug:     "test-note",
			maxRunes: 500,
			body: `---
title: "Test Note"
tags: [a, b]
---

# Heading 1
Some prose here with a [[WikiLink]] and a #hashtag.

## Subheading
More content.
`,
			wantChunks: 2,
			wantTags:   []string{"a", "b", "hashtag"},
			wantLinks:  []string{"WikiLink"},
			wantTitle:  "Test Note",
			checkText: func(t *testing.T, chunks []ASTChunk) {
				if len(chunks) > 0 {
					// The '#' should be stripped from the inline tag in prose
					expected := "Some prose here with a WikiLink and a hashtag."
					if chunks[0].Text != expected {
						t.Errorf("expected chunk text %q, got %q", expected, chunks[0].Text)
					}
				}
			},
		},
		{
			name:       "code block",
			slug:       "code-note",
			maxRunes:   500,
			body:       "```go\nfmt.Println(\"hi\")\n```\n",
			wantChunks: 1,
			wantTags:   []string{},
			wantLinks:  []string{},
			wantTitle:  "",
		},
		{
			name:       "chunk splitting boundary",
			slug:       "long-note",
			maxRunes:   50, // very small to force split
			body:       "# Intro\n\nFirst paragraph that is somewhat long and should definitely trigger a split.\n\nSecond paragraph here also fairly long.\n\nThird paragraph.\n",
			wantChunks: 3, // "Intro" + P1, P2, P3
			wantTags:   []string{},
			wantLinks:  []string{},
			wantTitle:  "",
		},
		{
			name:     "tag-only blocks skipped, inline hashtags cleaned",
			slug:     "tags-note",
			maxRunes: 500,
			body: `# Section 1
This is some prose with #golang inline.

#tag1 #tag2 #tag3

# Section 2
Some other text.
`,
			wantChunks: 2, // The tag-only middle paragraph is skipped!
			wantTags:   []string{"golang", "tag1", "tag2", "tag3"},
			wantLinks:  []string{},
			wantTitle:  "",
			checkText: func(t *testing.T, chunks []ASTChunk) {
				if len(chunks) != 2 {
					t.Fatalf("expected 2 chunks, got %d", len(chunks))
				}
				expected1 := "This is some prose with golang inline."
				expected2 := "Some other text."
				if chunks[0].Text != expected1 {
					t.Errorf("chunk 0: expected %q, got %q", expected1, chunks[0].Text)
				}
				if chunks[1].Text != expected2 {
					t.Errorf("chunk 1: expected %q, got %q", expected2, chunks[1].Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ParseAST(tt.body, tt.slug, tt.maxRunes)
			if len(res.Chunks) != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", len(res.Chunks), tt.wantChunks)
			}

			// Simple check for tags (order doesn't matter, but let's just do a basic presence check)
			if len(res.Tags) != len(tt.wantTags) {
				t.Errorf("got %d tags (%v), want %d (%v)", len(res.Tags), res.Tags, len(tt.wantTags), tt.wantTags)
			}
			if len(res.Links) != len(tt.wantLinks) {
				t.Errorf("got %d links, want %d", len(res.Links), len(tt.wantLinks))
			}

			if tt.wantTitle != "" {
				if title, ok := res.Frontmatter["title"].(string); !ok || title != tt.wantTitle {
					t.Errorf("got title %v, want %s", res.Frontmatter["title"], tt.wantTitle)
				}
			}

			if tt.checkText != nil {
				tt.checkText(t, res.Chunks)
			}
		})
	}
}
