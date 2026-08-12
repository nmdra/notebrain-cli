// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package parser

import (
	"reflect"
	"testing"
)

func TestExtractReferences_WikiAttachments(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []AttachmentRef
	}{
		{name: "image embed with size", body: "![[img.png|200]]", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "pdf plain link", body: "[[doc.pdf]]", want: []AttachmentRef{{Target: "doc.pdf", Kind: KindPDF, Source: SrcWiki}}},
		{name: "image embed with alias", body: "![[img.png|alt text]]", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "image with heading anchor", body: "[[img.png#anchor]]", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "subfolder image embed", body: "![[sub/img.png]]", want: []AttachmentRef{{Target: "sub/img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "relative dot prefix", body: "[[./local.png]]", want: []AttachmentRef{{Target: "./local.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "archive attachment", body: "[[bundle.zip]]", want: []AttachmentRef{{Target: "bundle.zip", Kind: KindOther, Source: SrcWiki}}},
		{name: "canvas attachment", body: "[[diagram.canvas]]", want: []AttachmentRef{{Target: "diagram.canvas", Kind: KindOther, Source: SrcWiki}}},
		{name: "unknown extension is not an attachment", body: "[[archive.xyz]]", want: nil},
		{name: "dotted note name is not an attachment", body: "[[Note 1.2.3]]", want: nil},
		{name: "plain note link is not an attachment", body: "[[Other Note]]", want: nil},
		{name: "uppercase extension is case-insensitive", body: "![[PHOTO.PNG]]", want: []AttachmentRef{{Target: "PHOTO.PNG", Kind: KindImage, Source: SrcWiki}}},
		{name: "duplicate embeds dedupe", body: "![[img.png]]\n\n![[img.png|200]]", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "code fence contents ignored", body: "```\n![[x.png]]\n[[secret.pdf]]\n```", want: nil},
		{name: "inline code ignored", body: "`![[x.png]]`", want: nil},
		{name: "empty target ignored", body: "[[#heading]]", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.Attachments, tt.want) {
				t.Errorf("ExtractReferences(%q).Attachments = %v, want %v", tt.body, got.Attachments, tt.want)
			}
			if len(got.External) != 0 {
				t.Errorf("ExtractReferences(%q).External = %v, want none", tt.body, got.External)
			}
		})
	}
}

func TestExtractReferences_MarkdownAttachments(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []AttachmentRef
	}{
		{name: "image", body: "![alt](img.png)", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcMarkdown}}},
		{name: "pdf in subfolder", body: "[doc](sub/file.pdf)", want: []AttachmentRef{{Target: "sub/file.pdf", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "percent-encoded destination kept raw", body: "![alt](Router%20Modes.webp)", want: []AttachmentRef{{Target: "Router%20Modes.webp", Kind: KindImage, Source: SrcMarkdown}}},
		{name: "parent traversal kept raw", body: "[x](../up.pdf)", want: []AttachmentRef{{Target: "../up.pdf", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "pdf with page fragment", body: "[x](STP.pdf#page=5)", want: []AttachmentRef{{Target: "STP.pdf#page=5", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "external link is not an attachment", body: "[text](https://example.com)", want: nil},
		{name: "relative note link without extension", body: "[rel](../other-note)", want: nil},
		{name: "anchor-only link", body: "[x](#anchor)", want: nil},
		{name: "code fence contents ignored", body: "```\n![x](img.png)\n```", want: nil},
		{name: "duplicate destinations dedupe", body: "![a](img.png) and ![b](img.png)", want: []AttachmentRef{{Target: "img.png", Kind: KindImage, Source: SrcMarkdown}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.Attachments, tt.want) {
				t.Errorf("ExtractReferences(%q).Attachments = %v, want %v", tt.body, got.Attachments, tt.want)
			}
		})
	}
}

func TestExtractReferences_External(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "markdown link", body: "[text](https://example.com/a)", want: []string{"https://example.com/a"}},
		{name: "markdown image embed", body: "![alt](https://example.com/i.png)", want: []string{"https://example.com/i.png"}},
		{name: "bare url", body: "see https://example.com here", want: []string{"https://example.com"}},
		{name: "angle url", body: "<https://example.com>", want: []string{"https://example.com"}},
		{name: "bare www url gains http protocol", body: "visit www.example.com", want: []string{"http://www.example.com"}},
		{name: "wikilink to external url", body: "[[https://example.com]]", want: []string{"https://example.com"}},
		{name: "wikilink to external image url", body: "[[https://example.com/img.png]]", want: []string{"https://example.com/img.png"}},
		{name: "multiple urls keep first occurrence order", body: "[b](https://b.org)\n\n[a](https://a.org) and https://b.org", want: []string{"https://b.org", "https://a.org"}},
		{name: "code fence contents ignored", body: "```\nhttps://example.com\n```", want: nil},
		{name: "excluded schemes", body: "[mail](mailto:a@b.c)\n\n[ftp](ftp://x.y/z)\n\nemail me at a@b.c", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.External, tt.want) {
				t.Errorf("ExtractReferences(%q).External = %v, want %v", tt.body, got.External, tt.want)
			}
			if len(got.Attachments) != 0 {
				t.Errorf("ExtractReferences(%q).Attachments = %v, want none", tt.body, got.Attachments)
			}
		})
	}
}

func TestExtractReferences_Mixed(t *testing.T) {
	body := "![[cover.png]] and [doc](manual.pdf) and https://example.com and [[https://links.example.com]]"
	got := ExtractReferences(body)
	wantAttach := []AttachmentRef{
		{Target: "cover.png", Kind: KindImage, Source: SrcWiki},
		{Target: "manual.pdf", Kind: KindPDF, Source: SrcMarkdown},
	}
	if !reflect.DeepEqual(got.Attachments, wantAttach) {
		t.Errorf("Attachments = %v, want %v", got.Attachments, wantAttach)
	}
	wantExt := []string{"https://example.com", "https://links.example.com"}
	if !reflect.DeepEqual(got.External, wantExt) {
		t.Errorf("External = %v, want %v", got.External, wantExt)
	}
}
