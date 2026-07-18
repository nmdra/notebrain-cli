package parser

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	latex "github.com/soypat/goldmark-latex"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"go.abhg.dev/goldmark/hashtag"
	"go.abhg.dev/goldmark/mermaid"
	"go.abhg.dev/goldmark/wikilink"
)

type chunkRenderer struct {
	rich bool
}

func newChunkRenderer(rich bool) *chunkRenderer {
	return &chunkRenderer{rich: rich}
}

func (r *chunkRenderer) render(n ast.Node, source []byte) string {
	var buf bytes.Buffer
	r.renderNode(&buf, n, source)
	return buf.String()
}

func (r *chunkRenderer) renderNode(w io.Writer, n ast.Node, source []byte) {
	if n == nil {
		return
	}
	if r.renderBlock(w, n, source) {
		return
	}
	if r.renderInline(w, n, source) {
		return
	}
	// Fallback for custom/unhandled nodes
	r.renderChildren(w, n, source)
}

func (r *chunkRenderer) renderBlock(w io.Writer, n ast.Node, source []byte) bool {
	switch node := n.(type) {
	case *ast.Document, *ast.Paragraph, *ast.TextBlock:
		r.renderChildren(w, node, source)
	case *ast.Heading:
		if r.rich {
			_, _ = w.Write([]byte(strings.Repeat("#", node.Level) + " "))
		}
		r.renderChildren(w, node, source)
		_, _ = w.Write([]byte("\n"))
	case *ast.FencedCodeBlock:
		lang := ""
		if node.Info != nil {
			infoSeg := node.Info.Segment
			lang = string(infoSeg.Value(source))
			if fields := strings.Fields(lang); len(fields) > 0 {
				lang = fields[0]
			}
		}
		if r.rich {
			_, _ = w.Write([]byte("```" + lang + "\n"))
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				_, _ = w.Write(line.Value(source))
			}
			_, _ = w.Write([]byte("```\n"))
		} else {
			if lang != "" {
				_, _ = w.Write([]byte("[code:" + lang + "]"))
			} else {
				_, _ = w.Write([]byte("[code]"))
			}
		}
	case *mermaid.Block:
		if r.rich {
			_, _ = w.Write([]byte("```mermaid\n"))
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				_, _ = w.Write(line.Value(source))
			}
			_, _ = w.Write([]byte("```\n"))
		} else {
			_, _ = w.Write([]byte("[diagram:mermaid]"))
		}
	case *ast.Blockquote:
		var buf bytes.Buffer
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			r.renderNode(&buf, child, source)
		}
		content := strings.TrimSpace(buf.String())
		if r.rich {
			for line := range strings.SplitSeq(content, "\n") {
				_, _ = w.Write([]byte("> " + line + "\n"))
			}
		} else {
			_, _ = w.Write([]byte(content))
		}
	case *ast.List:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			r.renderNode(w, child, source)
		}
	case *ast.ListItem:
		parent := node.Parent().(*ast.List)
		if r.rich {
			level := 0
			for p := node.Parent(); p != nil; p = p.Parent() {
				if _, ok := p.(*ast.List); ok {
					level++
				}
			}
			indent := strings.Repeat("  ", level-1)

			if parent.IsOrdered() {
				index := 1
				for sibling := node.PreviousSibling(); sibling != nil; sibling = sibling.PreviousSibling() {
					index++
				}
				_, _ = fmt.Fprintf(w, "%s%d. ", indent, index)
			} else {
				_, _ = w.Write([]byte(indent + "- "))
			}
		}
		var buf bytes.Buffer
		r.renderChildren(&buf, node, source)
		content := strings.TrimSpace(buf.String())
		_, _ = w.Write([]byte(content + "\n"))
	case *extast.Table, *extast.TableCell:
		r.renderChildren(w, node, source)
		if _, ok := node.(*extast.TableCell); ok {
			_, _ = w.Write([]byte(" | "))
		}
	case *extast.TableHeader, *extast.TableRow:
		r.renderChildren(w, node, source)
		_, _ = w.Write([]byte("\n"))
	default:
		return false
	}
	return true
}

func (r *chunkRenderer) renderInline(w io.Writer, n ast.Node, source []byte) bool {
	switch node := n.(type) {
	case *ast.Text:
		_, _ = w.Write(node.Segment.Value(source))
		if node.SoftLineBreak() || node.HardLineBreak() {
			_, _ = w.Write([]byte("\n"))
		}
	case *ast.String:
		_, _ = w.Write(node.Value)
	case *ast.CodeSpan:
		if r.rich {
			_, _ = w.Write([]byte("`"))
		}
		r.renderChildren(w, node, source)
		if r.rich {
			_, _ = w.Write([]byte("`"))
		}
	case *latex.InlineMath:
		_, _ = w.Write([]byte("$"))
		_, _ = w.Write(node.Content)
		_, _ = w.Write([]byte("$"))
	case *ast.Emphasis:
		marker := "*"
		if node.Level == 2 {
			marker = "**"
		}
		if r.rich {
			_, _ = w.Write([]byte(marker))
		}
		r.renderChildren(w, node, source)
		if r.rich {
			_, _ = w.Write([]byte(marker))
		}
	case *extast.Strikethrough:
		if r.rich {
			_, _ = w.Write([]byte("~~"))
		}
		r.renderChildren(w, node, source)
		if r.rich {
			_, _ = w.Write([]byte("~~"))
		}
	case *extast.TaskCheckBox:
		if node.IsChecked {
			_, _ = w.Write([]byte("[x] "))
		} else {
			_, _ = w.Write([]byte("[ ] "))
		}
	case *hashtag.Node:
		if r.rich {
			_, _ = w.Write([]byte("#" + string(node.Tag)))
		} else {
			_, _ = w.Write([]byte(string(node.Tag)))
		}
	case *wikilink.Node:
		var labelBuf bytes.Buffer
		r.renderChildren(&labelBuf, node, source)
		label := labelBuf.String()

		target := string(node.Target)
		if len(node.Fragment) > 0 {
			target += "#" + string(node.Fragment)
		}

		if r.rich {
			_, _ = w.Write([]byte("[[" + target))
			if label != "" && label != target {
				_, _ = w.Write([]byte("|" + label))
			}
			_, _ = w.Write([]byte("]]"))
		} else {
			if label != "" {
				_, _ = w.Write([]byte(label))
			} else {
				_, _ = w.Write([]byte(target))
			}
		}
	case *ast.Link:
		if r.rich {
			_, _ = w.Write([]byte("["))
			r.renderChildren(w, node, source)
			_, _ = w.Write([]byte("]("))
			_, _ = w.Write(node.Destination)
			_, _ = w.Write([]byte(")"))
		} else {
			r.renderChildren(w, node, source)
		}
	case *ast.AutoLink:
		_, _ = w.Write(node.Label(source))
	default:
		return false
	}
	return true
}

func (r *chunkRenderer) renderChildren(w io.Writer, n ast.Node, source []byte) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		r.renderNode(w, child, source)
	}
}
