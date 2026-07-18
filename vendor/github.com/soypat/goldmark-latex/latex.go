package latex

import (
	"bytes"
	_ "embed"
	"io"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Config contains parameters for controlling LaTeX output of a Renderer.
type Config struct {
	// Increase heading levels: if the offset is 1, \section (1) becomes \subsection (2) etc.
	// Negative offset is also valid.
	// Resulting levels are clipped between 1 and 6.
	HeadingLevelOffset int
	// Removes section numbering.
	NoHeadingNumbering bool
	// Replace the default preamble by setting this to a non-nil byte slice.
	// Should NOT end with \begin{document}, this is added automatically.
	Preamble []byte
	// If set renderer will render possibly unsafe elements, such as links and
	// code block raw content.
	Unsafe bool
	// Declares all used unicode characters in the preamble
	// and replaces them with the result of this function.
	DeclareUnicode func(rune) (raw string, isReplaced bool)
	// Omits printing of preamble and \begin{document} and \end{document} statements
	NoPreamble bool
	// Enables Quarto-like table captions. Define a table caption by adding a colon prefixed caption below the table.
	EnableTableCaptions bool
	// BibFile is the path to the .bib file passed to \bibliography{}.
	// When non-empty, \bibliographystyle{BibStyle}\bibliography{BibFile} is emitted before \end{document}.
	BibFile string
	// BibStyle is the argument to \bibliographystyle{}. Defaults to "plain" when BibFile is set.
	BibStyle string
	// CiteCmd is the LaTeX command used for citations, e.g. "cite", "citep", "parencite".
	// Defaults to "cite".
	CiteCmd string
}

// SetLatexOption implements the Option interface.
func (r Config) SetLatexOption(c *Config) { *c = r }

// Renderer is a LaTeX renderer implementation for extending
// goldmark to generate .tex files.
type Renderer struct {
	Config Config
}

// An Option interface sets options for HTML based renderers.
type Option interface {
	SetLatexOption(*Config)
}

// TableCaption is an AST node for a Pandoc-style table caption (`: text` paragraph after a table).
type TableCaption struct{ ast.BaseBlock }

// KindTableCaption is the NodeKind for TableCaption nodes.
var KindTableCaption = ast.NewNodeKind("TableCaption")

func (n *TableCaption) Kind() ast.NodeKind { return KindTableCaption }
func (n *TableCaption) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// Citation is an inline AST node representing one or more Pandoc-style citations.
// Syntax: [@key] or [@key1; @key2] — maps to \cite{key} or \cite{key1,key2}.
type Citation struct {
	ast.BaseInline
	// Keys holds the extracted citation keys, e.g. ["key1", "key2"].
	Keys [][]byte
}

// KindCitation is the NodeKind for Citation nodes.
var KindCitation = ast.NewNodeKind("Citation")

func (n *Citation) Kind() ast.NodeKind { return KindCitation }
func (n *Citation) Dump(source []byte, level int) {
	kvs := make(map[string]string, len(n.Keys))
	for i, k := range n.Keys {
		kvs["Key"+strconv.Itoa(i)] = string(k)
	}
	ast.DumpHelper(n, source, level, kvs, nil)
}

// CitationParser is a goldmark inline parser for Pandoc-style citations.
// It recognises [@key] and [@key1; @key2], producing Citation nodes.
// Register it with a priority lower than 200 (goldmark's link parser priority)
// so it runs before the link parser, which otherwise consumes all '[' tokens first.
// Example: util.Prioritized(CitationParser, 150).
var CitationParser parser.InlineParser = &citationParser{}

type citationParser struct{}

func (p *citationParser) Trigger() []byte { return []byte{'['} }

func (p *citationParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 3 || line[1] != '@' {
		return nil
	}
	end := bytes.IndexByte(line, ']')
	if end < 0 {
		return nil
	}
	// inner is "key1; @key2; @key3" (first '@' already consumed by the '[' + '@' check)
	inner := line[2:end]
	parts := bytes.Split(inner, []byte("; @"))
	c := &Citation{}
	for _, part := range parts {
		key := bytes.TrimSpace(part)
		if len(key) > 0 {
			c.Keys = append(c.Keys, key)
		}
	}
	if len(c.Keys) == 0 {
		return nil
	}
	block.Advance(end + 1)
	return c
}

// InlineMath is an inline AST node representing a LaTeX inline math expression.
// Syntax: $content$ — the closing $ must appear on the same line.
type InlineMath struct {
	ast.BaseInline
	Content []byte
}

// KindInlineMath is the NodeKind for InlineMath nodes.
var KindInlineMath = ast.NewNodeKind("InlineMath")

func (n *InlineMath) Kind() ast.NodeKind { return KindInlineMath }
func (n *InlineMath) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Content": string(n.Content)}, nil)
}

