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

// AttachmentRef is one attachment reference extracted from a note body.
// Target is cleaned for wiki refs (alias/anchor stripped) and the raw
// destination for markdown refs; resolution happens in the caller.
type AttachmentRef struct {
	Target string
	Kind   AttachmentKind
	Source AttachmentSource
}

// ExtractedRefs holds the references collected from a note body.
type ExtractedRefs struct {
	Attachments []AttachmentRef
	External    []string
}

// ExtractReferences walks a note body's AST and collects direct references:
// local attachments (wiki and markdown syntax) and external http(s) website
// links. URLs and content inside code fences never match. Results are deduped
// (attachments by cleaned target, external by exact URL) in first-occurrence
// document order.
func ExtractReferences(body string) ExtractedRefs {
	src := []byte(body)
	doc := mdParser.Parser().Parse(text.NewReader(src))

	var refs ExtractedRefs
	seenAttachments := make(map[string]struct{})
	seenExternal := make(map[string]struct{})

	addAttachment := func(ref AttachmentRef) {
		if _, ok := seenAttachments[ref.Target]; ok {
			return
		}
		seenAttachments[ref.Target] = struct{}{}
		refs.Attachments = append(refs.Attachments, ref)
	}
	addExternal := func(url string) {
		if _, ok := seenExternal[url]; ok {
			return
		}
		seenExternal[url] = struct{}{}
		refs.External = append(refs.External, url)
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
				addExternal(target)
				break
			}
			cleaned := cleanWikiTarget(target)
			if kind, ok := classifyAttachmentKind(cleaned); ok {
				addAttachment(AttachmentRef{Target: cleaned, Kind: kind, Source: SrcWiki})
			}
		case *ast.Link:
			handleMarkdownReference(string(nTyped.Destination), addAttachment, addExternal)
		case *ast.Image:
			handleMarkdownReference(string(nTyped.Destination), addAttachment, addExternal)
		case *ast.AutoLink:
			if nTyped.AutoLinkType != ast.AutoLinkURL {
				break
			}
			// URL() assembles the scheme for <...> autolinks; linkify nodes
			// already carry it in the value. Either way only http(s) counts.
			if url := string(nTyped.URL(src)); isHTTPScheme(url) {
				addExternal(url)
			}
		}
		return ast.WalkContinue, nil
	})

	return refs
}

// handleMarkdownReference classifies a markdown link/image destination as
// either an external http(s) URL or a local attachment.
func handleMarkdownReference(destination string, addAttachment func(AttachmentRef), addExternal func(string)) {
	if isHTTPScheme(destination) {
		addExternal(destination)
		return
	}
	if kind, ok := classifyAttachmentKind(destination); ok {
		addAttachment(AttachmentRef{Target: destination, Kind: kind, Source: SrcMarkdown})
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
