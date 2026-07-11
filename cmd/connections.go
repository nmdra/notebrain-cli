/*
Copyright © 2026 nmdra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"fmt"
)

type ConnectionsCmd struct {
	Note string `arg:"" help:"note slug, title, or file path (auto-resolved)"`
	Hops int    `help:"graph traversal depth (keep 1-2 to avoid exponential blowup)" default:"2"`
}

func (c *ConnectionsCmd) Run(globals *Globals) error {
	targetNote := c.Note
	hops := c.Hops

	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	targetSlug, err := st.ResolveNoteSlug(ctx, targetNote)
	if err != nil {
		return err
	}

	nodes, err := st.Connections(ctx, targetSlug, hops)
	if err != nil {
		return err
	}

	printResultsFormatted(fmt.Sprintf("connections --hops %d", hops), fmt.Sprintf("Graph Connections from: %q (slug: %s) [Hops: %d]", targetNote, targetSlug, hops), nodes, globals)
	return nil
}
