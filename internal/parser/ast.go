package parser

import (
	"bytes"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	latex "github.com/soypat/goldmark-latex"
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/hashtag"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
)

// Chunk is one section of a note, bounded by heading structure.
type Chunk struct {
	NoteSlug string
	Index    int
	// Text and RichText are the overlap-free display text used for storage and
	// retrieval output. Text is clean prose (code blocks replaced with
	// placeholders); RichText keeps actual code and markdown inline.
	Text     string
	RichText string
	// EmbedText is the text fed to the embedding model. For split sections it
	// includes the configured chunk-overlap so sentence-level continuity across
	// sub-chunk boundaries survives embedding. It is never stored or displayed.
	EmbedText   string
	HeadingPath string // e.g. "Architecture > Data Flow > Ingest"
	Level       int    // depth of the deepest heading in this chunk (1-6)
	HasTask     bool
	HasCode     bool // true when the chunk contains a fenced code block
}

// Result is the output from parsing the full document, containing the chunks and metadata.
type Result struct {
	Chunks      []Chunk
	Tags        []string
	Links       []string
	Frontmatter map[string]any
}

// mdParser is the shared goldmark instance configured with GFM, hashtags, wikilinks, metadata, footnotes, mermaid, and LaTeX.
var mdParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		&hashtag.Extender{
			Variant: hashtag.ObsidianVariant,
		},
		&wikilink.Extender{},
		&mermaid.Extender{},
		meta.New(meta.WithStoresInDocument()),
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
		parser.WithInlineParsers(
			util.Prioritized(latex.InlineMathParser, 500),
		),
		parser.WithASTTransformers(
			util.Prioritized(&metadataTransformer{}, 100),
		),
	),
)

var (
	tagsContextKey     = parser.NewContextKey()
	linksContextKey    = parser.NewContextKey()
	skipAttachmentsKey = parser.NewContextKey()
)

const (
	blockKindCode      = "code"
	blockKindParagraph = "paragraph"
	blockKindTaskList  = "task_list"
)

type metadataTransformer struct{}

func (t *metadataTransformer) Transform(node *ast.Document, _ text.Reader, pc parser.Context) {
	tagsSet := make(map[string]struct{})
	linksSet := make(map[string]struct{})

	skipAttachments, _ := pc.Get(skipAttachmentsKey).(bool)

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch nTyped := n.(type) {
		case *hashtag.Node:
			tagsSet[string(nTyped.Tag)] = struct{}{}
		case *wikilink.Node:
			target := string(nTyped.Target)
			isAttachment := IsAttachmentLink(target)
			// An embed of an attachment includes its content rather than
			// referencing a note; it is never part of the graph.
			if nTyped.Embed && isAttachment {
				return ast.WalkContinue, nil
			}
			if !skipAttachments || !isAttachment {
				linksSet[target] = struct{}{}
			}
		}
		return ast.WalkContinue, nil
	})

	pc.Set(tagsContextKey, tagsSet)
	pc.Set(linksContextKey, linksSet)
}

// Parse parses body text into Chunks, extracting wikilinks, tags, and frontmatter.
// maxChunkRunes controls the maximum rune length per chunk. overlapRunes controls how many
// runes are repeated at the start of the next sub-chunk when a section is split (overlap).
// If skipAttachments is true, links pointing to non-markdown attachments (images, PDFs, etc.) are ignored.
func Parse(body, noteSlug string, maxChunkRunes, overlapRunes int, skipAttachments bool) Result {
	src := []byte(body)
	reader := text.NewReader(src)
	pc := parser.NewContext()
	pc.Set(skipAttachmentsKey, skipAttachments)

	doc := mdParser.Parser().Parse(reader, parser.WithContext(pc))

	// Extract frontmatter metadata stored by goldmark-meta
	var frontmatter map[string]any
	if md := doc.OwnerDocument().Meta(); md != nil {
		frontmatter = md
	}

	// Retrieve tags and links collected by metadataTransformer
	tagsSet, _ := pc.Get(tagsContextKey).(map[string]struct{})
	if tagsSet == nil {
		tagsSet = make(map[string]struct{})
	}
	linksSet, _ := pc.Get(linksContextKey).(map[string]struct{})
	if linksSet == nil {
		linksSet = make(map[string]struct{})
	}

	finalTagsSet := make(map[string]struct{})
	for t := range tagsSet {
		finalTagsSet[strings.ToLower(t)] = struct{}{}
	}

	fmTags := extractFrontmatterTags(frontmatter)
	for _, t := range fmTags {
		finalTagsSet[strings.ToLower(t)] = struct{}{}
	}

	tags := make([]string, 0, len(finalTagsSet))
	for t := range finalTagsSet {
		tags = append(tags, t)
	}
	slices.Sort(tags)

	links := make([]string, 0, len(linksSet))
	for l := range linksSet {
		links = append(links, l)
	}
	slices.Sort(links)

	// Extract chunks using section logic
	var inlineRegistry []inlineInfo
	sections := extractSections(doc, src, &inlineRegistry)
	chunks := buildChunks(sections, noteSlug, maxChunkRunes, overlapRunes, inlineRegistry)

	return Result{
		Chunks:      chunks,
		Tags:        tags,
		Links:       links,
		Frontmatter: frontmatter,
	}
}