// InlineMathParser is a goldmark inline parser for $...$ inline math expressions.
// The closing $ must appear before the end of the current line; unmatched $ signs
// fall through and are escaped to \$ by the renderer.
// Example registration: util.Prioritized(InlineMathParser, 150).
var InlineMathParser parser.InlineParser = &inlineMathParser{}

type inlineMathParser struct{}

func (p *inlineMathParser) Trigger() []byte { return []byte{'$'} }

func (p *inlineMathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	// line[0] == '$'; search for the closing '$' on the same line
	end := bytes.IndexByte(line[1:], '$')
	if end < 0 {
		return nil
	}
	content := line[1 : end+1]
	if len(bytes.TrimSpace(content)) == 0 {
		return nil // reject $$
	}
	block.Advance(end + 2) // consume opening '$', content, and closing '$'
	return &InlineMath{Content: content}
}

// TableCaptionTransformer is a parser.ASTTransformer that converts a `: caption`
// paragraph immediately following a table into a TableCaption node.
// Register it with priority < 0 so it runs after goldmark's table transformer (priority 0).
var TableCaptionTransformer parser.ASTTransformer = &tableCaptionTransformer{}

type tableCaptionTransformer struct{}

func (t *tableCaptionTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	// Two-pass approach: collect candidates first, then replace.
	// Replacing a node during ast.Walk clears its sibling links via ReplaceChild,
	// which would cut the walk short and miss subsequent captions.
	type candidate struct {
		para      ast.Node
		firstText *ast.Text
		seg       text.Segment
	}
	var candidates []candidate
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != ast.KindParagraph {
			return ast.WalkContinue, nil
		}
		prev := n.PreviousSibling()
		if prev == nil || prev.Kind() != extast.KindTable {
			return ast.WalkContinue, nil
		}
		firstChild := n.FirstChild()
		if firstChild == nil || firstChild.Kind() != ast.KindText {
			return ast.WalkContinue, nil
		}
		seg := firstChild.(*ast.Text).Segment
		if !bytes.HasPrefix(seg.Value(reader.Source()), []byte(": ")) {
			return ast.WalkContinue, nil
		}
		candidates = append(candidates, candidate{n, firstChild.(*ast.Text), seg})
		return ast.WalkContinue, nil
	})
	for _, c := range candidates {
		c.firstText.Segment = c.seg.WithStart(c.seg.Start + 2)
		caption := &TableCaption{}
		for child := c.para.FirstChild(); child != nil; {
			next := child.NextSibling()
			caption.AppendChild(caption, child)
			child = next
		}
		c.para.Parent().ReplaceChild(c.para.Parent(), c.para, caption)
	}
}

// NewRenderer returns a new Renderer with given options.
// Options are applied in order of appearance.
// Example:
//
//	lr := latex.NewRenderer(latex.Config{
//		Unsafe: true, // Add desired configuration options.
//	})
//	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(lr, 1000)))
//	md := goldmark.New(goldmark.WithRenderer(r))
//	md.Convert(markdown, LaTeXoutput)
func NewRenderer(opts ...Option) renderer.NodeRenderer {
	r := &Renderer{
		Config: Config{},
	}
	for _, opt := range opts {
		opt.SetLatexOption(&r.Config)
	}
	return r
}

// RegisterFuncs implements goldmark's renderer.NodeRenderer interface.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// blocks
	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)

	// tables (GFM extension)
	reg.Register(KindTableCaption, r.renderTableCaption)
	reg.Register(extast.KindTable, r.renderTable)
	reg.Register(extast.KindTableHeader, r.renderTableHeader)
	reg.Register(extast.KindTableRow, r.renderTableRow)
	reg.Register(extast.KindTableCell, r.renderTableCell)

	// citations
	reg.Register(KindCitation, r.renderCitation)

	// inline math
	reg.Register(KindInlineMath, r.renderInlineMath)

	// inlines
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
}

