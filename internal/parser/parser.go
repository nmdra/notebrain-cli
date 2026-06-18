// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package parser provides Markdown parsing, slugification, and text chunking
// for notebrain-cli.
package parser

import (
	"regexp"
	"strings"
)

var (
	nonAlphaNum    = regexp.MustCompile(`[^a-z0-9\-]+`)
	multipleHyphen = regexp.MustCompile(`-{2,}`)
)

// Slugify converts a note name/filename to a URL-safe slug.
// It lowercases, trims .md, replaces spaces with hyphens,
// and removes non-alphanumeric characters except hyphens.
func Slugify(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	s = strings.TrimSuffix(s, ".md")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = multipleHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// StripFrontmatter removes YAML frontmatter (between --- delimiters)
// from the start of content.
func StripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	// Find the closing --- after the opening one.
	rest := content[4:] // skip opening "---\n"
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return content
	}
	// Skip past the closing "---" and any leading newlines.
	after := rest[idx+4:] // len("\n---") == 4
	after = strings.TrimLeft(after, "\n")
	return after
}

// Chunk represents a piece of chunked text with its sequential index.
type Chunk struct {
	Index int
	Text  string
}

// ChunkText splits text into word-based chunks of approximately chunkSize
// words with overlap words of overlap between consecutive chunks.
func ChunkText(text string, chunkSize, overlap int) []Chunk {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []Chunk
	start := 0
	idx := 0

	for start < len(words) {
		end := start + chunkSize
		if end > len(words) {
			end = len(words)
		}
		chunk := Chunk{
			Index: idx,
			Text:  strings.Join(words[start:end], " "),
		}
		chunks = append(chunks, chunk)
		idx++

		// Advance by (chunkSize - overlap) words.
		step := chunkSize - overlap
		if step < 1 {
			step = 1
		}
		start += step
	}

	return chunks
}
