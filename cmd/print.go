package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/PaesslerAG/jsonpath"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

const (
	formatText   = "text"
	formatJSON   = "json"
	formatNDJSON = "ndjson"
)

var getTerminalWidth = func() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
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
	Command string         `json:"command,omitempty"`
	Query   string         `json:"query"`
	Queries []string       `json:"queries,omitempty"`
	Total   int            `json:"total"`
	Results []store.Result `json:"results"`
}

// printResultsFormatted renders a list of results to stdout based on the requested format.
func printResultsFormatted(commandName string, headerQuery string, rawQuery string, results []store.Result, globals *Globals, displayFlags *ChunkDisplayFlags) {
	printResultsFormattedToWriter(os.Stdout, commandName, headerQuery, rawQuery, results, globals, displayFlags)
}

func printResultsFormattedToWriter(w io.Writer, commandName string, headerQuery string, rawQuery string, results []store.Result, globals *Globals, displayFlags *ChunkDisplayFlags) {
	initStyles()
	filtered := filterResults(results, globals, displayFlags)

	queryStr := headerQuery
	if globals.Format != formatText || globals.JSONPath != "" {
		if rawQuery != "" {
			queryStr = rawQuery
		}
	}

	cmdName := commandName

	if globals.JSONPath != "" {
		env := jsonEnvelope{
			Command: cmdName,
			Query:   queryStr,
			Queries: globals.Queries,
			Total:   len(filtered),
			Results: filtered,
		}
		if err := printJSONPathResultToWriter(w, env, globals.JSONPath); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return
	}

	switch globals.Format {
	case formatJSON:
		printJSONResults(w, cmdName, queryStr, filtered, globals)
	case "tsv":
		printTSVResults(w, filtered)
	default: // "text"
		printTextResults(w, commandName, headerQuery, filtered, globals)
	}
}

func filterResults(results []store.Result, globals *Globals, displayFlags *ChunkDisplayFlags) []store.Result {
	filtered := make([]store.Result, 0, len(results))
	minScore := 0.0
	includeText := false
	if displayFlags != nil {
		minScore = displayFlags.MinScore
		includeText = displayFlags.IncludeText
	}

	for _, r := range results {
		if r.Score < minScore {
			continue
		}
		if globals.SkipPhantom && r.IsPhantom {
			continue
		}
		if globals.HideTags {
			r.Tags = nil
		}
		if !globals.ShowFilePath {
			r.FilePath = ""
		}
		if !includeText {
			r.Text = ""
		}
		r.Score = math.Round(r.Score*10000) / 10000
		filtered = append(filtered, r)
	}
	return filtered
}

