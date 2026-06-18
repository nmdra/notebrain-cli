package hashtag

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Extender extends a goldmark Markdown object with support for parsing and
// rendering hashtags.
//
// Install it on your Markdown object upon creation.
//
//	goldmark.New(
//	  goldmark.WithExtensions(
//	    // ...
//	    &hashtag.Extender{...},
//	  ),
//	  // ...
//	)
//
// Provide a Resolver to render tags as links that point to a specific
// destination.
type Extender struct {
	// Resolver specifies destination links for hashtags, if any.
	//
	// Defaults to no links.
	Resolver Resolver

	// Variant is the flavor of the hashtag syntax to support.
	//
	// Defaults to DefaultVariant. See the documentation of individual
	// variants for more information.
	Variant Variant

	// Attributes are added to the <a> tag.
	//
	// Attributes will only be applied if the tag can be resolved by the Resolver.
	// Defaults to no attributes.
	Attributes []Attribute
}

var _ goldmark.Extender = (*Extender)(nil)

// Extend extends the provided goldmark Markdown object with support for
// hashtags.
func (e *Extender) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(&Parser{
				Variant: e.Variant,
			}, 999),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&Renderer{
				Resolver:   e.Resolver,
				Attributes: e.Attributes,
			}, 999),
		),
	)
}
