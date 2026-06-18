// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package parser

import (
	"strings"
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

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "with frontmatter",
			content: "---\ntitle: Hello\ntags: [go]\n---\nBody text here.",
			want:    "Body text here.",
		},
		{
			name:    "without frontmatter",
			content: "Just some plain text.",
			want:    "Just some plain text.",
		},
		{
			name:    "empty string",
			content: "",
			want:    "",
		},
		{
			name:    "frontmatter with no closing delimiter",
			content: "---\ntitle: Hello\nBody without closing.",
			want:    "---\ntitle: Hello\nBody without closing.",
		},
		{
			name:    "frontmatter only",
			content: "---\ntitle: Hello\n---",
			want:    "",
		},
		{
			name:    "frontmatter with trailing newline",
			content: "---\ntitle: Hello\n---\n\nBody after blank line.",
			want:    "Body after blank line.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripFrontmatter(tt.content)
			if got != tt.want {
				t.Errorf("StripFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChunkText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		chunkSize int
		overlap   int
		wantLen   int
		checkFunc func(t *testing.T, chunks []Chunk)
	}{
		{
			name:      "empty text",
			text:      "",
			chunkSize: 10,
			overlap:   2,
			wantLen:   0,
		},
		{
			name:      "short text single chunk",
			text:      "hello world foo",
			chunkSize: 10,
			overlap:   2,
			wantLen:   1,
			checkFunc: func(t *testing.T, chunks []Chunk) {
				t.Helper()
				if chunks[0].Index != 0 {
					t.Errorf("first chunk index = %d, want 0", chunks[0].Index)
				}
				if chunks[0].Text != "hello world foo" {
					t.Errorf("first chunk text = %q, want %q", chunks[0].Text, "hello world foo")
				}
			},
		},
		{
			name:      "multiple chunks with overlap",
			text:      strings.Join(makeWords(20), " "),
			chunkSize: 5,
			overlap:   2,
			checkFunc: func(t *testing.T, chunks []Chunk) {
				t.Helper()
				if len(chunks) < 2 {
					t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
				}
				// Verify sequential indices.
				for i, c := range chunks {
					if c.Index != i {
						t.Errorf("chunk %d index = %d, want %d", i, c.Index, i)
					}
				}
				// Verify overlap: last 2 words of chunk 0 appear at start of chunk 1.
				words0 := strings.Fields(chunks[0].Text)
				words1 := strings.Fields(chunks[1].Text)
				if len(words0) < 2 || len(words1) < 2 {
					t.Fatal("chunks too short to verify overlap")
				}
				overlapWords0 := words0[len(words0)-2:]
				overlapWords1 := words1[:2]
				for i := range overlapWords0 {
					if overlapWords0[i] != overlapWords1[i] {
						t.Errorf("overlap mismatch at position %d: %q != %q",
							i, overlapWords0[i], overlapWords1[i])
					}
				}
			},
		},
		{
			name:      "chunk size of 1 no overlap",
			text:      "alpha beta gamma",
			chunkSize: 1,
			overlap:   0,
			wantLen:   3,
			checkFunc: func(t *testing.T, chunks []Chunk) {
				t.Helper()
				expected := []string{"alpha", "beta", "gamma"}
				for i, c := range chunks {
					if c.Text != expected[i] {
						t.Errorf("chunk %d = %q, want %q", i, c.Text, expected[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text, tt.chunkSize, tt.overlap)
			if tt.wantLen > 0 && len(chunks) != tt.wantLen {
				t.Fatalf("ChunkText() returned %d chunks, want %d", len(chunks), tt.wantLen)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, chunks)
			}
		})
	}
}

// makeWords generates n words like "w0", "w1", …, "w(n-1)".
func makeWords(n int) []string {
	words := make([]string, n)
	for i := range n {
		words[i] = "w" + strings.Repeat("x", i%3) + string(rune('0'+i%10))
	}
	return words
}