// section is one heading-delimited block of content.
type section struct {
	headingPath string // full breadcrumb path
	level       int
	blocks      []block
}

// block is one parsed block element.
type block struct {
	kind     string // "paragraph", "code", "table", "task_list", "blockquote", "other"
	text     string // plain prose text (empty for code blocks)
	codeText string // raw code (only for "code" kind)
	language string // code fence language hint
}

func extractSections(doc ast.Node, src []byte, registry *[]inlineInfo) []section {
	e := &sectionExtractor{
		src:          src,
		registry:     registry,
		headingStack: make([]string, 7),
	}
	if err := ast.Walk(doc, e.walk); err != nil {
		slog.Warn("ast walk encountered error during section extraction", "err", err)
	}
	if e.current != nil && len(e.current.blocks) > 0 {
		e.sections = append(e.sections, *e.current)
	}
	return e.sections
}

type sectionExtractor struct {
	src          []byte
	registry     *[]inlineInfo
	headingStack []string
	sections     []section
	current      *section
}

func (e *sectionExtractor) ensureCurrent() {
	if e.current == nil {
		e.current = &section{headingPath: "", level: 0}
	}
}

func (e *sectionExtractor) walk(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	switch node := n.(type) {
	case *ast.Heading:
		return e.processHeading(node)
	case *ast.FencedCodeBlock:
		return e.processFencedCode(node)
	case *mermaid.Block:
		return e.processMermaid(node)
	case *ast.Paragraph:
		return e.processParagraph(node)
	case *ast.List:
		return e.processList(node)
	case *extast.Table:
		return e.processTable(node)
	case *ast.Blockquote:
		return e.processBlockquote(node)
	}
	return ast.WalkContinue, nil
}

