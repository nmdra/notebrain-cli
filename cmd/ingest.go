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
		fmt.Println("ingest called")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	ingestCmd.Flags().Int("workers", 4, "number of concurrent ingestion workers")
}
