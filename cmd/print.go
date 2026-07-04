package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/PaesslerAG/jsonpath"
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
	filtered := make([]store.Result, 0, len(results))
	for _, r := range results {
		if r.Score >= globals.MinScore {
			filtered = append(filtered, r)
		}
	}

	if globals.JSONPath != "" {
		env := jsonEnvelope{
			Command: commandName,
			Query:   query,
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

			if len(r.Tags) > 0 {
				formattedTags := make([]string, 0, len(r.Tags))
				for _, t := range r.Tags {
					formattedTags = append(formattedTags, "#"+t)
				}
				line += "  " + extraStyle.Render("["+strings.Join(formattedTags, " ")+"]")
			}
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
