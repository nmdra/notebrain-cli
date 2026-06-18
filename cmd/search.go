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

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// searchCmd represents the search command.
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search across indexed notes",
	Long: `Embed the query string and find the most semantically similar note
chunks stored in ChromaDB. Results are ranked by cosine similarity.

Examples:
  notebrain search "how does photosynthesis work"
  notebrain search "mitochondria energy" --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("search requires a query string")
		}
		query := args[0]
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

		// Build filters based on flags
		var filters []chroma.WhereClause

		if section, _ := cmd.Flags().GetString("section"); section != "" {
			filters = append(filters, chroma.EqString("heading_path", section))
		}
		if hasTasks, _ := cmd.Flags().GetBool("has-tasks"); hasTasks {
			filters = append(filters, chroma.EqBool("has_task", true))
		}
		if hasCode, _ := cmd.Flags().GetBool("has-code"); hasCode {
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

		printResults(fmt.Sprintf("Semantic Search: %q", query), results)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().Int("limit", 10, "maximum number of results to return")
	searchCmd.Flags().String("section", "", "filter by heading path (e.g. 'Project > Tasks')")
	searchCmd.Flags().Bool("has-tasks", false, "only return chunks that contain task lists")
	searchCmd.Flags().Bool("has-code", false, "only return chunks that contain code blocks")
}
