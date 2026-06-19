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

	tea "charm.land/bubbletea/v2"
	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/nmdra/notebrain-cli/cmd/tui"
	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/store"
)

type SearchCmd struct {
	Query       string `arg:"" help:"Search query"`
	Limit       int    `help:"maximum number of results to return" default:"10"`
	Section     string `help:"filter by heading path"`
	HasTasks    bool   `help:"only return chunks that contain task lists"`
	HasCode     bool   `help:"only return chunks that contain code blocks"`
	Interactive bool   `help:"open interactive browser"`
}

func (c *SearchCmd) Run(globals *Globals) error {

	query := c.Query
	limit := c.Limit

	chromaPath := globals.ChromaPath
	ctx := globals.Ctx
	st, err := store.Open(ctx, chromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	qVec, err := emb.Embed(ctx, query)
	if err != nil {
		return err
	}

	// Build filters based on flags
	var filters []chroma.WhereClause

	if section := c.Section; section != "" {
		filters = append(filters, chroma.EqString("heading_path", section))
	}
	if hasTasks := c.HasTasks; hasTasks {
		filters = append(filters, chroma.EqBool("has_task", true))
	}
	if hasCode := c.HasCode; hasCode {
		filters = append(filters, chroma.EqBool("has_code", true))
	}

	var whereFilter chroma.WhereFilter
	if len(filters) == 1 {
		whereFilter = filters[0]
	} else if len(filters) > 1 {
		whereFilter = chroma.And(filters...)
	}

	results, err := st.SemanticSearch(ctx, qVec, limit, whereFilter)
	if err != nil {
		return err
	}

	if c.Interactive {
		header := fmt.Sprintf("Semantic Search: %q", query)
		p := tea.NewProgram(tui.NewResultBrowser(header, globals.Vault, results))
		if _, err := p.Run(); err != nil {
			return err
		}
		return nil
	}

	printResults(fmt.Sprintf("Semantic Search: %q", query), results, globals.VaultName, hyperlinkSupported(globals))
	return nil
}
