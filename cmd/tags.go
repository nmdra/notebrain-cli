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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type TagsCmd struct {
	Query     string `arg:"" optional:"" help:"Tag name to search for (e.g. 'kubernetes'), or note slug/title if --shared or --for-note is used." completion-predictor:"note-slug"`
	List      bool   `group:"tags" help:"List all indexed tags with note counts instead of searching (query is ignored)." default:"false"`
	Shared    bool   `group:"tags" help:"Find notes sharing tags with the given note instead of searching by tag name." default:"false"`
	ForNote   bool   `group:"tags" name:"for-note" help:"Alias for --shared."`
	Children  bool   `group:"tags" help:"Include child tags in the hierarchy (e.g. 'kubernetes' also matches 'kubernetes/cka')." default:"false"`
	MinShared int    `group:"tags" help:"Minimum shared tags to include a result (only with --shared/--for-note)." default:"1"`
	Limit     int    `group:"tags" help:"maximum number of results (0 = no limit for --list; searches default to 50)" default:"0"`
	ChunkDisplayFlags
}

func (c *TagsCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if c.List {
		tags, listErr := st.ListTags(ctx, c.Limit)
		if listErr != nil {
			return listErr
		}
		if printErr := printTagsFormatted("tags --list", tags, globals); printErr != nil {
			return printErr
		}
		return nil
	}

	if strings.TrimSpace(c.Query) == "" {
		return &UsageError{Err: errors.New("tags requires a tag name to search for, or use --list to enumerate all tags")}
	}

	limit := c.Limit
	if limit <= 0 {
		limit = 50
	}

	if c.Shared || c.ForNote {
		var targetSlug string
		targetSlug, err = st.ResolveNoteSlug(ctx, c.Query)
		if err != nil {
			return err
		}

		var nodes []store.Result
		nodes, err = st.SharedTags(ctx, targetSlug, c.MinShared)
		if err != nil {
			return err
		}
		if len(nodes) > limit {
			nodes = nodes[:limit]
		}
		if err = populateContext(ctx, st, nodes, c.ContextWindow); err != nil {
			return err
		}

		return printResultsFormatted("tags --shared", fmt.Sprintf("Notes sharing tags with: %q (slug: %s) [Min Shared: %d]", c.Query, targetSlug, c.MinShared), targetSlug, nil, nodes, globals, &c.ChunkDisplayFlags)
	}

	// Direct tag search (default)
	normalizedTag := normalizeTagInput(c.Query)
	nodes, err := st.TagSearch(ctx, normalizedTag, limit, c.Children, nil, c.IncludeText)
	if err != nil {
		return err
	}
	if err := populateContext(ctx, st, nodes, c.ContextWindow); err != nil {
		return err
	}

	commandName := "tags"
	title := fmt.Sprintf("Notes containing tag: %q", c.Query)
	if c.Children {
		commandName = "tags --children"
		title = fmt.Sprintf("Notes containing tag: %q (and children tags)", c.Query)
	}

	if err := printResultsFormatted(commandName, title, "", nil, nodes, globals, &c.ChunkDisplayFlags); err != nil {
		return err
	}

	// "Did you mean" suggestions for misspelled tags, text output only so
	// machine formats stay clean (AGENTS.md).
	if len(nodes) == 0 && !c.Children && globals.Format == formatText {
		suggestions, serr := st.SuggestTags(ctx, normalizedTag, 3)
		if serr == nil && len(suggestions) > 0 {
			chips := make([]string, len(suggestions))
			for i, s := range suggestions {
				chips[i] = "#" + s
			}
			_, _ = fmt.Fprintln(os.Stdout, hintStyle.Render("  Did you mean: "+strings.Join(chips, ", ")+"?"))
		}
	}

	return nil
}

func normalizeTagInput(input string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(input), "#"))
}
