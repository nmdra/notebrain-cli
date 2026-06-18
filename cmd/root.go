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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "notebrain",
	Short: "Index and search your Obsidian vault with semantic intelligence",
	Long: `NoteBrain indexes an Obsidian vault into ChromaDB and provides
semantic search, backlink traversal, graph connections, hidden-link
discovery, shared-tag queries, and graph-boosted search.

Examples:
  notebrain ingest "**/*.md"
  notebrain search "how does photosynthesis work"
  notebrain backlinks "Mitochondria"
  notebrain connections "Mitochondria" --hops 3
  notebrain hidden "Mitochondria"
  notebrain tags "Mitochondria" --min-shared 2
  notebrain boosted "energy production" --seed "Mitochondria"
  notebrain stats
  notebrain reset`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	noHyperlinks   bool
	printVaultName string
)

func init() {
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		vault, _ := cmd.Flags().GetString("vault")
		if vault == "" {
			// If no flag provided, try the explicitly named vault env var first
			vault = os.Getenv("OBSIDIAN_VAULT_NAME")
			if vault == "" {
				// Fall back to the legacy one or extracting the name from the path
				legacyVault := os.Getenv("OBSIDIAN_VAULT")
				if legacyVault != "" {
					vault = legacyVault
				} else {
					vaultPath := os.Getenv("OBSIDIAN_VAULT_PATH")
					if vaultPath != "" {
						vault = vaultPath
					}
				}
			}
		}
		if vault != "" {
			vault = filepath.Base(vault)
		}
		printVaultName = vault
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	defaultChromaPath := filepath.Join(home, ".notebrain", "chroma")

	rootCmd.PersistentFlags().String("chroma-path", defaultChromaPath, "path to ChromaDB persistent storage")
	rootCmd.PersistentFlags().String("chroma-mode", "persistent", `ChromaDB client mode ("persistent" or "http")`)
	rootCmd.PersistentFlags().String("chroma-url", "http://localhost:8000", "ChromaDB server URL (used when --chroma-mode=http)")
	rootCmd.PersistentFlags().String("vault", "", "Obsidian vault name")
	rootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noHyperlinks, "no-hyperlinks", false, "Disable OSC 8 terminal hyperlinks in output")
}
