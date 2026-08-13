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
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type SearchCmd struct {
	Queries      []string `group:"search" arg:"" optional:"" name:"query" help:"search query (multiple args for multi-hit boosting)"`
	Limit        int      `group:"search" help:"maximum number of results" default:"10"`
	TopKPerNote  int      `group:"search" name:"top-k" help:"maximum chunks to retain per note (prevents one note dominating)" default:"3"`
	Section      string   `group:"search" help:"filter results to chunks under this heading path (e.g. 'Architecture > Components')"`
	Tag          string   `group:"search" help:"filter results to notes with this tag"`
	HasTasks     bool     `group:"search" help:"only return chunks containing task lists (checkboxes)"`
	HasCode      bool     `group:"search" help:"only return chunks containing fenced code blocks"`
	WithPDF      bool     `group:"search" help:"include PDF results in search"`
	ExcludeNotes []string `group:"search" name:"exclude-notes" help:"exclude notes from results (slug, title, or path; repeatable or comma-separated)" completion-predictor:"note-slug"`
	ExcludeNote  []string `group:"search" name:"exclude-note" hidden:"" help:"deprecated: use --exclude-notes"`
	ChunkDisplayFlags
}

func resolveQueries(queries []string) []string {
	if len(queries) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := []string{}
	for _, q := range queries {
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

// readQueryFromStdin returns the query piped through stdin when stdin is not
// a terminal. It reports false otherwise so the caller falls back to the
// normal usage error.
func readQueryFromStdin() (string, bool) {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return "", false
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false
	}
	query := strings.TrimSpace(string(data))
	return query, query != ""
}

func (c *SearchCmd) Run(globals *Globals) error {
	resolved := resolveQueries(c.Queries)
	if len(resolved) == 0 && c.Tag == "" {
		// If no positional query is given but stdin is piped (not a
		// terminal), read the query from stdin so the CLI composes in
		// scripts: echo "query" | notebrain search
		if query, ok := readQueryFromStdin(); ok {
			resolved = resolveQueries([]string{query})
		}
	}
	if len(resolved) == 0 && c.Tag == "" {
		return &UsageError{Err: fmt.Errorf("query or --tag is required")}
	}
	if c.TopKPerNote >= 4 {
		fmt.Fprintf(os.Stderr, "warning: --top-k >= 4 may exceed upstream ChromaDB embedded 1 MiB FFI limit on large notes\n")
	}
	// Only multi-query runs produce per-query hit attribution; keep queries
	// nil otherwise so JSON output omits the "queries" field.
	var displayQueries []string
	if len(resolved) > 1 {
		displayQueries = resolved
	}

	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if len(c.ExcludeNote) > 0 {
		c.ExcludeNotes = append(append([]string(nil), c.ExcludeNotes...), c.ExcludeNote...)
	}
	excluded, err := c.resolveExcludes(ctx, st)
	if err != nil {
		return err
	}

	slog.Debug("initializing embedding model")
	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	return c.runStatic(ctx, globals, st, emb, resolved, displayQueries, excluded)
}

// resolveExcludes normalizes, resolves, and validates --exclude-notes
// (and the deprecated --exclude-note alias) values.
// Each value may be a slug, title, filename, or partial path (the same
// resolution `get` and `hidden` use). Values that resolve to nothing are
// reported as a warning so typos do not silently no-op. Returns the resolved
// slugs, deduplicated. An ambiguous value (matching multiple notes) is a
// usage error and aborts the command.
func (c *SearchCmd) resolveExcludes(ctx context.Context, st storeAPI) ([]string, error) {
	if len(c.ExcludeNotes) == 0 {
		return nil, nil
	}
	// A single metadata scan serves both resolution and the existence
	// check for every exclusion, so the cost does not grow with the
	// number of excluded notes.
	resolved, indexed, err := st.ResolveNoteSlugs(ctx, c.ExcludeNotes)
	if err != nil {
		return nil, &UsageError{Err: fmt.Errorf("exclude note: %w", err)}
	}
	seen := make(map[string]struct{}, len(c.ExcludeNotes))
	out := make([]string, 0, len(c.ExcludeNotes))
	for _, raw := range c.ExcludeNotes {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		slug := resolved[value]
		if slug == "" {
			continue
		}
		if _, ok := indexed[slug]; !ok {
			fmt.Fprintf(os.Stderr, "warning: note %q not found; nothing excluded\n", value)
			continue
		}
		if _, ok := seen[slug]; !ok {
			seen[slug] = struct{}{}
			out = append(out, slug)
		}
	}
	return out, nil
}

// lexicalFallback runs the keyword fallback for a query that produced no
// semantic matches (or none above --min-score). It returns nil when the
// fallback also finds nothing.
func (c *SearchCmd) lexicalFallback(ctx context.Context, st storeAPI, query string, limit int, whereFilter store.WhereFilter) ([]store.Result, error) {
	results, err := st.LexicalSearch(ctx, query, limit, whereFilter)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// allBelowMinScore reports whether every row would be dropped by the
// --min-score filter (used to decide whether the lexical fallback should
// run even when the store returned rows).
func allBelowMinScore(results []store.Result, minScore float64) bool {
	if minScore <= 0 {
		return false
	}
	for _, r := range results {
		if r.Score >= minScore || r.Lexical {
			return false
		}
	}
	return true
}

func (c *SearchCmd) runStatic(ctx context.Context, globals *Globals, st storeAPI, emb embedderAPI, resolved, displayQueries, excluded []string) error {
	whereFilter := (&store.SearchFilter{
		Section:     c.Section,
		Tag:         c.Tag,
		HasTasks:    c.HasTasks,
		HasCode:     c.HasCode,
		IncludePDF:  c.WithPDF,
		Exclude:     excluded,
		ResolveTags: len(resolved) > 0,
	}).Build()

	excludeSuffix := ""
	if len(excluded) > 0 {
		excludeSuffix = fmt.Sprintf(" (excluding: %s)", strings.Join(excluded, ", "))
	}

	if len(resolved) == 0 {
		results, err := st.TagSearch(ctx, c.Tag, c.Limit, false, whereFilter, c.IncludeText)
		if err != nil {
			return err
		}
		if err := populateContext(ctx, st, results, c.ContextWindow); err != nil {
			return err
		}
		return printResultsFormatted("search", fmt.Sprintf("Tag Search: %q%s", c.Tag, excludeSuffix), c.Tag, displayQueries, results, globals, &c.ChunkDisplayFlags)
	}

	if len(resolved) > 1 {
		qVecs, err := emb.EmbedBatch(ctx, resolved)
		if err != nil {
			return err
		}
		results, err := st.MultiSemanticSearch(ctx, qVecs, resolved, c.Limit, c.TopKPerNote, whereFilter, c.IncludeText)
		if err != nil {
			return err
		}
		header := fmt.Sprintf("Multi-Hit Semantic Search: %q", strings.Join(resolved, ", "))
		if len(results) == 0 || allBelowMinScore(results, c.MinScore) {
			results, err = c.lexicalFallback(ctx, st, strings.Join(resolved, " "), c.Limit, whereFilter)
			if err != nil {
				return err
			}
			if len(results) > 0 {
				header = fmt.Sprintf("Lexical Search (no semantic matches): %q", strings.Join(resolved, ", "))
			}
		} else if err := populateContext(ctx, st, results, c.ContextWindow); err != nil {
			return err
		}

		if c.Tag != "" {
			header += fmt.Sprintf(" (Tag: %s)", c.Tag)
		}
		header += excludeSuffix
		return printResultsFormatted("search", header, strings.Join(resolved, " | "), displayQueries, results, globals, &c.ChunkDisplayFlags)
	}

	qVec, err := emb.Embed(ctx, resolved[0])
	if err != nil {
		return err
	}
	results, err := st.SemanticSearch(ctx, qVec, c.Limit, c.TopKPerNote, whereFilter, c.IncludeText)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Semantic Search: %q%s", resolved[0], excludeSuffix)
	if len(results) == 0 || allBelowMinScore(results, c.MinScore) {
		results, err = c.lexicalFallback(ctx, st, resolved[0], c.Limit, whereFilter)
		if err != nil {
			return err
		}
		if len(results) > 0 {
			header = fmt.Sprintf("Lexical Search (no semantic matches): %q%s", resolved[0], excludeSuffix)
		}
	} else if err := populateContext(ctx, st, results, c.ContextWindow); err != nil {
		return err
	}

	return printResultsFormatted("search", header, resolved[0], displayQueries, results, globals, &c.ChunkDisplayFlags)
}