func printJSONResults(w io.Writer, commandName, query string, filtered []store.Result, globals *Globals) {
	env := jsonEnvelope{
		Command: commandName,
		Query:   query,
		Queries: globals.Queries,
		Total:   len(filtered),
		Results: filtered,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

func printTSVResults(w io.Writer, filtered []store.Result) {
	_, _ = fmt.Fprintln(w, "slug\ttitle\tfile_path\tscore\ttags\textra\theading_path\ttext")
	for _, r := range filtered {
		tagsStr := formatTags(r.Tags)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%f\t%s\t%s\t%s\t%s\n",
			r.NoteSlug, r.Title, r.FilePath, r.Score, tagsStr, r.Extra, r.HeadingPath, r.Text)
	}
}

func emptyResultHint(commandName string) string {
	if strings.HasPrefix(commandName, "backlinks") {
		return "No incoming links found. Other notes may not reference this note, or the vault may need re-indexing: notebrain ingest"
	}
	if strings.HasPrefix(commandName, "connections") {
		return "No graph connections found within N hops. Try increasing --hops or check that the note has wikilinks."
	}
	if strings.HasPrefix(commandName, "hidden --deep") {
		if strings.Contains(commandName, "--include-linked") {
			return "No semantically similar notes found (deep mode). The note may be too unique, or the vault may need re-indexing: notebrain ingest"
		}
		return "No hidden connections found (deep mode). All semantically similar notes may already be linked. Try --include-linked to include them."
	}
	if strings.HasPrefix(commandName, "hidden") {
		if strings.Contains(commandName, "--include-linked") {
			return "No semantically similar notes found. The note may be too unique, or the vault may need re-indexing: notebrain ingest"
		}
		return "No hidden connections found. All semantically similar notes may already be linked. Try --include-linked to include them."
	}
	if strings.HasPrefix(commandName, "tags") {
		if strings.Contains(commandName, "--shared") {
			return "No notes share tags with this note. The note may have no tags, or try lowering --min-shared."
		}
		if strings.Contains(commandName, "--children") {
			return "No notes found with this tag or its children. Check that the tag is correct or that the vault is indexed: notebrain ingest"
		}
		return "No notes found with this tag. Try enabling hierarchical search with --children, or check that the vault is indexed: notebrain ingest"
	}
	if strings.HasPrefix(commandName, "search") {
		return "No matching notes found. Try broadening your query, or check that the vault is indexed: notebrain ingest"
	}
	if strings.HasPrefix(commandName, "boosted") {
		return "No results for this query. Try broadening your search terms or adjusting --boost."
	}
	return ""
}

func printTextResults(w io.Writer, commandName, query string, filtered []store.Result, globals *Globals) {
	_, _ = fmt.Fprintln(w, headerStyle.Render(query))

	if len(filtered) == 0 {
		hint := emptyResultHint(commandName)
		if hint != "" {
			_, _ = fmt.Fprintln(w, hintStyle.Render("  "+hint))
		} else {
			_, _ = fmt.Fprintln(w, extraStyle.Render("  (no results)"))
		}
		return
	}

	useLinks := hyperlinkSupported(globals) && globals.ShowFilePath
	termWidth := getTerminalWidth()

	noteCounts := make(map[string]int, len(filtered))
	for _, r := range filtered {
		noteCounts[r.NoteSlug]++
	}

	for i, r := range filtered {
		rank := rankStyle.Render(fmt.Sprintf("%d.", i+1))

		displayTitle := r.Title
		if r.HeadingPath != "" {
			displayTitle = fmt.Sprintf("%s (§ %s)", displayTitle, r.HeadingPath)
		} else if noteCounts[r.NoteSlug] > 1 {
			displayTitle = fmt.Sprintf("%s (chunk #%d)", displayTitle, r.ChunkIndex+1)
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

		scoreStr := fmt.Sprintf("score=%.4f", r.Score)
		score := scoreStyleFor(r.Score).Render(scoreStr)
		line := fmt.Sprintf("%s %s  %s", rank, title, score)

		if r.Extra != "" {
			line += "  " + extraStyle.Render("["+r.Extra+"]")
		}
		if r.IsPhantom {
			line += "  " + extraStyle.Render("[phantom]")
		}

		if strings.Contains(commandName, "deep") {
			if termWidth > 0 && ansi.StringWidth(line) > termWidth {
				line = ansi.Truncate(line, termWidth, "…")
			}
			_, _ = fmt.Fprintln(w, line)
			printDeepDetails(w, r, termWidth, globals)
			if i < len(filtered)-1 {
				_, _ = fmt.Fprintln(w)
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

		if termWidth > 0 && ansi.StringWidth(line) > termWidth {
			line = ansi.Truncate(line, termWidth, "…")
		}

		_, _ = fmt.Fprintln(w, line)
	}

	if useLinks {
		_, _ = fmt.Fprintln(w, "\n  "+extraStyle.Render("(Ctrl+click / Cmd+click a title to open in Obsidian)"))
	}
	_, _ = fmt.Fprintln(w, "  "+extraStyle.Render("Note: Results are matching text chunks; Repeated titles represent different relevant sections."))
	_, _ = fmt.Fprintln(w)
}

func printDeepDetails(w io.Writer, r store.Result, termWidth int, globals *Globals) {
	var details []string
	if len(r.MatchedQueries) > 0 {
		if globals.Verbose || len(r.MatchedQueries) <= 3 {
			details = append(details, fmt.Sprintf("Matched target sections (%d): %s", len(r.MatchedQueries), extraStyle.Render(`"`+strings.Join(r.MatchedQueries, `", "`)+`"`)))
		} else {
			topQueries := r.MatchedQueries[:3]
			moreCount := len(r.MatchedQueries) - 3
			details = append(details, fmt.Sprintf("Matched target sections (%d): %s (+%d more)", len(r.MatchedQueries), extraStyle.Render(`"`+strings.Join(topQueries, `", "`)+`"`), moreCount))
		}
	}
	if len(r.Tags) > 0 {
		formattedTags := make([]string, 0, len(r.Tags))
		for _, t := range r.Tags {
			formattedTags = append(formattedTags, "#"+t)
		}
		details = append(details, fmt.Sprintf("Tags: %s", extraStyle.Render(strings.Join(formattedTags, " "))))
	}

	maxLineLen := termWidth
	if maxLineLen <= 0 {
		maxLineLen = 140
	}
	for j, d := range details {
		prefix := "   ├─ "
		if j == len(details)-1 {
			prefix = "   └─ "
		}
		dLine := prefix + d
		if !globals.Verbose && ansi.StringWidth(dLine) > maxLineLen {
			dLine = ansi.Truncate(dLine, maxLineLen, "…")
		}
		_, _ = fmt.Fprintln(w, dLine)
	}
}

func normalizeJSONPath(jp string) string {
	jp = strings.TrimSpace(jp)
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
	return printJSONPathResultToWriter(os.Stdout, obj, jp)
}

func printJSONPathResultToWriter(w io.Writer, obj any, jp string) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("jsonpath marshal: %w", err)
	}
	var raw any
	if uerr := json.Unmarshal(data, &raw); uerr != nil {
		return fmt.Errorf("jsonpath unmarshal: %w", uerr)
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
			printSingleJSONPathValue(w, item)
		}
	default:
		printSingleJSONPathValue(w, val)
	}
	return nil
}

func printSingleJSONPathValue(w io.Writer, val any) {
	switch v := val.(type) {
	case string:
		_, _ = fmt.Fprintln(w, v)
	case float64, float32, int, int64, bool:
		_, _ = fmt.Fprintf(w, "%v\n", v)
	default:
		enc := json.NewEncoder(w)
		_ = enc.Encode(v)
	}
}
