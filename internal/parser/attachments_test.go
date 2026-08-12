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
		want []Ref
	}{
		{name: "image embed with size", body: "![[img.png|200]]", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "pdf plain link", body: "[[doc.pdf]]", want: []Ref{{Target: "doc.pdf", Kind: KindPDF, Source: SrcWiki}}},
		{name: "image embed with alias", body: "![[img.png|alt text]]", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "image with heading anchor", body: "[[img.png#anchor]]", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "subfolder image embed", body: "![[sub/img.png]]", want: []Ref{{Target: "sub/img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "relative dot prefix", body: "[[./local.png]]", want: []Ref{{Target: "./local.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "archive attachment", body: "[[bundle.zip]]", want: []Ref{{Target: "bundle.zip", Kind: KindOther, Source: SrcWiki}}},
		{name: "canvas attachment", body: "[[diagram.canvas]]", want: []Ref{{Target: "diagram.canvas", Kind: KindOther, Source: SrcWiki}}},
		{name: "unknown extension is not an attachment", body: "[[archive.xyz]]", want: nil},
		{name: "dotted note name is not an attachment", body: "[[Note 1.2.3]]", want: nil},
		{name: "plain note link is not an attachment", body: "[[Other Note]]", want: nil},
		{name: "uppercase extension is case-insensitive", body: "![[PHOTO.PNG]]", want: []Ref{{Target: "PHOTO.PNG", Kind: KindImage, Source: SrcWiki}}},
		{name: "duplicate embeds dedupe", body: "![[img.png]]\n\n![[img.png|200]]", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcWiki}}},
		{name: "code fence contents ignored", body: "```\n![[x.png]]\n[[secret.pdf]]\n```", want: nil},
		{name: "inline code ignored", body: "`![[x.png]]`", want: nil},
		{name: "empty target ignored", body: "[[#heading]]", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.Refs, tt.want) {
				t.Errorf("ExtractReferences(%q).Refs = %v, want %v", tt.body, got.Refs, tt.want)
			}
		})
	}
}

func TestExtractReferences_MarkdownAttachments(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Ref
	}{
		{name: "image", body: "![alt](img.png)", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcMarkdown}}},
		{name: "pdf in subfolder", body: "[doc](sub/file.pdf)", want: []Ref{{Target: "sub/file.pdf", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "percent-encoded destination kept raw", body: "![alt](Router%20Modes.webp)", want: []Ref{{Target: "Router%20Modes.webp", Kind: KindImage, Source: SrcMarkdown}}},
		{name: "parent traversal kept raw", body: "[x](../up.pdf)", want: []Ref{{Target: "../up.pdf", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "pdf with page fragment", body: "[x](STP.pdf#page=5)", want: []Ref{{Target: "STP.pdf#page=5", Kind: KindPDF, Source: SrcMarkdown}}},
		{name: "external link is not an attachment", body: "[text](https://example.com)", want: []Ref{{Target: "https://example.com", Kind: KindExternalLinks}}},
		{name: "relative note link without extension", body: "[rel](../other-note)", want: nil},
		{name: "anchor-only link", body: "[x](#anchor)", want: nil},
		{name: "code fence contents ignored", body: "```\n![x](img.png)\n```", want: nil},
		{name: "duplicate destinations dedupe", body: "![a](img.png) and ![b](img.png)", want: []Ref{{Target: "img.png", Kind: KindImage, Source: SrcMarkdown}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.Refs, tt.want) {
				t.Errorf("ExtractReferences(%q).Refs = %v, want %v", tt.body, got.Refs, tt.want)
			}
		})
	}
}

func TestExtractReferences_External(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Ref
	}{
		{name: "markdown link", body: "[text](https://example.com/a)", want: []Ref{{Target: "https://example.com/a", Kind: KindExternalLinks}}},
		{name: "markdown image embed", body: "![alt](https://example.com/i.png)", want: []Ref{{Target: "https://example.com/i.png", Kind: KindExternalLinks}}},
		{name: "bare url", body: "see https://example.com here", want: []Ref{{Target: "https://example.com", Kind: KindExternalLinks}}},
		{name: "angle url", body: "<https://example.com>", want: []Ref{{Target: "https://example.com", Kind: KindExternalLinks}}},
		{name: "bare www url gains http protocol", body: "visit www.example.com", want: []Ref{{Target: "http://www.example.com", Kind: KindExternalLinks}}},
		{name: "wikilink to external url", body: "[[https://example.com]]", want: []Ref{{Target: "https://example.com", Kind: KindExternalLinks}}},
		{name: "wikilink to external image url", body: "[[https://example.com/img.png]]", want: []Ref{{Target: "https://example.com/img.png", Kind: KindExternalLinks}}},
		{name: "multiple urls keep first occurrence order", body: "[b](https://b.org)\n\n[a](https://a.org) and https://b.org", want: []Ref{
			{Target: "https://b.org", Kind: KindExternalLinks},
			{Target: "https://a.org", Kind: KindExternalLinks},
		}},
		{name: "code fence contents ignored", body: "```\nhttps://example.com\n```", want: nil},
		{name: "excluded schemes", body: "[mail](mailto:a@b.c)\n\n[ftp](ftp://x.y/z)\n\nemail me at a@b.c", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferences(tt.body)
			if !reflect.DeepEqual(got.Refs, tt.want) {
				t.Errorf("ExtractReferences(%q).Refs = %v, want %v", tt.body, got.Refs, tt.want)
			}
		})
	}
}

func TestExtractReferences_CrossKindFirstOccurrenceOrder(t *testing.T) {
	body := "![[cover.png]] and [ext1](https://example.com) and ![[second.png]] and [ext2](https://links.example.com)"
	got := ExtractReferences(body)
	want := []Ref{
		{Target: "cover.png", Kind: KindImage, Source: SrcWiki},
		{Target: "https://example.com", Kind: KindExternalLinks},
		{Target: "second.png", Kind: KindImage, Source: SrcWiki},
		{Target: "https://links.example.com", Kind: KindExternalLinks},
	}
	if !reflect.DeepEqual(got.Refs, want) {
		t.Errorf("Refs = %v, want %v (cross-kind first-occurrence order)", got.Refs, want)
	}
}

func TestExtractReferences_Mixed(t *testing.T) {
	body := "![[cover.png]] and [doc](manual.pdf) and https://example.com and [[https://links.example.com]]"
	got := ExtractReferences(body)
	want := []Ref{
		{Target: "cover.png", Kind: KindImage, Source: SrcWiki},
		{Target: "manual.pdf", Kind: KindPDF, Source: SrcMarkdown},
		{Target: "https://example.com", Kind: KindExternalLinks},
		{Target: "https://links.example.com", Kind: KindExternalLinks},
	}
	if !reflect.DeepEqual(got.Refs, want) {
		t.Errorf("Refs = %v, want %v", got.Refs, want)
	}
}
