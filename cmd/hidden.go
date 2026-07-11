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

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type HiddenCmd struct {
	Note          string `arg:"" help:"note slug, title, or file path (auto-resolved)"`
	Limit         int    `help:"maximum number of hidden connections to return" default:"10"`
	Deep          bool   `help:"analyze each chunk individually for granular section-level matches"`
	TopK          int    `name:"top-k" help:"chunks to evaluate per candidate note in --deep mode" default:"3"`
	IncludeLinked bool   `name:"include-linked" help:"include notes even if they are already linked directly or indirectly"`
}

func (c *HiddenCmd) Run(globals *Globals) error {
	targetNote := c.Note
	limit := c.Limit

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

	opts := []store.HiddenOption{store.WithIncludeLinked(c.IncludeLinked)}

	if c.Deep {
		results, seedChunks, err := st.HiddenConnectionsDeep(ctx, targetSlug, limit, c.TopK, globals.IncludeText, opts...)
		if err != nil {
			return err
		}
		st.PopulateContext(ctx, results, globals.ContextWindow)
		globals.Queries = seedChunks
		cmdName := "hidden --deep"
		title := fmt.Sprintf("Deep chunk-by-chunk hidden connections for: %q (slug: %s) [%d target chunks analyzed]", targetNote, targetSlug, len(seedChunks))
		if c.IncludeLinked {
			cmdName = "hidden --deep --include-linked"
			title = fmt.Sprintf("Deep chunk-by-chunk related connections (including linked) for: %q (slug: %s) [%d target chunks analyzed]", targetNote, targetSlug, len(seedChunks))
		}
		printResultsFormatted(cmdName, title, results, globals)
		return nil
	}

	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	qVec, err := emb.Embed(ctx, targetNote) // Embed the note title as the search context
	if err != nil {
		return err
	}

	results, err := st.HiddenConnections(ctx, qVec, targetSlug, limit, globals.IncludeText, opts...)
	if err != nil {
		return err
	}
	st.PopulateContext(ctx, results, globals.ContextWindow)

	cmdName := "hidden"
	title := fmt.Sprintf("Hidden connections for: %q (slug: %s)", targetNote, targetSlug)
	if c.IncludeLinked {
		cmdName = "hidden --include-linked"
		title = fmt.Sprintf("Related connections (including linked) for: %q (slug: %s)", targetNote, targetSlug)
	}
	printResultsFormatted(cmdName, title, results, globals)
	return nil
}
