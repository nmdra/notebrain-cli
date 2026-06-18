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

	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/ingest"
	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// ingestCmd represents the ingest command.
var ingestCmd = &cobra.Command{
	Use:   "ingest [glob]",
	Short: "Index vault notes into ChromaDB",
	Long: `Parse Markdown files from the configured Obsidian vault, extract
chunks and metadata, compute embeddings, and upsert them into ChromaDB.

An optional glob pattern filters which files to ingest (default: all .md files).

Examples:
  notebrain ingest
  notebrain ingest "**/*.md"
  notebrain ingest --workers 8`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workers, _ := cmd.Flags().GetInt("workers")
		vaultPath, _ := cmd.Flags().GetString("vault")
		if vaultPath == "" {
			return fmt.Errorf("--vault must be specified")
		}

		glob := ""
		if len(args) > 0 {
			glob = args[0]
		}

		ctx := cmd.Context()
		chromaPath, _ := cmd.Flags().GetString("chroma-path")

		fmt.Println("Opening ChromaDB store...")
		st, err := store.Open(ctx, chromaPath)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		fmt.Println("Initializing embedded ONNX vector models...")
		emb, err := embedder.NewLocalEmbedder()
		if err != nil {
			return err
		}
		defer func() { _ = emb.Close() }()

		fmt.Printf("Starting ingestion pipeline with %d workers...\n", workers)
		pipeline := ingest.NewPipeline(st, emb, workers)
		return pipeline.Run(ctx, vaultPath, glob)
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	ingestCmd.Flags().Int("workers", 4, "number of concurrent ingestion workers")
}
