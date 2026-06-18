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

	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// hiddenCmd represents the hidden command.
var hiddenCmd = &cobra.Command{
	Use:   "hidden <note>",
	Short: "Find semantically similar but unlinked notes",
	Long: `Discover "hidden connections" — notes that are semantically close to
the given note but have no direct wikilink relationship. These are notes
you might want to link together.

Examples:
  notebrain hidden "Mitochondria"
  notebrain hidden "Mitochondria" --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote := args[0]
		targetSlug := parser.Slugify(targetNote)
		limit, _ := cmd.Flags().GetInt("limit")
		ctx := cmd.Context()

		chromaPath, _ := cmd.Flags().GetString("chroma-path")
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

		qVec, err := emb.Embed(ctx, targetNote) // Embed the note title as the search context
		if err != nil {
			return err
		}

		results, err := st.HiddenConnections(ctx, qVec, targetSlug, limit)
		if err != nil {
			return err
		}

		fmt.Printf("Hidden connections for: %q (slug: %s)\n\n", targetNote, targetSlug)
		if len(results) == 0 {
			fmt.Println("No hidden connections found.")
			return nil
		}
		vaultPath, _ := cmd.Flags().GetString("vault")
		vaultName := filepath.Base(vaultPath)

		for _, r := range results {
			fmt.Printf("■ %s\n  Distance: %.4f | File: %s\n\n", formatObsidianLink(vaultName, r.Title, r.FilePath), r.Score, r.FilePath)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(hiddenCmd)

	hiddenCmd.Flags().Int("limit", 10, "maximum number of hidden connections to return")
}
