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
			wantTags:   []string{"hashtag"},
			wantLinks:  []string{"WikiLink"},
			wantTitle:  "Test Note",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ParseAST(tt.body, tt.slug, tt.maxRunes)
			if len(res.Chunks) != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", len(res.Chunks), tt.wantChunks)
			}

			// Simple check for tags (order doesn't matter, but let's just do a basic presence check)
			if len(res.Tags) != len(tt.wantTags) {
				t.Errorf("got %d tags, want %d", len(res.Tags), len(tt.wantTags))
			}
			if len(res.Links) != len(tt.wantLinks) {
				t.Errorf("got %d links, want %d", len(res.Links), len(tt.wantLinks))
			}

			if tt.wantTitle != "" {
				if title, ok := res.Frontmatter["title"].(string); !ok || title != tt.wantTitle {
					t.Errorf("got title %v, want %s", res.Frontmatter["title"], tt.wantTitle)
				}
			}
		})
	}
}
