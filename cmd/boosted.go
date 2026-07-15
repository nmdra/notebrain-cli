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
)

type BoostedCmd struct {
	Query string  `arg:"" help:"search query text"`
	Limit int     `help:"maximum number of results" default:"10"`
	Seed  string  `help:"seed note (slug, title, or path) whose graph neighbors get score boost" required:"true"`
	Boost float64 `help:"score multiplier for graph-connected results (e.g. 1.5 = 50% boost)" default:"1.5"`
}

func (c *BoostedCmd) Run(globals *Globals) error {
	query := c.Query
	seed := c.Seed
	boost := c.Boost
	limit := c.Limit

	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	seedSlug, err := st.ResolveNoteSlug(ctx, seed)
	if err != nil {
		return err
	}

	emb, err := embedder.NewLocalEmbedder(embedder.WithQuiet(globals.Format != "text" || globals.JSONPath != ""))
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	qVec, err := emb.Embed(ctx, query)
	if err != nil {
		return err
	}

	results, err := st.GraphBoostedSearch(ctx, qVec, seedSlug, boost, limit, globals.IncludeText)
	if err != nil {
		return err
	}
	st.PopulateContext(ctx, results, globals.ContextWindow)

	printResultsFormatted("boosted", fmt.Sprintf("Graph-Boosted Search Results for: %q (seed: %s, boost: %.2f)", query, seedSlug, boost), query, results, globals)
	return nil
}
