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

	"github.com/nmdra/notebrain-cli/internal/parser"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// connectionsCmd represents the connections command.
var connectionsCmd = &cobra.Command{
	Use:   "connections <note>",
	Short: "Find notes connected via graph traversal",
	Long: `Starting from the given note, perform a breadth-first traversal of
the wikilink graph stored in nb_links. Returns all notes reachable within
the specified number of hops.

Examples:
  notebrain connections "Mitochondria"
  notebrain connections "Mitochondria" --hops 3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetNote := args[0]
		targetSlug := parser.Slugify(targetNote)
		hops, _ := cmd.Flags().GetInt("hops")
		ctx := cmd.Context()

		chromaPath, _ := cmd.Flags().GetString("chroma-path")
		st, err := store.Open(ctx, chromaPath)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		nodes, err := st.Connections(ctx, targetSlug, hops)
		if err != nil {
			return err
		}

		printResults(fmt.Sprintf("Graph Connections from: %q (slug: %s) [Hops: %d]", targetNote, targetSlug, hops), nodes)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(connectionsCmd)

	connectionsCmd.Flags().Int("hops", 2, "maximum number of graph hops to traverse")
}
