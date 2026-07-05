// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package parser provides Markdown parsing, slugification, and text chunking
// for notebrain-cli.
package parser

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	nonAlphaNum    = regexp.MustCompile(`[^a-z0-9\-]+`)
	multipleHyphen = regexp.MustCompile(`-{2,}`)
)

// TitleFromPath derives a fallback title from the relative file path.
func TitleFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}

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

// IsAttachmentLink returns true if target points to a non-markdown file or attachment
// (e.g., images, PDFs, canvas files).
func IsAttachmentLink(target string) bool {
	s := strings.TrimSpace(target)
	if s == "" {
		return false
	}
	// Strip alias if present (e.g. "image.png|My Image")
	if idx := strings.Index(s, "|"); idx != -1 {
		s = s[:idx]
	}
	// Strip anchor if present (e.g. "Note#Heading")
	if idx := strings.Index(s, "#"); idx != -1 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(s))
	if ext != "" && ext != ".md" {
		return true
	}
	return false
}
