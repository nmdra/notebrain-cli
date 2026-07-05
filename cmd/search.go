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

	tea "charm.land/bubbletea/v2"
	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/charmbracelet/x/term"
	"github.com/nmdra/notebrain-cli/v2/cmd/tui"
	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type SearchCmd struct {
	Query       string `arg:"" optional:"" help:"Search query (omit when using --interactive or --tag)"`
	Limit       int    `help:"maximum number of results to return" default:"10"`
	TopKPerNote int    `name:"top-k" help:"maximum number of chunks to return per note" default:"3"`
	Section     string `help:"filter by heading path"`
	Tag         string `help:"filter or search by tag name"`
	HasTasks    bool   `help:"only return chunks that contain task lists"`
	HasCode     bool   `help:"only return chunks that contain code blocks"`
	Interactive bool   `help:"launch live interactive search TUI"`
}

func (c *SearchCmd) Run(globals *Globals) error {

	if !c.Interactive && c.Query == "" && c.Tag == "" {
		return fmt.Errorf("query or --tag is required (or use --interactive for live search)")
	}

	chromaPath := globals.ChromaPath
	ctx := globals.Ctx
	st, err := store.Open(ctx, chromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	st.SkipAttachments = globals.SkipAttachments

	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	// ── Interactive live-search TUI ────────────────────────────────────────
	if c.Interactive {
		if !term.IsTerminal(os.Stdout.Fd()) || os.Getenv("TERM") == "dumb" {
			return fmt.Errorf("interactive mode requires a TTY terminal; use --format json or remove --interactive")
		}
		// Build the chroma where-filter once (same logic as static path).
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
		if c.Tag != "" {
			filters = append(filters, store.TagWhereClause(c.Tag))
		}
		var whereFilter chroma.WhereFilter
		if len(filters) == 1 {
			whereFilter = filters[0]
		} else if len(filters) > 1 {
			whereFilter = chroma.And(filters...)
		}

		limit := c.Limit
		topK := c.TopKPerNote
		searchFn := func(ctx context.Context, query string) ([]store.Result, error) {
			var results []store.Result
			var err error
			tui.SuppressOutputs(func() {
				var qVec []float32
				qVec, err = emb.Embed(ctx, query)
				if err != nil {
					err = fmt.Errorf("embed query: %w", err)
					return
				}
				results, err = st.SemanticSearch(ctx, qVec, limit, topK, whereFilter, false)
				if err == nil {
					st.PopulateContext(ctx, results, globals.ContextWindow)
				}
			})
			return results, err
		}

		model := tui.NewLiveSearch(searchFn, globals.VaultPath, limit, c.Query, globals.UseEditor)
		p := tea.NewProgram(model)
		var runErr error
		// Suppress ChromaDB/hnswlib integrity-check noise from polluting the TUI.
		tui.SuppressStderr(func() {
			_, runErr = p.Run()
		})
		return runErr
	}

	// ── Static non-interactive search ──────────────────────────────────────
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
	if c.Tag != "" && c.Query != "" {
		filters = append(filters, store.TagWhereClause(c.Tag))
	}
	var whereFilter chroma.WhereFilter
	if len(filters) == 1 {
		whereFilter = filters[0]
	} else if len(filters) > 1 {
		whereFilter = chroma.And(filters...)
	}

	if c.Query == "" {
		results, err := st.TagSearch(ctx, c.Tag, c.Limit, whereFilter, globals.IncludeText)
		if err != nil {
			return err
		}
		st.PopulateContext(ctx, results, globals.ContextWindow)
		printResultsFormatted("search", fmt.Sprintf("Tag Search: %q", c.Tag), results, globals)
		return nil
	}

	qVec, err := emb.Embed(ctx, c.Query)
	if err != nil {
		return err
	}

	results, err := st.SemanticSearch(ctx, qVec, c.Limit, c.TopKPerNote, whereFilter, globals.IncludeText)
	if err != nil {
		return err
	}
	st.PopulateContext(ctx, results, globals.ContextWindow)

	printResultsFormatted("search", fmt.Sprintf("Semantic Search: %q", c.Query), results, globals)
	return nil
}
