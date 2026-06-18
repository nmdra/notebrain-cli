package cmd

import (
	"fmt"
	"github.com/nmdra/notebrain-cli/internal/store"
)

func printResults(header string, results []store.Result) {
	useLinks := hyperlinkSupported()

	fmt.Printf("\n=== %s ===\n", header)
	if len(results) == 0 {
		fmt.Println("(no results)")
		return
	}

	for i, r := range results {
		extra := ""
		if r.Extra != "" {
			extra = "  [" + r.Extra + "]"
		}

		// Build the clickable title
		title := r.Title
		if useLinks && r.FilePath != "" {
			uri := obsidianURI(printVaultName, r.FilePath)
			title = hyperlink(uri, r.Title)
		}

		fmt.Printf("%2d. %-42s  score=%.4f%s\n", i+1, title, r.Score, extra)
	}

	if useLinks {
		fmt.Println("\n  (Ctrl+click or Cmd+click a title to open in Obsidian)")
	}
	fmt.Println()
}
