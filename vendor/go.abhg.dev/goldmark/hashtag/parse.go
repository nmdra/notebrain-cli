package hashtag

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/forPelevin/gomoji"
	"github.com/rivo/uniseg"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Parser is a Goldmark inline parser for parsing hashtag nodes.
//
// Hashtags start with "#". The list of other characters allowed in the hashtag
// is determined by variant. See the documentation for Variant for more
// details.
type Parser struct {
	// Variant is the flavor of the hashtag syntax to support.
	//
	// Defaults to DefaultVariant. See the documentation of individual
	// variants for more information.
	Variant Variant
}

// Variant represents one of the different flavours of hashtag syntax.
type Variant uint

const (
	// DefaultVariant is the default flavor of hashtag syntax supported by
	// this package.
	//
	// In this format, hashtags start with "#" and an alphabet, followed by
	// zero or more alphanumeric characters and the following symbols.
	//
	//   /_-
	DefaultVariant Variant = iota

	// ObsidianVariant is a flavor of the hashtag syntax that aims to be
	// compatible with Obsidian (https://obsidian.md/).
	//
	// In this format, hashtags start with "#" followed by alphabets,
	// numbers, emoji, or any of the following symbols.
	//
	//   /_-
	//
	// Hashtags cannot be entirely numeric and must contain at least one
	// non-numeric character.
	//
	// See also https://help.obsidian.md/How+to/Working+with+tags.
	ObsidianVariant
)

// span returns the index in the provided string at which the hashtag for this
// variant ends, or -1 if this is not a valid hashtag string.
//
// s must be the part of the hashtag *after* the "#".
func (v Variant) span(tag []byte) int {
	switch v {
	case ObsidianVariant:
		// Tags cannot contain spaces, so if there's a space, that's
		// the furthest our tag edge can be. This helps avoid trying to
		// walk the entire string with uniseg.Graphemes.
		if idx := bytes.IndexFunc(tag, unicode.IsSpace); idx >= 0 {
			tag = tag[:idx]
		}
		end := len(tag)

		gr := uniseg.NewGraphemes(string(tag))
		for gr.Next() {
			if endOfObsidianHashtag(gr) {
				end, _ = gr.Positions()
				break
			}
		}

		// If there isn't at least one non-numeric character,
		// this isn't a valid tag.
		if i := bytes.IndexFunc(tag[:end], nonNumeric); i < 0 {
			return -1
		}

		return end

	default:
		// Hashtag must start with a letter.
		start, sz := utf8.DecodeRune(tag)
		if !unicode.IsLetter(start) {
			return -1
		}
		tag = tag[sz:]

		// If the end of the tag is visible, that's the end index.
		// Otherwise, it's the rest of the string.
		if i := bytes.IndexFunc(tag, endOfHashtag); i >= 0 {
			return i + sz // (+ first letter)
		}
		return len(tag) + sz // (+ first letter)
	}
}

var _ parser.InlineParser = (*Parser)(nil)

var _hash = byte('#')

// Trigger reports characters that trigger this parser.
func (*Parser) Trigger() []byte {
	return []byte{_hash}
}

// Parse parses a hashtag node.
func (p *Parser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()

	if len(line) == 0 || line[0] != _hash {
		return nil
	}
	line = line[1:]

	end := p.Variant.span(line)
	if end < 0 {
		return nil
	}
	seg = seg.WithStop(seg.Start + end + 1) // + '#'

	n := Node{
		Tag: block.Value(seg.WithStart(seg.Start + 1)), // omit the "#"
	}
	n.AppendChild(&n, ast.NewTextSegment(seg))
	block.Advance(seg.Len())
	return &n
}

func nonNumeric(r rune) bool {
	return !unicode.IsDigit(r)
}

func endOfHashtag(r rune) bool {
	return !unicode.IsLetter(r) &&
		!unicode.IsDigit(r) &&
		r != '_' && r != '-' && r != '/'
}

func endOfObsidianHashtag(gr *uniseg.Graphemes) bool {
	rs := gr.Runes()
	return len(rs) == 1 && endOfHashtag(rs[0]) && !gomoji.ContainsEmoji(gr.Str())
}
