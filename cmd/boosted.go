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

// boostedCmd represents the boosted command.
var boostedCmd = &cobra.Command{
	Use:   "boosted <query>",
	Short: "Graph-boosted semantic search",
	Long: `Perform a semantic search and then boost the scores of results that
are reachable from the seed note via the wikilink graph. This surfaces
results that are both semantically relevant and structurally connected.

Examples:
  notebrain boosted "energy production" --seed "Mitochondria"
  notebrain boosted "cell biology" --seed "ATP" --boost 2.0 --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		seed, _ := cmd.Flags().GetString("seed")
		seedSlug := parser.Slugify(seed)
		boost, _ := cmd.Flags().GetFloat64("boost")
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

		qVec, err := emb.Embed(ctx, query)
		if err != nil {
			return err
		}

		results, err := st.GraphBoostedSearch(ctx, qVec, seedSlug, boost, limit)
		if err != nil {
			return err
		}

		fmt.Printf("Graph-Boosted Search Results for: %q (seed: %s, boost: %.2f)\n\n", query, seedSlug, boost)
		vaultPath, _ := cmd.Flags().GetString("vault")
		vaultName := filepath.Base(vaultPath)

		for _, r := range results {
			fmt.Printf("■ %s\n  Boosted Score: %.4f | File: %s\n\n", formatObsidianLink(vaultName, r.Title, r.FilePath), r.Score, r.FilePath)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(boostedCmd)

	boostedCmd.Flags().String("seed", "", "seed note for graph-based score boosting (required)")
	_ = boostedCmd.MarkFlagRequired("seed")
	boostedCmd.Flags().Float64("boost", 1.5, "multiplier applied to graph-connected results")
	boostedCmd.Flags().Int("limit", 10, "maximum number of results to return")
}
