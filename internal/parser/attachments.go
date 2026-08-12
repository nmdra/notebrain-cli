// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/wikilink"
)

// AttachmentKind classifies a resolved reference.
type AttachmentKind string

const (
	KindImage         AttachmentKind = "image"
	KindPDF           AttachmentKind = "pdf"
	KindOther         AttachmentKind = "other"
	KindExternalLinks AttachmentKind = "external-links"
)

// AttachmentSource records which markdown syntax produced a reference, so
// resolution can apply Obsidian semantics (wiki vs note-folder-relative).
type AttachmentSource string

const (
	SrcWiki     AttachmentSource = "wiki"
	SrcMarkdown AttachmentSource = "markdown"
)

// Ref is one reference extracted from a note body. Target holds the cleaned
// wiki target, the raw markdown destination, or the full URL for external
// links; Kind classifies it; Source records which markdown syntax produced it
// so resolution can apply Obsidian semantics (wiki vs note-folder-relative).
// External links carry no Source: they are never resolved against the vault.
type Ref struct {
	Target string
	Kind   AttachmentKind
	Source AttachmentSource
}

// ExtractedRefs holds the references collected from a note body, deduped (by
// cleaned target or exact URL) in first-occurrence document order across all
// kinds.
type ExtractedRefs struct {
	Refs []Ref
}

// ExtractReferences walks a note body's AST and collects direct references:
// local attachments (wiki and markdown syntax) and external http(s) website
// links. URLs and content inside code fences never match. Results are deduped
// by target (or exact URL) in first-occurrence document order, so an external
// link that appears before an image is reported before it.
func ExtractReferences(body string) ExtractedRefs {
	src := []byte(body)
	doc := mdParser.Parser().Parse(text.NewReader(src))

	var refs ExtractedRefs
	seen := make(map[string]struct{})

	add := func(ref Ref) {
		if _, ok := seen[ref.Target]; ok {
			return
		}
		seen[ref.Target] = struct{}{}
		refs.Refs = append(refs.Refs, ref)
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nTyped := n.(type) {
		case *wikilink.Node:
			target := string(nTyped.Target)
			if target == "" {
				break
			}
			if isHTTPScheme(target) {
				add(Ref{Target: target, Kind: KindExternalLinks})
				break
			}
			cleaned := cleanWikiTarget(target)
			if kind, ok := classifyAttachmentKind(cleaned); ok {
				add(Ref{Target: cleaned, Kind: kind, Source: SrcWiki})
			}
		case *ast.Link:
			handleMarkdownReference(string(nTyped.Destination), add)
		case *ast.Image:
			handleMarkdownReference(string(nTyped.Destination), add)
		case *ast.AutoLink:
			if nTyped.AutoLinkType != ast.AutoLinkURL {
				break
			}
			// URL() assembles the scheme for <...> autolinks; linkify nodes
			// already carry it in the value. Either way only http(s) counts.
			if url := string(nTyped.URL(src)); isHTTPScheme(url) {
				add(Ref{Target: url, Kind: KindExternalLinks})
			}
		}
		return ast.WalkContinue, nil
	})

	return refs
}

// handleMarkdownReference classifies a markdown link/image destination as
// either an external http(s) URL or a local attachment.
func handleMarkdownReference(destination string, add func(Ref)) {
	if isHTTPScheme(destination) {
		add(Ref{Target: destination, Kind: KindExternalLinks})
		return
	}
	if kind, ok := classifyAttachmentKind(destination); ok {
		add(Ref{Target: destination, Kind: kind, Source: SrcMarkdown})
	}
}

// isHTTPScheme reports whether s starts with an http:// or https:// scheme.
func isHTTPScheme(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// cleanWikiTarget strips the alias (|...) and heading fragment (#...) from a
// wiki link target so the file target itself is classified and resolved.
func cleanWikiTarget(target string) string {
	if idx := strings.Index(target, "|"); idx != -1 {
		target = target[:idx]
	}
	if idx := strings.Index(target, "#"); idx != -1 {
		target = target[:idx]
	}
	return strings.TrimSpace(target)
}

// classifyAttachmentKind returns the kind of an attachment reference for a
// cleaned target (extensions only), or false when the target is not a known
// attachment (e.g. other notes). Unlike IsAttachmentLink, PDFs classify here
// because refs list them as file references even though ingestion treats
// them as notes.
func classifyAttachmentKind(target string) (AttachmentKind, bool) {
	if idx := strings.LastIndex(target, "#"); idx != -1 {
		target = target[:idx]
	}
	ext := strings.ToLower(filepath.Ext(target))
	if _, ok := imageExts[ext]; ok {
		return KindImage, true
	}
	if ext == ".pdf" {
		return KindPDF, true
	}
	if _, ok := attachmentExts[ext]; ok {
		return KindOther, true
	}
	return "", false
}
