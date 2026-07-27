package pdf2md

import (
	"strings"
)

// Block represents a structural element of the document that can render itself as Markdown.
type Block interface {
	Markdown() string
}

// ParagraphBlock represents a standard text paragraph.
type ParagraphBlock struct {
	Text string
}

func (p ParagraphBlock) Markdown() string {
	return p.Text
}

// HeadingBlock represents a section heading.
type HeadingBlock struct {
	Level int // 1 to 6
	Text  string
}

func (h HeadingBlock) Markdown() string {
	prefix := strings.Repeat("#", h.Level)
	return prefix + " " + h.Text
}

// ListBlock represents a list (bulleted or numbered).
type ListBlock struct {
	Items []string
}

func (l ListBlock) Markdown() string {
	return strings.Join(l.Items, "\n")
}

// CodeBlock represents preformatted/monospace text.
type CodeBlock struct {
	Code string
}

func (c CodeBlock) Markdown() string {
	return "```\n" + c.Code + "\n```"
}

// TableBlock represents a columnar table.
type TableBlock struct {
	Rows [][]string
}

func (t TableBlock) Markdown() string {
	if len(t.Rows) == 0 {
		return ""
	}
	var buf strings.Builder
	for i, row := range t.Rows {
		buf.WriteString("| " + strings.Join(row, " | ") + " |\n")
		// Add separator after first row
		if i == 0 {
			buf.WriteString("|")
			for range row {
				buf.WriteString(" --- |")
			}
			buf.WriteString("\n")
		}
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
