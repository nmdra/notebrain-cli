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
	formatText = "text"
	formatJSON = "json"
	formatTSV  = "tsv"
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

// printResultsFormatted renders a list of results to stdout based on the
// requested format. queries carries the raw search queries that produced the
// results (used for per-query hit attribution and the JSON "queries" field);
// pass nil when not applicable. It returns an error (e.g. an invalid JSONPath)
// so commands can exit non-zero; stats/get already fail on JSONPath errors.
func printResultsFormatted(commandName string, headerQuery string, rawQuery string, queries []string, results []store.Result, globals *Globals, displayFlags *ChunkDisplayFlags) error {
	return printResultsFormattedToWriter(os.Stdout, commandName, headerQuery, rawQuery, queries, results, globals, displayFlags)
}

func printResultsFormattedToWriter(w io.Writer, commandName string, headerQuery string, rawQuery string, queries []string, results []store.Result, globals *Globals, displayFlags *ChunkDisplayFlags) error {
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
			Queries: queries,
			Total:   len(filtered),
			Results: filtered,
		}
		return printJSONPathResultToWriter(w, env, globals.JSONPath)
	}

	switch globals.Format {
	case formatJSON:
		printJSONResults(w, cmdName, queryStr, queries, filtered)
	case formatTSV:
		printTSVResults(w, filtered)
	default: // "text"
		printTextResults(w, commandName, headerQuery, queries, filtered, globals)
	}
	return nil
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
		if r.Score < minScore && !r.Lexical {
			continue
		}
		if globals.SkipPhantom && r.IsPhantom {
			continue
		}
		if !globals.ShowTags {
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
	if displayFlags != nil && displayFlags.GroupByNote {
		filtered = groupByNote(filtered)
	}
	return filtered
}

// groupByNote collapses chunk rows of the same note into its best-scoring
// row. When a note matched in more than one chunk, the kept row carries an
// "N matching chunks" extra so the coverage is still visible.
func groupByNote(results []store.Result) []store.Result {
	type agg struct {
		best  store.Result
		count int
	}
	bySlug := make(map[string]*agg, len(results))
	var order []string
	for _, r := range results {
		a, ok := bySlug[r.NoteSlug]
		if !ok {
			bySlug[r.NoteSlug] = &agg{best: r, count: 1}
			order = append(order, r.NoteSlug)
			continue
		}
		a.count++
		if r.Score > a.best.Score {
			a.best = r
		}
	}
	out := make([]store.Result, 0, len(order))
	for _, slug := range order {
		a := bySlug[slug]
		r := a.best
		if a.count > 1 {
			r.Extra = fmt.Sprintf("%d matching chunks", a.count)
		}
		out = append(out, r)
	}
	return out
}

