package cmd

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/nmdra/notebrain-cli/internal/store"
)

// hyperlink wraps visible text in an OSC 8 terminal hyperlink.
func hyperlink(useLinks bool, uri, text string) string {
	if !useLinks {
		return text
	}
	// OSC 8 format: ESC ] 8 ; params ; uri ESC \  text  ESC ] 8 ; ; ESC \
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", uri, text)
}

// printResults renders a list of results to stdout with styled formatting.
// vaultName is used for generating Obsidian URIs in hyperlinks.
func printResults(header string, results []store.Result, vaultName string, useLinks bool) {
	fmt.Println(headerStyle.Render(header))

	if len(results) == 0 {
		fmt.Println(extraStyle.Render("  (no results)"))
		return
	}

	for i, r := range results {
		rank := rankStyle.Render(fmt.Sprintf("%d.", i+1))

		// Apply style and truncation first
		paddedTitle := lipgloss.NewStyle().Width(42).Render(r.Title)
		title := paddedTitle

		// Then wrap with OSC 8 if supported
		if useLinks && r.FilePath != "" {
			uri := store.ObsidianURI(vaultName, r.FilePath)
			title = hyperlink(true, uri, paddedTitle)
		}

		score := scoreStyle.Render(fmt.Sprintf("score=%.4f", r.Score))
		line := fmt.Sprintf("%s %s  %s", rank, title, score)

		if r.Extra != "" {
			line += "  " + extraStyle.Render("["+r.Extra+"]")
		}
		fmt.Println(line)
	}

	if useLinks {
		fmt.Println("\n  " + extraStyle.Render("(Ctrl+click or Cmd+click a title to open in Obsidian)"))
	}
	fmt.Println()
}
