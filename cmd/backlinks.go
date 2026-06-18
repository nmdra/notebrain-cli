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

// backlinksCmd represents the backlinks command.
var backlinksCmd = &cobra.Command{
	Use:   "backlinks <note>",
	Short: "Find notes linking to a given note",
	Long: `Query the nb_links collection in ChromaDB to find every note that
contains a wikilink pointing to the specified note.

Examples:
  notebrain backlinks "Mitochondria"
  notebrain backlinks "Daily Notes/2026-06-18"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote := args[0]
		targetSlug := parser.Slugify(targetNote)
		ctx := cmd.Context()

		chromaPath, _ := cmd.Flags().GetString("chroma-path")
		st, err := store.Open(ctx, chromaPath)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		links, err := st.Backlinks(ctx, targetSlug)
		if err != nil {
			return err
		}

		fmt.Printf("Backlinks for: %q (slug: %s)\n\n", targetNote, targetSlug)
		if len(links) == 0 {
			fmt.Println("No backlinks found.")
			return nil
		}
		vaultPath, _ := cmd.Flags().GetString("vault")
		vaultName := filepath.Base(vaultPath)

		for _, l := range links {
			fmt.Printf("■ %s\n", formatObsidianLink(vaultName, l.Title, l.FilePath))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