func printJSONResults(w io.Writer, commandName, query string, queries []string, filtered []store.Result) {
	env := jsonEnvelope{
		Command: commandName,
		Query:   query,
		Queries: queries,
		Total:   len(filtered),
		Results: filtered,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

// tsvEscape escapes characters that would break a tab-separated record:
// tab, newline, and carriage return. Field content stays on one line so
// line-based parsers (wc -l, cut, awk -F'\t') keep working.
func tsvEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

func printTSVResults(w io.Writer, filtered []store.Result) {
	_, _ = fmt.Fprintln(w, "note_slug\ttitle\tfile_path\tscore\ttags\textra\theading_path\ttext")
	for _, r := range filtered {
		tagsStr := formatTags(r.Tags)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%.4f\t%s\t%s\t%s\t%s\n",
			tsvEscape(r.NoteSlug), tsvEscape(r.Title), tsvEscape(r.FilePath), r.Score,
			tsvEscape(tagsStr), tsvEscape(r.Extra), tsvEscape(r.HeadingPath), tsvEscape(r.Text))
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

func printTextResults(w io.Writer, commandName, query string, queries []string, filtered []store.Result, globals *Globals) {
	termWidth := getTerminalWidth()

	header := headerStyle
	if termWidth > 0 {
		header = header.Width(termWidth)
	}
	_, _ = fmt.Fprintln(w, header.Render(query))

	if len(filtered) == 0 {
		hint := emptyResultHint(commandName)
		if hint != "" {
			_, _ = fmt.Fprintln(w, hintStyle.Render("  "+hint))
		} else {
			_, _ = fmt.Fprintln(w, extraStyle.Render("  (no results)"))
		}
		return
	}

	useLinks := hyperlinkSupported() && globals.ShowFilePath

	noteCounts := make(map[string]int, len(filtered))
	for _, r := range filtered {
		noteCounts[r.NoteSlug]++
	}

	for i, r := range filtered {
		rank := rankStyle.Render(fmt.Sprintf("%d.", i+1))

		displayTitle := r.Title
		if r.FileType == store.FileTypePDF {
			displayTitle = pdfTagStyle.Render("[PDF] ") + displayTitle
		}

		if r.HeadingPath != "" {
			short := store.TrimHeadingTitlePrefix(r.Title, r.HeadingPath)
			if short != "" {
				displayTitle = fmt.Sprintf("%s (§ %s)", displayTitle, short)
			}
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
			uri := ObsidianURI(globals.VaultName, r.FilePath)
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
			printDeepDetails(w, r, termWidth)
			if i < len(filtered)-1 {
				_, _ = fmt.Fprintln(w)
			}
			continue
		}

		if len(r.Tags) > 0 {
			line += "  " + extraStyle.Render("["+strings.Join(formatTagChips(r.Tags), " ")+"]")
		}
		if len(r.MatchedQueries) > 0 && len(queries) > 1 {
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

// maxMatchedSectionsShown caps the number of section paths printed on the
// "Matched target sections" line. The total count stays in the header, so
// long section lists remain readable without wrapping or mid-path truncation.
const maxMatchedSectionsShown = 3

func printDeepDetails(w io.Writer, r store.Result, termWidth int) {
	var details []string
	if len(r.MatchedQueries) > 0 {
		shown := r.MatchedQueries
		if len(shown) > maxMatchedSectionsShown {
			shown = shown[:maxMatchedSectionsShown]
		}
		text := `"` + strings.Join(shown, `", "`) + `"`
		if len(r.MatchedQueries) > maxMatchedSectionsShown {
			text += fmt.Sprintf(`, "… (+%d more)"`, len(r.MatchedQueries)-maxMatchedSectionsShown)
		}
		details = append(details, fmt.Sprintf("Matched target sections (%d): %s", len(r.MatchedQueries), extraStyle.Render(text)))
	}
	if len(r.Tags) > 0 {
		details = append(details, fmt.Sprintf("Tags: %s", extraStyle.Render(strings.Join(formatTagChips(r.Tags), " "))))
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
		if ansi.StringWidth(dLine) > maxLineLen {
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

// printEnvelopeJSON handles the JSON and JSONPath output branches shared by
// every command envelope: JSONPath extraction first (a single selected value),
// then the indented JSON form. It reports whether the envelope was printed.
func printEnvelopeJSON(w io.Writer, env any, globals *Globals) (bool, error) {
	if globals.JSONPath != "" {
		return true, printJSONPathResultToWriter(w, env, globals.JSONPath)
	}
	if globals.Format == formatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return true, enc.Encode(env)
	}
	return false, nil
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

// tagListEnvelope is the machine-readable shape of "tags --list" output.
type tagListEnvelope struct {
	Command string           `json:"command,omitempty"`
	Total   int              `json:"total"`
	Tags    []store.TagCount `json:"tags"`
}

// printTagsFormatted renders a tag listing to stdout based on the requested
// format. JSONPath extraction applies to the tag envelope when requested.
func printTagsFormatted(commandName string, tags []store.TagCount, globals *Globals) error {
	return printTagsFormattedToWriter(os.Stdout, commandName, tags, globals)
}

func printTagsFormattedToWriter(w io.Writer, commandName string, tags []store.TagCount, globals *Globals) error {
	initStyles()
	env := tagListEnvelope{Command: commandName, Total: len(tags), Tags: tags}

	if handled, err := printEnvelopeJSON(w, env, globals); handled {
		return err
	}

	switch globals.Format {
	case formatTSV:
		_, _ = fmt.Fprintln(w, "tag\tcount")
		for _, t := range tags {
			_, _ = fmt.Fprintf(w, "%s\t%d\n", tsvEscape(t.Tag), t.Count)
		}
		return nil
	default: // "text"
		for _, t := range tags {
			_, _ = fmt.Fprintf(w, "#%s\t(%d %s)\n", t.Tag, t.Count, pluralizeNote(t.Count))
		}
		return nil
	}
}

// pluralizeNote returns "note" or "notes" for a count.
func pluralizeNote(n int) string {
	if n == 1 {
		return "note"
	}
	return "notes"
}
