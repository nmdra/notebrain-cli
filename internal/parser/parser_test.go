// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package parser

import (
	"reflect"
	"sort"
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

func TestIsAttachmentLink(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "empty", target: "", want: false},
		{name: "simple note", target: "Apache Kafka", want: false},
		{name: "note with folder", target: "01.Projects/System Design/Apache Flink", want: false},
		{name: "note with md extension", target: "My Note.md", want: false},
		{name: "note with heading", target: "My Note#Section 1", want: false},
		{name: "note with alias", target: "My Note|Display Text", want: false},
		{name: "note with anchor and alias", target: "My Note#Heading|Alias", want: false},
		{name: "webp image", target: "redis-queue-1741846972555.webp", want: true},
		{name: "png image with alias", target: "image.png|My Image", want: true},
		{name: "pdf document", target: "docs/spec.pdf", want: true},
		{name: "canvas file", target: "architecture.canvas", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAttachmentLink(tt.target); got != tt.want {
				t.Errorf("IsAttachmentLink(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		slug       string
		maxRunes   int
		wantChunks int
		wantTags   []string
		wantLinks  []string
		wantTitle  string
		checkText  func(*testing.T, []Chunk)
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
			checkText: func(t *testing.T, chunks []Chunk) {
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
			checkText: func(t *testing.T, chunks []Chunk) {
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
			res := Parse(tt.body, tt.slug, tt.maxRunes, 0, false)
			if len(res.Chunks) != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", len(res.Chunks), tt.wantChunks)
			}

			sort.Strings(res.Tags)
			sort.Strings(tt.wantTags)
			if !reflect.DeepEqual(res.Tags, tt.wantTags) {
				t.Errorf("got tags %v, want %v", res.Tags, tt.wantTags)
			}
			sort.Strings(res.Links)
			sort.Strings(tt.wantLinks)
			if !reflect.DeepEqual(res.Links, tt.wantLinks) {
				t.Errorf("got links %v, want %v", res.Links, tt.wantLinks)
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

func TestBuildChunks_CodePreservation(t *testing.T) {
	body := "---\ntitle: Test\n---\n# Setup\n\nSome intro text.\n\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\nMore text after code."
	res := Parse(body, "test-note", 2000, 0, false)

	if len(res.Chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}
	c := res.Chunks[0]

	if !strings.Contains(c.Text, "[code:go]") {
		t.Errorf("Text should contain [code:go] placeholder: got %q", c.Text)
	}
	if strings.Contains(c.Text, "fmt.Println") {
		t.Errorf("Text should NOT contain actual code: got %q", c.Text)
	}

	if !strings.Contains(c.RichText, "fmt.Println") {
		t.Errorf("RichText should contain actual code: got %q", c.RichText)
	}
	if strings.Contains(c.RichText, "[code:go]") {
		t.Errorf("RichText should NOT contain placeholder: got %q", c.RichText)
	}
}

func TestBuildChunks_CodeOnlyChunk(t *testing.T) {
	body := "---\ntitle: Test\n---\n# Code Section\n\n```python\ndef hello():\n    print('world')\n```\n"
	res := Parse(body, "test-note", 2000, 0, false)

	found := false
	for _, c := range res.Chunks {
		if c.CodeBlocks > 0 {
			found = true
			if c.RichText == "" {
				t.Error("code chunk should have non-empty RichText")
			}
		}
	}
	if !found {
		t.Error("expected a chunk with code blocks")
	}
}

func TestParse_SkipAttachments(t *testing.T) {
	body := "Here is a note link [[Apache Kafka]] and an image link ![[redis-queue-1741846972555.webp]] and pdf [[doc.pdf]]."

	resSkip := Parse(body, "test-note", 1000, 0, true)
	if len(resSkip.Links) != 1 || resSkip.Links[0] != "Apache Kafka" {
		t.Errorf("expected only Apache Kafka when skipAttachments=true, got %v", resSkip.Links)
	}

	resNoSkip := Parse(body, "test-note", 1000, 0, false)
	sort.Strings(resNoSkip.Links)
	expected := []string{"Apache Kafka", "doc.pdf", "redis-queue-1741846972555.webp"}
	if !reflect.DeepEqual(resNoSkip.Links, expected) {
		t.Errorf("expected all links when skipAttachments=false, got %v, want %v", resNoSkip.Links, expected)
	}
}

func TestParse_ASTStructure(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantTextSubs []string
		wantRichSubs []string
		wantHasTask  bool
	}{
		{
			name: "tight_ordered_list",
			body: "### Modular Architecture\n\nOpenChoreo is designed to be highly extensible. You can:\n\n1. Use default modules — sensible defaults\n2. Swap modules — replace default module\n3. Build your own — create custom modules\n\nThis architecture allows tools to be integrated without issue.",
			wantTextSubs: []string{
				"OpenChoreo is designed to be highly extensible. You can:\n\n1. Use default modules — sensible defaults\n2. Swap modules — replace default module\n3. Build your own — create custom modules\n\nThis architecture allows tools to be integrated without issue.",
			},
			wantRichSubs: []string{
				"1. Use default modules — sensible defaults",
				"2. Swap modules — replace default module",
				"3. Build your own — create custom modules",
			},
		},
		{
			name: "unordered_bullet_list",
			body: "# Bullets\n\n- First point\n- Second point\n",
			wantTextSubs: []string{
				"- First point\n- Second point",
			},
			wantRichSubs: []string{
				"- First point\n- Second point",
			},
		},
		{
			name: "task_list",
			body: "# Tasks\n\n- [ ] Unfinished task\n- [x] Completed task\n",
			wantTextSubs: []string{
				"- [ ] Unfinished task\n- [x] Completed task",
			},
			wantRichSubs: []string{
				"- [ ] Unfinished task\n- [x] Completed task",
			},
			wantHasTask: true,
		},
		{
			name: "nested_list",
			body: "# Nested\n\n1. Top level one\n   - Nested bullet A\n   - Nested bullet B\n2. Top level two\n",
			wantTextSubs: []string{
				"1. Top level one\n  - Nested bullet A\n  - Nested bullet B\n2. Top level two",
			},
			wantRichSubs: []string{
				"1. Top level one\n  - Nested bullet A\n  - Nested bullet B\n2. Top level two",
			},
		},
		{
			name: "table_structure",
			body: "### Summary Data\n\n| Feature | Status | Priority |\n| --- | --- | --- |\n| Semantic Search | Active | High |\n| Graph View | Active | Medium |\n",
			wantTextSubs: []string{
				"| Feature | Status | Priority |\n| --- | --- | --- |\n| Semantic Search | Active | High |\n| Graph View | Active | Medium |",
			},
			wantRichSubs: []string{
				"| Feature | Status | Priority |\n| --- | --- | --- |\n| Semantic Search | Active | High |\n| Graph View | Active | Medium |",
			},
		},
		{
			name: "blockquote_and_callout",
			body: "# Quote\n\n> [!NOTE]\n> NoteBrain indexes Obsidian vaults into ChromaDB.\n> Graph boosted search is included.\n",
			wantTextSubs: []string{
				"> [!NOTE]\n> NoteBrain indexes Obsidian vaults into ChromaDB.\n> Graph boosted search is included.",
			},
			wantRichSubs: []string{
				"> [!NOTE]\n> NoteBrain indexes Obsidian vaults into ChromaDB.\n> Graph boosted search is included.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Parse(tt.body, "test-ast", 2000, 0, false)
			if len(res.Chunks) == 0 {
				t.Fatalf("expected at least 1 chunk for test %s, got 0", tt.name)
			}
			chunk := res.Chunks[0]

			for _, sub := range tt.wantTextSubs {
				if !strings.Contains(chunk.Text, sub) {
					t.Errorf("chunk.Text missing expected substring.\nWant substring:\n%s\nGot Text:\n%s", sub, chunk.Text)
				}
			}
			for _, sub := range tt.wantRichSubs {
				if !strings.Contains(chunk.RichText, sub) {
					t.Errorf("chunk.RichText missing expected substring.\nWant substring:\n%s\nGot RichText:\n%s", sub, chunk.RichText)
				}
			}
			if chunk.HasTask != tt.wantHasTask {
				t.Errorf("expected HasTask %v, got %v", tt.wantHasTask, chunk.HasTask)
			}
		})
	}
}

func TestParse_ExternalLinks(t *testing.T) {
	body := `# Resources

Check out [Context Caching](https://api-docs.deepseek.com/guides/kv_cache/) for details.

Also see [Prompt Caching Video](https://youtu.be/u57EnkQaUTY) for an overview.
`
	res := Parse(body, "test-links", 2000, 0, false)
	if len(res.Chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}
	c := res.Chunks[0]

	// Text (embeddings): plain prose, no URLs
	if strings.Contains(c.Text, "https://") {
		t.Errorf("Text should NOT contain URLs: got %q", c.Text)
	}
	if !strings.Contains(c.Text, "Context Caching") {
		t.Errorf("Text should contain anchor text: got %q", c.Text)
	}

	// RichText (display): full Markdown link syntax
	if !strings.Contains(c.RichText, "[Context Caching](https://api-docs.deepseek.com/guides/kv_cache/)") {
		t.Errorf("RichText should contain full link: got %q", c.RichText)
	}
	if !strings.Contains(c.RichText, "[Prompt Caching Video](https://youtu.be/u57EnkQaUTY)") {
		t.Errorf("RichText should contain full link: got %q", c.RichText)
	}
}

func TestParse_WikilinkPreserved(t *testing.T) {
	body := "# Notes\n\nSee [[Apache Kafka]] and [[System Design|SD]] for more.\n"
	res := Parse(body, "test-wl", 2000, 0, false)
	c := res.Chunks[0]

	if !strings.Contains(c.RichText, "[[Apache Kafka]]") {
		t.Errorf("RichText should preserve wikilink: got %q", c.RichText)
	}
	if !strings.Contains(c.RichText, "[[System Design|SD]]") {
		t.Errorf("RichText should preserve aliased wikilink: got %q", c.RichText)
	}
	// Plain text: just the display text
	if !strings.Contains(c.Text, "Apache Kafka") {
		t.Errorf("Text should contain wikilink text: got %q", c.Text)
	}
	if strings.Contains(c.Text, "[[") {
		t.Errorf("Text should NOT contain [[ brackets: got %q", c.Text)
	}
}

func TestParse_EmphasisPreserved(t *testing.T) {
	body := "# Style\n\nThis is **bold** and *italic* and `code` and ~~struck~~.\n"
	res := Parse(body, "test-em", 2000, 0, false)
	c := res.Chunks[0]

	if !strings.Contains(c.RichText, "**bold**") {
		t.Errorf("RichText should preserve bold: got %q", c.RichText)
	}
	if !strings.Contains(c.RichText, "*italic*") {
		t.Errorf("RichText should preserve italic: got %q", c.RichText)
	}
	if !strings.Contains(c.RichText, "`code`") {
		t.Errorf("RichText should preserve inline code: got %q", c.RichText)
	}
	if !strings.Contains(c.RichText, "~~struck~~") {
		t.Errorf("RichText should preserve strikethrough: got %q", c.RichText)
	}

	// Plain text: no formatting markers
	if strings.Contains(c.Text, "**") || strings.Contains(c.Text, "~~") {
		t.Errorf("Text should NOT contain formatting markers: got %q", c.Text)
	}
	if !strings.Contains(c.Text, "bold") && !strings.Contains(c.Text, "italic") {
		t.Errorf("Text should contain prose words: got %q", c.Text)
	}
}

func TestParse_BlockquoteWithLinks(t *testing.T) {
	body := "# Refs\n\n> See [API Docs](https://example.com/api) for details.\n"
	res := Parse(body, "test-bq-link", 2000, 0, false)
	c := res.Chunks[0]

	if !strings.Contains(c.RichText, "[API Docs](https://example.com/api)") {
		t.Errorf("RichText in blockquote should preserve link: got %q", c.RichText)
	}
	if strings.Contains(c.Text, "https://") {
		t.Errorf("Text in blockquote should NOT contain URL: got %q", c.Text)
	}
}

func BenchmarkParse(b *testing.B) {
	body := `---
title: "Benchmark Note"
tags: [alpha, beta, gamma]
---

# Introduction
This is a long introductory paragraph that contains [[WikiLink1]] and [[WikiLink2]] as well as some inline #hashtags like #golang and #performance.

## Subsection 1
Here is a paragraph with more text and another sentence. We want to test how fast the AST parser can tokenize, extract metadata, and chunk this document into smaller pieces.

` + "```go\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n}\n```" + `

## Subsection 2
Final concluding paragraph with some more prose.
`
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Parse(body, "benchmark-note", 800, 100, false)
	}
}
