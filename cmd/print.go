package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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

type jsonEnvelope struct {
	Command string         `json:"command"`
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Results []store.Result `json:"results"`
}

// printResultsFormatted renders a list of results to stdout based on the requested format.
func printResultsFormatted(commandName string, query string, results []store.Result, globals *Globals) {
	// 1. Filter by min score
	var filtered []store.Result
	for _, r := range results {
		if r.Score >= globals.MinScore {
			filtered = append(filtered, r)
		}
	}
	if filtered == nil {
		filtered = []store.Result{} // avoid null in JSON
	}

	// 2. Route by format
	switch globals.Format {
	case "json":
		env := jsonEnvelope{
			Command: commandName,
			Query:   query,
			Total:   len(filtered),
			Results: filtered,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(env)

	case "ndjson":
		enc := json.NewEncoder(os.Stdout)
		for _, r := range filtered {
			_ = enc.Encode(r)
		}

	case "tsv":
		fmt.Println("slug\ttitle\tfile_path\tscore\textra\theading_path\ttext")
		for _, r := range filtered {
			// clean tabs and newlines from text/title just in case
			fmt.Printf("%s\t%s\t%s\t%f\t%s\t%s\t%s\n",
				r.NoteSlug, r.Title, r.FilePath, r.Score, r.Extra, r.HeadingPath, r.Text)
		}

	default: // "text"
		fmt.Println(headerStyle.Render(query))

		if len(filtered) == 0 {
			fmt.Println(extraStyle.Render("  (no results)"))
			return
		}

		useLinks := hyperlinkSupported(globals)

		for i, r := range filtered {
			rank := rankStyle.Render(fmt.Sprintf("%d.", i+1))

			paddedTitle := lipgloss.NewStyle().Width(42).Render(r.Title)
			title := paddedTitle

			if useLinks && r.FilePath != "" {
				uri := store.ObsidianURI(globals.VaultName, r.FilePath)
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
}
