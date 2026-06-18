package parser

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	meta "github.com/yuin/goldmark-meta"
	"go.abhg.dev/goldmark/hashtag"
	"go.abhg.dev/goldmark/wikilink"
)

// ASTChunk is one section of a note, bounded by heading structure.
type ASTChunk struct {
	NoteSlug    string
	Index       int
	Text        string // clean prose text for embedding
	HeadingPath string // e.g. "Architecture > Data Flow > Ingest"
	Level       int    // depth of the deepest heading in this chunk (1-6)
	CodeBlocks  int    // number of fenced code blocks in this chunk
	HasTable    bool
	HasTask     bool
	WordCount   int
}

// ASTResult is the output from parsing the full document, containing the chunks and metadata.
type ASTResult struct {
	Chunks      []ASTChunk
	Tags        []string
	Links       []string
	Frontmatter map[string]interface{}
}

// mdParser is the shared goldmark instance configured with GFM, hashtags, wikilinks, and metadata.
var mdParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		&hashtag.Extender{
			Variant: hashtag.ObsidianVariant,
		},
		&wikilink.Extender{},
		meta.New(meta.WithStoresInDocument()),
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

// ParseAST parses body text into ASTChunks, extracting wikilinks, tags, and frontmatter.
func ParseAST(body, noteSlug string, maxChunkRunes int) ASTResult {
	src := []byte(body)
	reader := text.NewReader(src)
	doc := mdParser.Parser().Parse(reader)

	// Extract frontmatter metadata stored by goldmark-meta
	var frontmatter map[string]interface{}
	if md := doc.OwnerDocument().Meta(); md != nil {
		frontmatter = md
	}

	// Walk AST to collect Tags and Links
	tagsSet := make(map[string]struct{})
	linksSet := make(map[string]struct{})

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *hashtag.Node:
			tagsSet[string(node.Tag)] = struct{}{}
		case *wikilink.Node:
			linksSet[string(node.Target)] = struct{}{}
		}

		return ast.WalkContinue, nil
	})

	var tags []string
	for t := range tagsSet {
		tags = append(tags, t)
	}
	var links []string
	for l := range linksSet {
		links = append(links, l)
	}

	// Extract chunks using section logic
	sections := extractSections(doc, src)
	chunks := buildChunks(sections, noteSlug, maxChunkRunes)

	return ASTResult{
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

func extractSections(doc ast.Node, src []byte) []section {
	headingStack := make([]string, 7) // index = heading level 1-6
	var sections []section
	var current *section

	ensureCurrent := func() {
		if current == nil {
			current = &section{headingPath: "", level: 0}
		}
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			if current != nil && len(current.blocks) > 0 {
				sections = append(sections, *current)
			}
			headingText := extractText(node, src)
			lvl := node.Level
			headingStack[lvl] = headingText
			for i := lvl + 1; i <= 6; i++ {
				headingStack[i] = ""
			}
			var parts []string
			for i := 1; i <= lvl; i++ {
				if headingStack[i] != "" {
					parts = append(parts, headingStack[i])
				}
			}
			current = &section{
				headingPath: strings.Join(parts, " > "),
				level:       lvl,
			}
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			ensureCurrent()
			lang := ""
			if node.Info != nil {
				lang = string(node.Info.Segment.Value(src))
				lang = strings.Fields(lang)[0] // strip trailing options
			}
			var code strings.Builder
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				code.Write(line.Value(src))
			}
			current.blocks = append(current.blocks, block{
				kind:     "code",
				codeText: code.String(),
				language: lang,
			})
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			ensureCurrent()
			t := extractText(node, src)
			if t == "" {
				return ast.WalkSkipChildren, nil
			}
			kind := "paragraph"
			if containsTaskList(node) {
				kind = "task_list"
			}
			current.blocks = append(current.blocks, block{
				kind: kind,
				text: t,
			})
			return ast.WalkSkipChildren, nil

		case *extast.Table:
			ensureCurrent()
			current.blocks = append(current.blocks, block{
				kind: "table",
				text: extractText(node, src),
			})
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			ensureCurrent()
			current.blocks = append(current.blocks, block{
				kind: "blockquote",
				text: extractText(node, src),
			})
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	if current != nil && len(current.blocks) > 0 {
		sections = append(sections, *current)
	}

	if len(sections) == 0 {
		return []section{{headingPath: "", level: 0}}
	}
	return sections
}

func buildChunks(sections []section, noteSlug string, maxRunes int) []ASTChunk {
	var chunks []ASTChunk
	idx := 0

	for _, sec := range sections {
		codeCount := 0
		hasTable := false
		hasTask := false

		var prose strings.Builder
		for _, b := range sec.blocks {
			switch b.kind {
			case "code":
				codeCount++
				if b.language != "" {
					prose.WriteString("[code:" + b.language + "] ")
				} else {
					prose.WriteString("[code] ")
				}
			case "table":
				hasTable = true
				prose.WriteString(b.text + " ")
			case "task_list":
				hasTask = true
				prose.WriteString(b.text + " ")
			default:
				prose.WriteString(b.text + " ")
			}
		}

		text := strings.TrimSpace(prose.String())
		if text == "" && codeCount == 0 && !hasTable && !hasTask {
			continue // Skip truly empty sections
		}

		// If the section only has code blocks / tables, its 'text' might just be "[code:go] ". Let's keep it.
		runes := []rune(text)
		if maxRunes <= 0 || len(runes) <= maxRunes {
			chunks = append(chunks, ASTChunk{
				NoteSlug:    noteSlug,
				Index:       idx,
				Text:        text,
				HeadingPath: sec.headingPath,
				Level:       sec.level,
				CodeBlocks:  codeCount,
				HasTable:    hasTable,
				HasTask:     hasTask,
				WordCount:   len(strings.Fields(text)),
			})
			idx++
			continue
		}

		subTexts := splitAtBoundary(runes, maxRunes)
		for _, sub := range subTexts {
			chunks = append(chunks, ASTChunk{
				NoteSlug:    noteSlug,
				Index:       idx,
				Text:        sub,
				HeadingPath: sec.headingPath,
				Level:       sec.level,
				CodeBlocks:  codeCount,
				HasTable:    hasTable,
				HasTask:     hasTask,
				WordCount:   len(strings.Fields(sub)),
			})
			idx++
		}
	}
	return chunks
}

func splitAtBoundary(runes []rune, maxRunes int) []string {
	var parts []string
	start := 0
	for start < len(runes) {
		end := start + maxRunes
		if end >= len(runes) {
			parts = append(parts, strings.TrimSpace(string(runes[start:])))
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
		parts = append(parts, strings.TrimSpace(string(runes[start:breakAt])))
		start = breakAt
	}
	return parts
}

func extractText(n ast.Node, src []byte) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := node.(*ast.Text); ok {
			buf.Write(t.Segment.Value(src))
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
