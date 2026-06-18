package hashtag

import (
	"fmt"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Resolver resolves hashtags to pages they should link to.
type Resolver interface {
	// ResolveHashtag reports the link that the provided hashtag Node
	// should point to, or an empty destination for hashtags that should
	// not link to anything.
	ResolveHashtag(*Node) (destination []byte, err error)
}

// Renderer renders hashtag nodes into HTML, optionally linking them to
// specific pages.
//
//	#foo
//
// Renders as the following by default.
//
//	<span class="hashtag">#foo</span>
//
// Supply a Resolver that returns a non-empty destination to render it like
// the following.
//
//	<span class="hashtag"><a href="...">#foo</a></span>
type Renderer struct {
	// Resolver specifies how where hashtag links should point, if at all.
	//
	// When a Resolver returns an empty destination for a hashtag, the
	// Renderer will render the hashtag as plain text rather than a link.
	//
	// Defaults to empty destinations for all hashtags.
	Resolver Resolver

	Attributes []Attribute

	hasDest sync.Map // *Node => struct{}
}

// Attribute defines an attribute to be added to an HTML tag.
//
//	Attribute{ Attr: "class", Value: "tag"}
//
// Will result in <a class="tag" ...>
type Attribute struct {
	Name  string
	Value string
}

// RegisterFuncs registers rendering functions from this renderer onto the
// provided registerer.
func (r *Renderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(Kind, r.Render)
}

// Render renders a hashtag node as HTML.
func (r *Renderer) Render(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*Node)
	if !ok {
		return ast.WalkStop, fmt.Errorf("unexpected node %T, expected *Node", node)
	}

	if entering {
		if err := r.enter(w, n); err != nil {
			return ast.WalkStop, err
		}
	} else {
		r.exit(w, n)
	}

	return ast.WalkContinue, nil
}

func (r *Renderer) enter(w util.BufWriter, n *Node) error {
	_, _ = w.WriteString(`<span class="hashtag">`)

	var dest []byte
	if res := r.Resolver; res != nil {
		var err error
		dest, err = res.ResolveHashtag(n)
		if err != nil {
			return fmt.Errorf("resolve hashtag %q: %w", n.Tag, err)
		}
	}

	if len(dest) == 0 {
		return nil
	}

	r.hasDest.Store(n, struct{}{})
	_, _ = w.WriteString(`<a `)
	for _, attr := range r.Attributes {
		_, _ = w.WriteString(attr.Name)
		_, _ = w.WriteString(`="`)
		_, _ = w.Write(util.EscapeHTML([]byte(attr.Value)))
		_, _ = w.WriteString(`" `)
	}
	_, _ = w.WriteString(`href="`)
	_, _ = w.Write(util.URLEscape(dest, true /* resolve references */))
	_, _ = w.WriteString(`">`)
	return nil
}

func (r *Renderer) exit(w util.BufWriter, n *Node) {
	if _, ok := r.hasDest.LoadAndDelete(n); ok {
		_, _ = w.WriteString("</a>")
	}
	_, _ = w.WriteString("</span>")
}
