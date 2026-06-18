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
	"path/filepath"

	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// tagsCmd represents the tags command.
var tagsCmd = &cobra.Command{
	Use:   "tags <note>",
	Short: "Find notes sharing tags with a given note",
	Long: `Look up the tags on the specified note and find other indexed notes
that share one or more of those tags. Results are ranked by the number of
shared tags.

Examples:
  notebrain tags "Mitochondria"
  notebrain tags "Mitochondria" --min-shared 2`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote := args[0]
		targetSlug := parser.Slugify(targetNote)
		minShared, _ := cmd.Flags().GetInt("min-shared")
		ctx := cmd.Context()

		chromaPath, _ := cmd.Flags().GetString("chroma-path")
		st, err := store.Open(ctx, chromaPath)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		nodes, err := st.SharedTags(ctx, targetSlug, minShared)
		if err != nil {
			return err
		}

		fmt.Printf("Notes sharing tags with: %q (slug: %s) [Min Shared: %d]\n\n", targetNote, targetSlug, minShared)
		if len(nodes) == 0 {
			fmt.Println("No related notes found.")
			return nil
		}
		vaultPath, _ := cmd.Flags().GetString("vault")
		vaultName := filepath.Base(vaultPath)

		for _, n := range nodes {
			fmt.Printf("■ %s\n", formatObsidianLink(vaultName, n.Title, n.FilePath))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tagsCmd)

	tagsCmd.Flags().Int("min-shared", 1, "minimum number of shared tags to include a result")
}
