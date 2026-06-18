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

	"github.com/nmdra/notebrain-cli/internal/store"
	"github.com/spf13/cobra"
)

// statsCmd represents the stats command.
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show collection statistics",
	Long: `Display statistics for the ChromaDB collections used by NoteBrain,
including the number of chunks, links, unique notes, and embedding dimensions.

Examples:
  notebrain stats`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		chromaPath, _ := cmd.Flags().GetString("chroma-path")
		st, err := store.Open(ctx, chromaPath)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		stats, err := st.Stats(ctx)
		if err != nil {
			return err
		}

		fmt.Println("NoteBrain ChromaDB Statistics")
		fmt.Println("=============================")
		fmt.Printf("Total Chunks : %d\n", stats["chunks"])
		fmt.Printf("Total Links  : %d\n", stats["links"])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