func (r *Renderer) renderDocument(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		if r.Config.BibFile != "" {
			style := r.Config.BibStyle
			if style == "" {
				style = "plain"
			}
			_, _ = w.WriteString("\\bibliographystyle{")
			_, _ = w.WriteString(style)
			_, _ = w.WriteString("}\n\\bibliography{")
			_, _ = w.WriteString(r.Config.BibFile)
			_, _ = w.WriteString("}\n")
		}
		if !r.Config.NoPreamble {
			w.WriteString("\n\\end{document}\n")
		}
		return ast.WalkStop, nil
	} else if r.Config.NoPreamble {
		return ast.WalkContinue, nil
	}

	if r.Config.Preamble == nil {
		w.Write(defaultPreamble)
	} else {
		w.Write(r.Config.Preamble)
	}

	if r.Config.DeclareUnicode != nil {
		_ = w.WriteByte('\n')
		const unicodeDecl = "\\DeclareUnicodeCharacter{"
		const zeropad = "00"
		declared := make(map[rune]struct{})
		n := len(source)
		i := 0
		for i < n {
			char, lchar := utf8.DecodeRune(source[i:])
			i += lchar
			if lchar == 1 {
				continue // ASCII character.
			}
			if _, ok := declared[char]; ok {
				continue
			}
			declared[char] = struct{}{}
			replace, ok := r.Config.DeclareUnicode(char)
			if !ok {
				continue
			}
			_, _ = w.WriteString(unicodeDecl)
			num := strconv.FormatUint(uint64(char), 16)
			_, _ = w.WriteString(zeropad[:2-(len(num)-2)])
			_, _ = w.WriteString(num)
			_, _ = w.WriteString("}{")
			_, _ = w.WriteString(replace)
			_, _ = w.WriteString("}\n")
		}
	}
	w.WriteString("\n\\begin{document}\n")
	return ast.WalkContinue, nil
}

// Do not modify.
//
//go:embed defaultPreamble.tex
var defaultPreamble []byte

// DefaultPreamble returns a copy of the default preamble provided by goldmark-latex.
// It does not include \begin{document} text within, as expected by Config.Preamble.
func DefaultPreamble() []byte {
	cp := make([]byte, len(defaultPreamble))
	copy(cp, defaultPreamble)
	return cp
}

