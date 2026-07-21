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
	"strings"
)

type TagsCmd struct {
	Query     string `arg:"" help:"Tag name to search for (e.g. 'kubernetes'), or note slug/title if --shared is used."`
	Shared    bool   `help:"Find notes sharing tags with the given note instead of searching by tag name." default:"false"`
	Children  bool   `help:"Include child tags in the hierarchy (e.g. 'kubernetes' also matches 'kubernetes/cka')." default:"false"`
	MinShared int    `help:"Minimum shared tags to include a result (only with --shared)." default:"1"`
}

func (c *TagsCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if c.Shared {
		targetSlug, err := st.ResolveNoteSlug(ctx, c.Query)
		if err != nil {
			return err
		}

		nodes, err := st.SharedTags(ctx, targetSlug, c.MinShared)
		if err != nil {
			return err
		}

		printResultsFormatted("tags --shared", fmt.Sprintf("Notes sharing tags with: %q (slug: %s) [Min Shared: %d]", c.Query, targetSlug, c.MinShared), targetSlug, nodes, globals, nil)
		return nil
	}

	// Direct tag search (default)
	normalizedTag := normalizeTagInput(c.Query)
	nodes, err := st.TagSearch(ctx, normalizedTag, 999999, c.Children, nil, false)
	if err != nil {
		return err
	}

	commandName := "tags"
	title := fmt.Sprintf("Notes containing tag: %q", c.Query)
	if c.Children {
		commandName = "tags --children"
		title = fmt.Sprintf("Notes containing tag: %q (and children tags)", c.Query)
	}

	printResultsFormatted(commandName, title, "", nodes, globals, nil)
	return nil
}

func normalizeTagInput(input string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(input), "#"))
}