func (e *sectionExtractor) processHeading(node *ast.Heading) (ast.WalkStatus, error) {
	if e.current != nil && len(e.current.blocks) > 0 {
		e.sections = append(e.sections, *e.current)
	}
	headingText := extractPlainText(node, e.src)
	lvl := node.Level
	e.headingStack[lvl] = headingText
	for i := lvl + 1; i <= 6; i++ {
		e.headingStack[i] = ""
	}
	parts := make([]string, 0, lvl)
	for i := 1; i <= lvl; i++ {
		if e.headingStack[i] != "" {
			parts = append(parts, e.headingStack[i])
		}
	}
	e.current = &section{
		headingPath: strings.Join(parts, " > "),
		level:       lvl,
	}
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processFencedCode(node *ast.FencedCodeBlock) (ast.WalkStatus, error) {
	e.ensureCurrent()
	lang := ""
	if node.Info != nil {
		infoSeg := node.Info.Segment
		lang = string(infoSeg.Value(e.src))
		if fields := strings.Fields(lang); len(fields) > 0 {
			lang = fields[0]
		}
	}
	var code strings.Builder
	for i := 0; i < node.Lines().Len(); i++ {
		line := node.Lines().At(i)
		code.Write(line.Value(e.src))
	}
	e.current.blocks = append(e.current.blocks, block{
		kind:     blockKindCode,
		codeText: code.String(),
		language: lang,
	})
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processMermaid(node *mermaid.Block) (ast.WalkStatus, error) {
	e.ensureCurrent()
	var code strings.Builder
	for i := 0; i < node.Lines().Len(); i++ {
		line := node.Lines().At(i)
		code.Write(line.Value(e.src))
	}
	e.current.blocks = append(e.current.blocks, block{
		kind:     blockKindCode,
		codeText: code.String(),
		language: "mermaid",
	})
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processParagraph(node *ast.Paragraph) (ast.WalkStatus, error) {
	e.ensureCurrent()
	if isOnlyHashtags(node, e.src) {
		return ast.WalkSkipChildren, nil
	}
	t := extractText(node, e.src, e.registry)
	if t == "" {
		return ast.WalkSkipChildren, nil
	}
	kind := blockKindParagraph
	if containsTaskList(node) {
		kind = blockKindTaskList
	}
	e.current.blocks = append(e.current.blocks, block{
		kind: kind,
		text: t,
	})
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processList(node *ast.List) (ast.WalkStatus, error) {
	e.ensureCurrent()
	t, isTask := extractListText(node, e.src, 0, e.registry)
	if t == "" {
		return ast.WalkSkipChildren, nil
	}
	kind := "list"
	if isTask {
		kind = blockKindTaskList
	}
	e.current.blocks = append(e.current.blocks, block{
		kind: kind,
		text: t,
	})
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processTable(node *extast.Table) (ast.WalkStatus, error) {
	e.ensureCurrent()
	t := extractTableText(node, e.src, e.registry)
	if t == "" {
		return ast.WalkSkipChildren, nil
	}
	e.current.blocks = append(e.current.blocks, block{
		kind: "table",
		text: t,
	})
	return ast.WalkSkipChildren, nil
}

func (e *sectionExtractor) processBlockquote(node *ast.Blockquote) (ast.WalkStatus, error) {
	e.ensureCurrent()
	t := extractBlockquoteText(node, e.src, 0, e.registry)
	if t == "" {
		return ast.WalkSkipChildren, nil
	}
	e.current.blocks = append(e.current.blocks, block{
		kind: "blockquote",
		text: t,
	})
	return ast.WalkSkipChildren, nil
}

type codeBlockInfo struct {
	lang string
	code string
}

type inlineInfo struct {
	plain string
	rich  string
}

func formatChunkText(raw string, codeInfos []codeBlockInfo, inlineRegistry []inlineInfo, rich bool) string {
	out := raw
	// 1. Replace code block placeholders
	for i, info := range codeInfos {
		placeholder := fmt.Sprintf("\x00CODE:%d:%s\x00", i, info.lang)
		if rich {
			fence := "```" + info.lang + "\n" + strings.TrimSpace(info.code) + "\n```"
			out = strings.ReplaceAll(out, placeholder, fence)
		} else {
			clean := "[code]"
			if info.lang != "" {
				clean = "[code:" + info.lang + "]"
			}
			out = strings.ReplaceAll(out, placeholder, clean)
		}
	}
	// 2. Replace inline formatting/link placeholders (backwards to resolve nested structures correctly)
	for i, info := range slices.Backward(inlineRegistry) {
		placeholder := fmt.Sprintf("\x00INLINE:%d\x00", i)
		replacement := info.plain
		if rich {
			replacement = info.rich
		}
		out = strings.ReplaceAll(out, placeholder, replacement)
	}
	return strings.TrimSpace(out)
}

func buildChunks(sections []section, noteSlug string, maxRunes, overlapRunes int, inlineRegistry []inlineInfo) []Chunk {
	chunks := make([]Chunk, 0, len(sections))
	idx := 0

	for _, sec := range sections {
		codeCount := 0
		hasTable := false
		hasTask := false

		var prose strings.Builder
		var codeInfos []codeBlockInfo
		for idx, b := range sec.blocks {
			if idx > 0 {
				if b.kind == "paragraph" && sec.blocks[idx-1].kind == "paragraph" {
					prose.WriteByte(' ')
				} else {
					prose.WriteString("\n\n")
				}
			}
			switch b.kind {
			case blockKindCode:
				codeCount++
				codeIdx := len(codeInfos)
				codeInfos = append(codeInfos, codeBlockInfo{lang: b.language, code: b.codeText})
				_, _ = fmt.Fprintf(&prose, "\x00CODE:%d:%s\x00", codeIdx, b.language)
			case "table":
				hasTable = true
				prose.WriteString(b.text)
			case blockKindTaskList, "list":
				if b.kind == blockKindTaskList {
					hasTask = true
				}
				prose.WriteString(b.text)
			default:
				prose.WriteString(b.text)
			}
		}

		rawText := strings.TrimSpace(prose.String())
		if rawText == "" && codeCount == 0 && !hasTable && !hasTask {
			continue // Skip truly empty sections
		}

		cleanText := formatChunkText(rawText, codeInfos, inlineRegistry, false)
		richText := formatChunkText(rawText, codeInfos, inlineRegistry, true)

		if maxRunes <= 0 || utf8.RuneCountInString(cleanText) <= maxRunes {
			chunks = append(chunks, Chunk{
				NoteSlug:    noteSlug,
				Index:       idx,
				Text:        cleanText,
				RichText:    richText,
				EmbedText:   cleanText,
				HeadingPath: sec.headingPath,
				Level:       sec.level,
				HasTask:     hasTask,
				HasCode:     codeCount > 0,
			})
			idx++
			continue
		}

		rawRunes := []rune(rawText)
		subParts := splitAtBoundary(rawRunes, maxRunes, overlapRunes)
		for _, sub := range subParts {
			chunks = append(chunks, Chunk{
				NoteSlug:    noteSlug,
				Index:       idx,
				Text:        formatChunkText(sub.displayRaw, codeInfos, inlineRegistry, false),
				RichText:    formatChunkText(sub.displayRaw, codeInfos, inlineRegistry, true),
				EmbedText:   formatChunkText(sub.embedRaw, codeInfos, inlineRegistry, false),
				HeadingPath: sec.headingPath,
				Level:       sec.level,
				HasTask:     hasTask,
				HasCode:     strings.Contains(sub.displayRaw, "\x00CODE:"),
			})
			idx++
		}
	}
	return chunks
}

// splitPart is one sub-chunk of a split section.
// embedRaw carries the configured overlap at its start (embedding continuity);
// displayRaw is overlap-free and concatenates losslessly with the other parts.
type splitPart struct {
	embedRaw   string // raw runes for embedding (overlap repeated from previous part)
	displayRaw string // raw runes for display (no overlap)
}

// splitAtBoundary splits a rune slice into parts of at most maxRunes runes each,
// preferring sentence boundaries (./!/?) or newlines as break points.
// overlapRunes from the previous part are repeated at the start of each new
// part's embedRaw only, so display output never duplicates content.
// The overlap rewind never lands inside a \x00...\x00 placeholder token.
func splitAtBoundary(runes []rune, maxRunes, overlapRunes int) []splitPart {
	spans := placeholderSpans(runes)
	parts := make([]splitPart, 0, (len(runes)/maxRunes)+1)
	start := 0
	displayStart := 0
	for start < len(runes) {
		end := start + maxRunes
		if end >= len(runes) {
			parts = append(parts, splitPart{
				embedRaw:   strings.TrimSpace(string(runes[start:])),
				displayRaw: strings.TrimSpace(string(runes[displayStart:])),
			})
			break
		}
		breakAt := end
		for i := end; i > start+maxRunes/2; i-- {
			r := runes[i]
			if (r == '.' || r == '!' || r == '?') && i+1 < len(runes) && runes[i+1] == ' ' {
				breakAt = i + 1
				break
			}
			if r == '\n' {
				breakAt = i
				break
			}
		}
		parts = append(parts, splitPart{
			embedRaw:   strings.TrimSpace(string(runes[start:breakAt])),
			displayRaw: strings.TrimSpace(string(runes[displayStart:breakAt])),
		})

		// Apply overlap: back up by overlapRunes so the next chunk's embed text
		// shares context. Safety floor: nextStart must advance beyond start to
		// prevent infinite loops.
		nextStart := breakAt - overlapRunes
		if nextStart <= start {
			nextStart = breakAt
		}
		// Never rewind into the middle of a placeholder token; snap to its
		// start so formatChunkText can still resolve it.
		nextStart = snapToPlaceholderStart(spans, nextStart)
		start = nextStart
		displayStart = breakAt
	}
	return parts
}

// placeholderSpans returns rune-index spans [start, end) of \x00...\x00
// placeholder tokens in raw, so splits never land mid-token.
func placeholderSpans(raw []rune) [][2]int {
	var spans [][2]int
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\x00' {
			continue
		}
		j := i + 1
		for j < len(raw) && raw[j] != '\x00' {
			j++
		}
		if j < len(raw) {
			spans = append(spans, [2]int{i, j + 1})
			i = j
		}
	}
	return spans
}

// snapToPlaceholderStart returns the start of the placeholder token containing
// pos, or pos unchanged when pos is not inside any placeholder token.
func snapToPlaceholderStart(spans [][2]int, pos int) int {
	for _, sp := range spans {
		if pos > sp[0] && pos < sp[1] {
			return sp[0]
		}
	}
	return pos
}

func handleInlineNode(node ast.Node, entering bool, src []byte, registry *[]inlineInfo, buf *bytes.Buffer, breakChar byte) (ast.WalkStatus, bool) {
	switch nTyped := node.(type) {
	case *ast.Link, *ast.AutoLink, *wikilink.Node, *ast.Emphasis, *ast.CodeSpan, *extast.Strikethrough, *hashtag.Node, *latex.InlineMath:
		if entering {
			idx := len(*registry)
			plain := newChunkRenderer(false).render(nTyped, src)
			rich := newChunkRenderer(true).render(nTyped, src)
			*registry = append(*registry, inlineInfo{plain: plain, rich: rich})
			_, _ = fmt.Fprintf(buf, "\x00INLINE:%d\x00", idx)
			return ast.WalkSkipChildren, true
		}
	case *ast.Text:
		if entering {
			val := nTyped.Segment.Value(src)
			buf.Write(val)
			if nTyped.SoftLineBreak() || nTyped.HardLineBreak() {
				buf.WriteByte(breakChar)
			}
		}
		return ast.WalkContinue, true
	}
	return ast.WalkContinue, false
}

func extractText(n ast.Node, src []byte, registry *[]inlineInfo) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if status, handled := handleInlineNode(node, entering, src, registry, &buf, ' '); handled {
			return status, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

func extractPlainText(n ast.Node, src []byte) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := node.(*ast.Text); ok {
			val := t.Segment.Value(src)
			if node.Parent() != nil {
				if _, isHashtag := node.Parent().(*hashtag.Node); isHashtag {
					// Strip the leading '#' from the hashtag text in prose
					if len(val) > 0 && val[0] == '#' {
						val = val[1:]
					}
				}
			}
			buf.Write(val)
			if t.SoftLineBreak() || t.HardLineBreak() {
				buf.WriteByte(' ')
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

func containsTaskList(n ast.Node) bool {
	found := false
	err := ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := node.(*extast.TaskCheckBox); ok {
				found = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return false
	}
	return found
}

func isOnlyHashtags(n ast.Node, src []byte) bool {
	onlyHashtags := true
	hasHashtags := false

	if err := ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Skip block level containers, walk into their children
		if node.Type() == ast.TypeBlock || node.Type() == ast.TypeDocument {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *hashtag.Node:
			hasHashtags = true
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			txt := string(n.Segment.Value(src))
			if strings.TrimSpace(txt) != "" {
				onlyHashtags = false
				return ast.WalkStop, nil
			}
		default:
			onlyHashtags = false
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	}); err != nil {
		slog.Warn("ast walk encountered error during hashtag check", "err", err)
	}

	return hasHashtags && onlyHashtags
}

func extractFrontmatterTags(fm map[string]any) []string {
	if fm == nil {
		return nil
	}

	var rawTags any
	if val, ok := fm["tags"]; ok {
		rawTags = val
	} else if val, ok := fm["tag"]; ok {
		rawTags = val
	}

	if rawTags == nil {
		return nil
	}

	var tags []string
	switch val := rawTags.(type) {
	case string:
		// e.g. "tag1, tag2" or "tag1 tag2"
		var parts []string
		if strings.Contains(val, ",") {
			parts = strings.Split(val, ",")
		} else {
			parts = strings.Fields(val)
		}
		tags = make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			t = strings.TrimPrefix(t, "#")
			if t != "" {
				tags = append(tags, t)
			}
		}
	case []any:
		tags = make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				t := strings.TrimSpace(s)
				t = strings.TrimPrefix(t, "#")
				if t != "" {
					tags = append(tags, t)
				}
			}
		}
	case []string:
		tags = make([]string, 0, len(val))
		for _, s := range val {
			t := strings.TrimSpace(s)
			t = strings.TrimPrefix(t, "#")
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

func extractListText(l *ast.List, src []byte, indentLevel int, registry *[]inlineInfo) (string, bool) {
	var lines []string
	hasTask := false

	itemIdx := 0
	for child := l.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}

		prefix := strings.Repeat("  ", indentLevel)
		if l.IsOrdered() {
			start := l.Start
			if start == 0 {
				start = 1
			}
			prefix += fmt.Sprintf("%d. ", start+itemIdx)
		} else {
			prefix += "- "
		}

		var itemParts []string
		for itemChild := item.FirstChild(); itemChild != nil; itemChild = itemChild.NextSibling() {
			switch sub := itemChild.(type) {
			case *ast.List:
				nestedText, nestedTask := extractListText(sub, src, indentLevel+1, registry)
				if nestedText != "" {
					itemParts = append(itemParts, nestedText)
				}
				if nestedTask {
					hasTask = true
				}
			default:
				t, task := extractItemText(sub, src, registry)
				if task {
					hasTask = true
				}
				if t != "" {
					if len(itemParts) == 0 {
						itemParts = append(itemParts, prefix+t)
					} else {
						itemParts = append(itemParts, strings.Repeat("  ", indentLevel+1)+t)
					}
				}
			}
		}

		if len(itemParts) > 0 {
			lines = append(lines, strings.Join(itemParts, "\n"))
		} else if len(itemParts) == 0 {
			lines = append(lines, prefix)
		}
		itemIdx++
	}
	return strings.Join(lines, "\n"), hasTask
}

func extractItemText(n ast.Node, src []byte, registry *[]inlineInfo) (string, bool) {
	var buf bytes.Buffer
	hasTask := false

	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if tc, ok := node.(*extast.TaskCheckBox); ok && entering {
			hasTask = true
			if tc.IsChecked {
				buf.WriteString("[x] ")
			} else {
				buf.WriteString("[ ] ")
			}
			return ast.WalkContinue, nil
		}
		if status, handled := handleInlineNode(node, entering, src, registry, &buf, ' '); handled {
			return status, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String()), hasTask
}

func extractTableText(tbl *extast.Table, src []byte, registry *[]inlineInfo) string {
	var rows []string

	for rowNode := tbl.FirstChild(); rowNode != nil; rowNode = rowNode.NextSibling() {
		switch row := rowNode.(type) {
		case *extast.TableHeader:
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if c, ok := cell.(*extast.TableCell); ok {
					cells = append(cells, extractText(c, src, registry))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
				sepCells := make([]string, len(cells))
				for i := range sepCells {
					sepCells[i] = "---"
				}
				rows = append(rows, "| "+strings.Join(sepCells, " | ")+" |")
			}
		case *extast.TableRow:
			var cells []string
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if c, ok := cell.(*extast.TableCell); ok {
					cells = append(cells, extractText(c, src, registry))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
			}
		}
	}
	return strings.Join(rows, "\n")
}

func extractBlockquoteText(bq *ast.Blockquote, src []byte, indent int, registry *[]inlineInfo) string {
	var lines []string
	prefix := strings.Repeat("  ", indent) + "> "

	for child := bq.FirstChild(); child != nil; child = child.NextSibling() {
		var childText string
		switch n := child.(type) {
		case *ast.List:
			childText, _ = extractListText(n, src, 0, registry)
		case *extast.Table:
			childText = extractTableText(n, src, registry)
		case *ast.Blockquote:
			childText = extractBlockquoteText(n, src, indent, registry)
		case *ast.FencedCodeBlock:
			lang := ""
			if n.Info != nil {
				lang = string(n.Info.Segment.Value(src))
				if fields := strings.Fields(lang); len(fields) > 0 {
					lang = fields[0]
				}
			}
			var code strings.Builder
			for i := 0; i < n.Lines().Len(); i++ {
				line := n.Lines().At(i)
				code.Write(line.Value(src))
			}
			childText = "```" + lang + "\n" + strings.TrimSpace(code.String()) + "\n```"
		default:
			childText = extractBlockquoteChildText(child, src, registry)
		}

		if strings.TrimSpace(childText) != "" {
			for line := range strings.SplitSeq(childText, "\n") {
				lines = append(lines, prefix+line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func extractBlockquoteChildText(n ast.Node, src []byte, registry *[]inlineInfo) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if status, handled := handleInlineNode(node, entering, src, registry, &buf, '\n'); handled {
			return status, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}
