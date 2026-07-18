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
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/charmbracelet/x/term"
	"github.com/nmdra/notebrain-cli/v2/cmd/tui"
	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type SearchCmd struct {
	Queries     []string `arg:"" optional:"" name:"query" help:"search query (multiple args for multi-hit boosting)"`
	Split       bool     `help:"split query by delimiters for independent sub-searches with multi-hit boosting"`
	SplitBy     string   `name:"split-by" help:"delimiter characters for --split" default:",|;"`
	Limit       int      `help:"maximum number of results" default:"10"`
	TopKPerNote int      `name:"top-k" help:"maximum chunks to retain per note (prevents one note dominating)" default:"3"`
	Section     string   `help:"filter results to chunks under this heading path (e.g. 'Architecture > Components')"`
	Tag         string   `help:"filter results to notes with this tag"`
	HasTasks    bool     `help:"only return chunks containing task lists (checkboxes)"`
	HasCode     bool     `help:"only return chunks containing fenced code blocks"`
	Interactive bool     `help:"launch the live interactive search TUI"`
}

func resolveQueries(queries []string, split bool, splitBy string) []string {
	if len(queries) == 0 {
		return nil
	}
	var rawList []string
	if split {
		f := func(c rune) bool {
			return strings.ContainsRune(splitBy, c)
		}
		for _, q := range queries {
			rawList = append(rawList, strings.FieldsFunc(q, f)...)
		}
	} else {
		rawList = queries
	}

	seen := make(map[string]struct{})
	out := []string{}
	for _, q := range rawList {
		cleaned := strings.TrimSpace(q)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; !ok {
			seen[cleaned] = struct{}{}
			out = append(out, cleaned)
		}
	}
	return out
}

func (c *SearchCmd) Run(globals *Globals) error {
	if c.TopKPerNote >= 4 {
		fmt.Fprintf(os.Stderr, "warning: --top-k >= 4 may exceed upstream ChromaDB embedded 1 MiB FFI limit on large notes\n")
	}
	resolved := resolveQueries(c.Queries, c.Split, c.SplitBy)
	if !c.Interactive && len(resolved) == 0 && c.Tag == "" {
		return fmt.Errorf("query or --tag is required (or use --interactive for live search)")
	}
	queryStr := strings.Join(resolved, " | ")
	if len(resolved) > 1 {
		globals.Queries = resolved
	}

	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	emb, err := embedder.NewLocalEmbedder(embedder.WithQuiet(globals.Format != "text" || globals.JSONPath != ""))
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	if c.Interactive {
		return c.runInteractive(ctx, globals, st, emb, resolved, queryStr)
	}
	return c.runStatic(ctx, globals, st, emb, resolved)
}

func (c *SearchCmd) runInteractive(ctx context.Context, globals *Globals, st *store.Store, emb *embedder.LocalEmbedder, resolved []string, queryStr string) error {
	if !term.IsTerminal(os.Stdout.Fd()) || os.Getenv("TERM") == "dumb" {
		return fmt.Errorf("interactive mode requires a TTY terminal; use --format json or remove --interactive")
	}
	whereFilter := c.buildWhereFilter(true)
	limit := c.Limit
	topK := c.TopKPerNote

	searchFn := func(ctx context.Context, query string) ([]store.Result, error) {
		var results []store.Result
		var err error
		tui.SuppressOutputs(func() {
			resQueries := resolveQueries([]string{query}, c.Split, c.SplitBy)
			if len(resQueries) > 1 {
				var qVecs [][]float32
				qVecs, err = emb.EmbedBatch(ctx, resQueries)
				if err == nil {
					results, err = st.MultiSemanticSearch(ctx, qVecs, resQueries, limit, topK, whereFilter, false)
				}
			} else {
				var qVec []float32
				qVec, err = emb.Embed(ctx, query)
				if err == nil {
					results, err = st.SemanticSearch(ctx, qVec, limit, topK, whereFilter, false)
				}
			}
			if err == nil {
				st.PopulateContext(ctx, results, globals.ContextWindow)
			}
		})
		if err != nil {
			return nil, err
		}
		return results, nil
	}

	model := tui.NewLiveSearch(searchFn, globals.VaultPath, limit, queryStr, globals.UseEditor)
	p := tea.NewProgram(model)
	var runErr error
	tui.SuppressStderr(func() {
		_, runErr = p.Run()
	})
	return runErr
}

func (c *SearchCmd) runStatic(ctx context.Context, globals *Globals, st *store.Store, emb *embedder.LocalEmbedder, resolved []string) error {
	whereFilter := c.buildWhereFilter(len(resolved) > 0)

	if len(resolved) == 0 {
		results, err := st.TagSearch(ctx, c.Tag, c.Limit, whereFilter, globals.IncludeText)
		if err != nil {
			return err
		}
		st.PopulateContext(ctx, results, globals.ContextWindow)
		printResultsFormatted("search", fmt.Sprintf("Tag Search: %q", c.Tag), c.Tag, results, globals)
		return nil
	}

	if len(resolved) > 1 {
		qVecs, err := emb.EmbedBatch(ctx, resolved)
		if err != nil {
			return err
		}
		results, err := st.MultiSemanticSearch(ctx, qVecs, resolved, c.Limit, c.TopKPerNote, whereFilter, globals.IncludeText)
		if err != nil {
			return err
		}
		st.PopulateContext(ctx, results, globals.ContextWindow)

		var quoted []string
		for _, q := range resolved {
			quoted = append(quoted, fmt.Sprintf("%q", q))
		}
		header := fmt.Sprintf("Semantic Search (%d split queries): %s", len(resolved), strings.Join(quoted, " | "))
		printResultsFormatted("search", header, strings.Join(resolved, " | "), results, globals)
		return nil
	}

	qVec, err := emb.Embed(ctx, resolved[0])
	if err != nil {
		return err
	}

	results, err := st.SemanticSearch(ctx, qVec, c.Limit, c.TopKPerNote, whereFilter, globals.IncludeText)
	if err != nil {
		return err
	}
	st.PopulateContext(ctx, results, globals.ContextWindow)

	printResultsFormatted("search", fmt.Sprintf("Semantic Search: %q", resolved[0]), resolved[0], results, globals)
	return nil
}

func (c *SearchCmd) buildWhereFilter(resolveTags bool) chroma.WhereFilter {
	filters := make([]chroma.WhereClause, 0, 4)
	if c.Section != "" {
		filters = append(filters, chroma.EqString("heading_path", c.Section))
	}
	if c.HasTasks {
		filters = append(filters, chroma.EqBool("has_task", true))
	}
	if c.HasCode {
		filters = append(filters, chroma.EqBool("has_code", true))
	}
	if c.Tag != "" && resolveTags {
		filters = append(filters, store.TagWhereClause(c.Tag))
	}
	if len(filters) == 1 {
		return filters[0]
	}
	if len(filters) > 1 {
		return chroma.And(filters...)
	}
	return nil
}
