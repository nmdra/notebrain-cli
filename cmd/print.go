package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/PaesslerAG/jsonpath"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

var getTerminalWidth = func() int {
	w, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

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
	Queries []string       `json:"queries,omitempty"`
	Total   int            `json:"total"`
	Results []store.Result `json:"results"`
}

// printResultsFormatted renders a list of results to stdout based on the requested format.
func printResultsFormatted(commandName string, query string, results []store.Result, globals *Globals) {
	// 1. Filter by min score and phantom links
	filtered := make([]store.Result, 0, len(results))
	for _, r := range results {
		if r.Score < globals.MinScore {
			continue
		}
		if globals.SkipPhantom && r.IsPhantom {
			continue
		}
		if globals.HideTags {
			r.Tags = nil
		}
		filtered = append(filtered, r)
	}

	if globals.JSONPath != "" {
		env := jsonEnvelope{
			Command: commandName,
			Query:   query,
			Queries: globals.Queries,
			Total:   len(filtered),
			Results: filtered,
		}
		if err := printJSONPathResult(env, globals.JSONPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return
	}

	// 2. Route by format
	switch globals.Format {
	case "json":
		env := jsonEnvelope{
			Command: commandName,
			Query:   query,
			Queries: globals.Queries,
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
		fmt.Println("slug\ttitle\tfile_path\tscore\ttags\textra\theading_path\ttext")
		for _, r := range filtered {
			tagsStr := strings.Join(r.Tags, ",")
			fmt.Printf("%s\t%s\t%s\t%f\t%s\t%s\t%s\t%s\n",
				r.NoteSlug, r.Title, r.FilePath, r.Score, tagsStr, r.Extra, r.HeadingPath, r.Text)
		}

	default: // "text"
		fmt.Println(headerStyle.Render(query))

		if len(filtered) == 0 {
			fmt.Println(extraStyle.Render("  (no results)"))
			return
		}

		useLinks := hyperlinkSupported(globals)
		termWidth := getTerminalWidth()

		for i, r := range filtered {
			rank := rankStyle.Render(fmt.Sprintf("%d.", i+1))

			displayTitle := r.Title
			if r.HeadingPath != "" {
				displayTitle = fmt.Sprintf("%s (§ %s)", displayTitle, r.HeadingPath)
			} else if len(filtered) > 1 {
				sameNoteCount := 0
				for _, other := range filtered {
					if other.NoteSlug == r.NoteSlug {
						sameNoteCount++
					}
				}
				if sameNoteCount > 1 {
					displayTitle = fmt.Sprintf("%s (chunk #%d)", displayTitle, r.ChunkIndex+1)
				}
			}

			titleWidth := 42
			if termWidth > 0 {
				titleWidth = max(min(termWidth-40, 80), 20)
				displayTitle = ansi.Truncate(displayTitle, titleWidth, "…")
			}

			paddedTitle := lipgloss.NewStyle().Width(titleWidth).Render(displayTitle)
			title := paddedTitle

			if useLinks && r.FilePath != "" {
				uri := store.ObsidianURI(globals.VaultName, r.FilePath)
				title = hyperlink(true, uri, paddedTitle)
			}

			score := scoreStyle.Render(fmt.Sprintf("score=%.4f", r.Score))
			line := fmt.Sprintf("%s %s  %s", rank, title, score)

			if strings.Contains(commandName, "deep") {
				if r.Extra != "" {
					line += "  " + extraStyle.Render("["+r.Extra+"]")
				}
				if r.IsPhantom {
					line += "  " + extraStyle.Render("[phantom]")
				}
				if termWidth > 0 && ansi.StringWidth(line) > termWidth {
					line = ansi.Truncate(line, termWidth, "…")
				}
				fmt.Println(line)

				var details []string
				if len(r.MatchedQueries) > 0 {
					details = append(details, fmt.Sprintf("Matches target sections: %s", extraStyle.Render(`"`+strings.Join(r.MatchedQueries, `", "`)+`"`)))
				}
				if len(r.Tags) > 0 {
					formattedTags := make([]string, 0, len(r.Tags))
					for _, t := range r.Tags {
						formattedTags = append(formattedTags, "#"+t)
					}
					details = append(details, fmt.Sprintf("Tags: %s", extraStyle.Render(strings.Join(formattedTags, " "))))
				}
				if globals.IncludeText && r.Text != "" {
					cleanText := strings.ReplaceAll(strings.TrimSpace(r.Text), "\n", " ")
					maxTextWidth := 80
					if termWidth > 20 {
						maxTextWidth = termWidth - 12
					}
					if ansi.StringWidth(cleanText) > maxTextWidth {
						cleanText = ansi.Truncate(cleanText, maxTextWidth, "…")
					}
					details = append(details, fmt.Sprintf("Text: %s", extraStyle.Render(`"`+cleanText+`"`)))
				}

				for j, d := range details {
					prefix := "   ├─ "
					if j == len(details)-1 {
						prefix = "   └─ "
					}
					fmt.Println(prefix + d)
				}
				if i < len(filtered)-1 && len(details) > 0 {
					fmt.Println()
				}
				continue
			}

			if len(r.Tags) > 0 {
				formattedTags := make([]string, 0, len(r.Tags))
				for _, t := range r.Tags {
					formattedTags = append(formattedTags, "#"+t)
				}
				line += "  " + extraStyle.Render("["+strings.Join(formattedTags, " ")+"]")
			}
			if len(r.MatchedQueries) > 0 && len(globals.Queries) > 1 {
				line += "  " + extraStyle.Render(`[hits: "`+strings.Join(r.MatchedQueries, `", "`)+`"]`)
			}
			if r.Extra != "" {
				line += "  " + extraStyle.Render("["+r.Extra+"]")
			}
			if r.IsPhantom {
				line += "  " + extraStyle.Render("[phantom]")
			}

			if termWidth > 0 && ansi.StringWidth(line) > termWidth {
				line = ansi.Truncate(line, termWidth, "…")
			}

			fmt.Println(line)
		}

		if useLinks {
			fmt.Println("\n  " + extraStyle.Render("(Ctrl+click / Cmd+click a title to open in Obsidian)"))
		}
		fmt.Println("  " + extraStyle.Render("Note: Results are matching text chunks; Repeated titles represent different relevant sections."))
		fmt.Println()
	}
}

func normalizeJSONPath(jp string) string {
	jp = strings.TrimSpace(jp)
	// If kubectl style {.items[0]} or {$.items[0]}
	if strings.HasPrefix(jp, "{") && strings.HasSuffix(jp, "}") {
		jp = strings.TrimPrefix(jp, "{")
		jp = strings.TrimSuffix(jp, "}")
		jp = strings.TrimSpace(jp)
	}
	if strings.HasPrefix(jp, ".") && !strings.HasPrefix(jp, "..") {
		jp = "$" + jp
	} else if !strings.HasPrefix(jp, "$") && !strings.HasPrefix(jp, "@") {
		jp = "$." + jp
	}
	return jp
}

func printJSONPathResult(obj any, jp string) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("jsonpath marshal: %w", err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("jsonpath unmarshal: %w", err)
	}

	normPath := normalizeJSONPath(jp)
	res, err := jsonpath.Get(normPath, raw)
	if err != nil {
		return fmt.Errorf("evaluate jsonpath %q: %w", jp, err)
	}

	if res == nil {
		return nil
	}

	switch val := res.(type) {
	case []any:
		for _, item := range val {
			printSingleJSONPathValue(item)
		}
	default:
		printSingleJSONPathValue(val)
	}
	return nil
}

func printSingleJSONPathValue(val any) {
	switch v := val.(type) {
	case string:
		fmt.Println(v)
	case float64, float32, int, int64, bool:
		fmt.Printf("%v\n", v)
	default:
		enc := json.NewEncoder(os.Stdout)
		_ = enc.Encode(v)
	}
}