func (r *Renderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		headingLevel := max(0, min(6, r.Config.HeadingLevelOffset+n.Level-1))
		start := headingTable[headingLevel][bool2int(r.Config.NoHeadingNumbering)]
		_ = w.WriteByte('\n')
		_, _ = w.Write(start)
		if headingLevel >= 5 {
			// _, _ = w.Write(softBreak)
			w.WriteByte('\n')
		}
	} else {
		_, _ = w.WriteString("}\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderBlockquote(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(blockQuoteStart)
	} else {
		_, _ = w.Write(blockQuoteEnd)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(blockCodeStart)
		_ = w.WriteByte('\n')
		r.writeRawLines(w, source, n)
	} else {
		_, _ = w.Write(blockCodeEnd)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	if bytes.Equal(n.Language(source), rawLatexInfo) {
		// Pandoc-style raw LaTeX passthrough: a ```{=latex} fence emits its
		// contents verbatim. This is arbitrary code injection, so it is gated
		// behind Config.Unsafe; otherwise the block is dropped.
		if entering {
			if r.Config.Unsafe {
				_ = w.WriteByte('\n')
				r.writeRawLines(w, source, n)
			} else {
				_, _ = w.WriteString("\n% goldmark-latex: raw LaTeX block skipped; enable Unsafe to emit it\n")
			}
		}
		return ast.WalkContinue, nil
	}
	if entering {
		_, _ = w.Write(blockCodeStart)
		language := n.Language(source)
		language = language[:min(10, len(language))]
		_, supported := supportedLang[string(language)]
		if language != nil && supported {
			_, _ = w.WriteString("[language=")
			escapeLaTeX(w, language)
			_ = w.WriteByte(']')
		}
		_ = w.WriteByte('\n')
		r.writeRawLines(w, source, n)
	} else {
		_, _ = w.Write(blockCodeEnd)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	w.WriteString("\n% goldmark-latex: HTML block rendering unsupported, skipped\n")
	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderList(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.List)
	tag := "itemize"
	if n.IsOrdered() {
		tag = "enumerate"
	}
	if entering {
		_, _ = w.WriteString("\n\\begin{")
		_, _ = w.WriteString(tag)
		_, _ = w.WriteString("}\n")
	} else {
		_, _ = w.WriteString("\\end{")
		_, _ = w.WriteString(tag)
		_, _ = w.WriteString("}\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderListItem(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(itemCommand)
		fc := n.FirstChild()
		if fc != nil {
			if _, ok := fc.(*ast.TextBlock); !ok {
				// _ = w.WriteByte('\n')
			}
		}
	} else {
		_ = w.WriteByte('\n')
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderParagraph(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		// A paragraph holding only an image renders as a figure float; a
		// trailing \\ after \end{figure} is invalid LaTeX, so suppress it.
		if c := n.FirstChild(); c != nil && c == n.LastChild() && c.Kind() == ast.KindImage {
			return ast.WalkContinue, nil
		}
		parent := n.Parent()
		pkind := parent.Kind()
		if pkind != ast.KindList && pkind != ast.KindListItem {
			_, _ = w.Write(hardBreak)
		} else {
			_, _ = w.WriteString("\n\n")
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTextBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		if n.NextSibling() != nil && n.FirstChild() != nil {
			_ = w.WriteByte('\n')
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderThematicBreak(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(hruleCommand)
		_ = w.WriteByte('\n')
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*extast.Table)
	hasCaption := r.Config.EnableTableCaptions && node.NextSibling() != nil && node.NextSibling().Kind() == KindTableCaption
	if entering {
		if hasCaption {
			_, _ = w.WriteString("\n\\begin{table}[h!]\n\\centering\n")
		}
		_, _ = w.WriteString("\\begin{tabular}{")
		for _, align := range n.Alignments {
			switch align {
			case extast.AlignRight:
				_ = w.WriteByte('r')
			case extast.AlignCenter:
				_ = w.WriteByte('c')
			default:
				_ = w.WriteByte('l')
			}
		}
		_, _ = w.WriteString("}\n\\hline\n")
	} else {
		_, _ = w.WriteString("\\hline\n\\end{tabular}\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderCitation(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*Citation)
	cmd := r.Config.CiteCmd
	if cmd == "" {
		cmd = "cite"
	}
	_, _ = w.WriteString("\\")
	_, _ = w.WriteString(cmd)
	_ = w.WriteByte('{')
	for i, key := range n.Keys {
		if i > 0 {
			_ = w.WriteByte(',')
		}
		_, _ = w.Write(key)
	}
	_ = w.WriteByte('}')
	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderInlineMath(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*InlineMath)
	_ = w.WriteByte('$')
	_, _ = w.Write(n.Content)
	_ = w.WriteByte('$')
	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderTableCaption(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !r.Config.EnableTableCaptions {
		return ast.WalkSkipChildren, nil
	}
	if entering {
		_, _ = w.WriteString("\\caption{")
	} else {
		_, _ = w.WriteString("}\n\\end{table}\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTableHeader(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString(" \\\\\n\\hline\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTableRow(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString(" \\\\\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderTableCell(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering && n.PreviousSibling() != nil {
		_, _ = w.WriteString(" & ")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}
	url := n.URL(source)
	label := n.Label(source)
	_, _ = w.WriteString("\\href{")
	if n.AutoLinkType == ast.AutoLinkEmail && haslowerprefix(url, mailToPrefix) {
		_, _ = w.WriteString("mailto:")
	}
	escLink(w, url)
	_, _ = w.WriteString("}{")
	escapeLaTeX(w, label)
	_ = w.WriteByte('}')
	return ast.WalkContinue, nil
}

// haslowerprefix is an allocation free implementation of
//
//	bytes.HasPrefix(bytes.ToLower(a), bytes.ToLower(b))
func haslowerprefix(a, b []byte) bool {
	n := min(len(a), len(b))
	i := 0
	for i < n {
		ra, la := utf8.DecodeRune(a[i:])
		rb, lb := utf8.DecodeRune(b[i:])
		if la != lb || unicode.ToLower(ra) != unicode.ToLower(rb) {
			return false
		}
		i += la
	}
	return true
}

func (r *Renderer) renderCodeSpan(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_ = w.WriteByte('}')
		return ast.WalkContinue, nil
	}

	// Render all children within code span. Should all be Text kind.
	_, _ = w.Write(codeSpanStart)
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		segment := c.(*ast.Text).Segment
		value := segment.Value(source)
		if bytes.HasSuffix(value, []byte("\n")) {
			escapeLaTeX(w, value[:len(value)-1])
			_ = w.WriteByte(' ')
		} else {
			escapeLaTeX(w, value)
		}
	}
	return ast.WalkSkipChildren, nil // Skip all of them after rendering.
}

func (r *Renderer) renderEmphasis(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		const (
			emph  = "\\textit{"
			bold  = "\\textbf{"
			emph3 = "\\emph{"
		)
		n := node.(*ast.Emphasis)
		tag := emph
		switch n.Level {
		case 2:
			tag = bold
		case 3:
			tag = emph3
		}
		w.WriteString(tag)
	} else {
		w.WriteByte('}')
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		_, _ = w.WriteString(`\href{`)
		if r.Config.Unsafe || !html.IsDangerousURL(n.Destination) {
			escapeLaTeX(w, n.Destination)
			// _, _ = w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
		}
		_, _ = w.WriteString("}{")
	} else {
		_ = w.WriteByte('}')
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Image)
	if !r.Config.Unsafe && html.IsDangerousURL(n.Destination) {
		return ast.WalkSkipChildren, nil
	}
	// The alt text (the node's children) becomes the figure caption.
	hasCaption := n.HasChildren()
	if entering {
		_, _ = w.WriteString("\n\\begin{figure}[h]\n\\centering\n\\includegraphics[width=\\textwidth]{")
		// The destination is a file path and must be written literally;
		// LaTeX-escaping it would corrupt characters such as '_' in filenames.
		_, _ = w.Write(n.Destination)
		_, _ = w.WriteString("}\n")
		if hasCaption {
			_, _ = w.WriteString("\\caption{")
		}
	} else {
		if hasCaption {
			_, _ = w.WriteString("}\n")
		}
		_, _ = w.WriteString("\\end{figure}\n")
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// No rawHTML rendering supported
	w.WriteString("\n% goldmark-latex: raw HTML rendering unsupported\n")
	return ast.WalkSkipChildren, nil
}

func (r *Renderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment.Value(source)
	if n.IsRaw() {
		w.Write(segment)
		// r.Writer.RawWrite(w, segment.Value(source))
	} else {
		escapeLaTeX(w, segment)
		if n.HardLineBreak() {
			_, _ = w.Write(hardBreak)
		} else if n.SoftLineBreak() {
			// _, _ = w.Write(softBreak)
			_ = w.WriteByte('\n')
		}
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) renderString(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)
	if n.IsCode() || n.IsRaw() {
		_, _ = w.Write(n.Value)
	} else {
		escapeLaTeX(w, n.Value)
	}
	return ast.WalkContinue, nil
}

func (r *Renderer) writeRawLines(w util.BufWriter, source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		text := line.Value(source)
		if r.Config.Unsafe || !bytes.Contains(text, endCmdPrefix) {
			_, _ = w.Write(text)
		} else {
			_, _ = w.WriteString("% goldmark-latex: Skipped following line due to possibly unsafe content:\n%")
			_, _ = w.Write(text)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

var (
	endCmdPrefix    = []byte("\\end")
	rawLatexInfo    = []byte("{=latex}")
	mailToPrefix    = []byte(":mailto")
	hardBreak       = []byte("\\\\\n\n")
	codeSpanStart   = []byte("\\texttt{")
	blockQuoteStart = []byte("\n\\begin{framed}\n\\begin{quote}\n")
	blockQuoteEnd   = []byte("\\end{quote}\n\\end{framed}\n")
	blockCodeStart  = []byte("\n\\begin{lstlisting}")
	blockCodeEnd    = []byte("\\end{lstlisting}\n")
	hruleCommand    = []byte("\n\\hrulefill\n")

	itemCommand  = []byte("\\item~ ")
	headingTable = [6][2][]byte{
		// {[]byte("\\part{"), []byte("\\part*{")},
		// {[]byte("\\chapter{"), []byte("\\chapter*{")},
		{[]byte("\\section{"), []byte("\\section*{")},
		{[]byte("\\subsection{"), []byte("\\subsection*{")},
		{[]byte("\\subsubsection{"), []byte("\\subsubsection*{")},
		{[]byte("\\paragraph{"), []byte("\\paragraph*{")},
		{[]byte("\\subparagraph{"), []byte("\\subparagraph*{")},
		{[]byte("\\textbf{"), []byte("\\textbf{")},
	}
)

var escapeTable = [256][]byte{
	'\\': []byte("\\textbackslash~"),
	'~':  []byte("\\textasciitilde~"),
	'^':  []byte("\\textasciicircum~"),
	'&':  []byte("\\&"),
	'%':  []byte("\\%"),
	'$':  []byte("\\$"),
	'#':  []byte("\\#"),
	'_':  []byte("\\_"),
	'{':  []byte("\\{"),
	'}':  []byte("\\}"),
}

func escapeLaTeX(w io.Writer, s []byte) {
	var start, end int
	for end < len(s) {
		escSeq := escapeTable[s[end]]
		if escSeq != nil {
			w.Write(s[start:end])
			w.Write(escSeq)
			start = end + 1
		}
		end++
	}
	if start < len(s) && end <= len(s) {
		w.Write(s[start:end])
	}
}

func escLink(w io.Writer, text []byte) {
	escapeLaTeX(w, text)
}

// Languages supported by lstlisting.
// Generated with the following program with http://mirrors.ctan.org/macros/latex/contrib/listings/lstdrvrs.dtx.
//
//	r := strings.NewReader(a)
//	scan := bufio.NewScanner(r)
//	re := regexp.MustCompile(`\{[A-Za-z0-9]*\}`)
//	found := make(map[string]bool)
//	for scan.Scan() {
//		line := scan.Text()
//		a := strings.LastIndex("{", line)
//		if a > 1 {
//			line = line[a-1:]
//		}
//		got := re.FindString(line)
//		if len(got) > 2 {
//			lang := strings.ToLower(got[1 : len(got)-1])
//			if !found[lang] {
//				fmt.Printf("\"%s\":{},\n", lang)
//				found[lang] = true
//			}
//		}
//	}
var supportedLang = map[string]struct{}{
	"abap":        {},
	"acm":         {},
	"acmscript":   {},
	"acsl":        {},
	"ada":         {},
	"algol":       {},
	"assembler":   {},
	"awk":         {},
	"basic":       {},
	"clean":       {},
	"idl":         {},
	"c":           {},
	"caml":        {},
	"cil":         {},
	"cobol":       {},
	"comsol":      {},
	"csh":         {},
	"bash":        {},
	"sh":          {},
	"delphi":      {},
	"eiffel":      {},
	"elan":        {},
	"erlang":      {},
	"euphoria":    {},
	"fortran":     {},
	"gap":         {},
	"go":          {},
	"gcl":         {},
	"gnuplot":     {},
	"hansl":       {},
	"haskell":     {},
	"html":        {},
	"inform":      {},
	"java":        {},
	"jvmis":       {},
	"scala":       {},
	"ksh":         {},
	"lingo":       {},
	"lisp":        {},
	"elisp":       {},
	"llvm":        {},
	"logo":        {},
	"lua":         {},
	"make":        {},
	"matlab":      {},
	"mathematica": {},
	"mercury":     {},
	"metapost":    {},
	"miranda":     {},
	"mizar":       {},
	"ml":          {},
	"mupad":       {},
	"nastran":     {},
	"ocl":         {},
	"octave":      {},
	"oz":          {},
	"pascal":      {},
	"perl":        {},
	"php":         {},
	"plasm":       {},
	"postscript":  {},
	"pov":         {},
	"prolog":      {},
	"promela":     {},
	"pstricks":    {},
	"python":      {},
	"rexx":        {},
	"oorexx":      {},
	"reduce":      {},
	"rsl":         {},
	"ruby":        {},
	"scilab":      {},
	"shelxl":      {},
	"simula":      {},
	"sparql":      {},
	"sql":         {},
	"swift":       {},
	"tcl":         {},
	"s":           {},
	"r":           {},
	"sas":         {},
	"tex":         {},
	"vbscript":    {},
	"verilog":     {},
	"vhdl":        {},
	"vrml":        {},
	"xslt":        {},
	"ant":         {},
	"xml":         {},
}
