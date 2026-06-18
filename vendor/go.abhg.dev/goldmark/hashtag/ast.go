package hashtag

import "github.com/yuin/goldmark/ast"

// Kind is the kind of hashtag AST nodes.
var Kind = ast.NewNodeKind("Hashtag")

// Node is a hashtag node in a Goldmark Markdown document.
type Node struct {
	ast.BaseInline

	// Tag is the portion of the hashtag following the '#'.
	Tag []byte
}

// Kind reports the kind of hashtag nodes.
func (*Node) Kind() ast.NodeKind { return Kind }

// Dump dumps the contents of Node to stdout for debugging.
func (n *Node) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{
		"Tag": string(n.Tag),
	}, nil)
}
