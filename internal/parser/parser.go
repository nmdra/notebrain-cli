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
	// nonAlphaNum matches runs of characters that are not letters, digits,
	// or hyphens. \p{L} and \p{N} keep non-ASCII letters and numerals
	// (e.g. accented or CJK text) so "Café" and "Cafe" produce distinct slugs.
	nonAlphaNum    = regexp.MustCompile(`[^\p{L}\p{N}\-]+`)
	multipleHyphen = regexp.MustCompile(`-{2,}`)
)

// TitleFromPath derives a fallback title from the relative file path.
func TitleFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if strings.HasSuffix(strings.ToLower(base), ".md") {
		base = base[:len(base)-3]
	}
	if strings.HasSuffix(strings.ToLower(base), ".pdf") {
		base = base[:len(base)-4]
	}
	return base
}

// Slugify converts a note name/filename to a slug.
// It lowercases, trims .md, replaces spaces with hyphens,
// and removes punctuation, emoji, and other symbols. Unicode letters
// and numerals are preserved so accented or CJK names do not collapse
// into the same slug.
func Slugify(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".md")
	if before, ok := strings.CutSuffix(s, ".pdf"); ok {
		s = before + "-pdf"
	}
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = multipleHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// attachmentExts is the allowlist of known attachment file extensions.
// It is an allowlist (rather than "any suffix that is not .md/.pdf") so
// notes with dotted names such as "Note 1.2.3" are not misclassified
// as attachments. PDFs are excluded: they are ingested as notes.
var attachmentExts = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".svg": {}, ".webp": {},
	".avif": {}, ".bmp": {}, ".tiff": {}, ".ico": {}, ".heic": {},
	".mp4": {}, ".webm": {}, ".mov": {}, ".mkv": {}, ".avi": {},
	".mp3": {}, ".wav": {}, ".ogg": {}, ".flac": {}, ".m4a": {}, ".aac": {},
	".canvas": {}, ".excalidraw": {}, ".zip": {}, ".gz": {}, ".tar": {},
	".7z": {}, ".rar": {}, ".docx": {}, ".pptx": {}, ".xlsx": {}, ".epub": {},
}

// IsAttachmentLink returns true if target points to a known attachment
// file (e.g. images, canvas files, archives). Dotted note names such as
// "Note 1.2.3" are notes, not attachments.
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
	_, isAttachment := attachmentExts[ext]
	return isAttachment
}
