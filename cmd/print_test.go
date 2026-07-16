package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestPrintResultsFormatted_Phantom(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{NoteSlug: "real-note", Title: "Real Note", Score: 1.0, IsPhantom: false},
		{NoteSlug: "phantom-note", Title: "Phantom Note", Score: 0.8, IsPhantom: true},
	}
	globals := &Globals{
		Format:      "text",
		SkipPhantom: true,
	}

	printResultsFormattedToWriter(&buf, "test", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "Real Note") {
		t.Errorf("Expected Real Note in output when SkipPhantom=true, got %q", out)
	}
	if strings.Contains(out, "Phantom Note") {
		t.Errorf("Did not expect Phantom Note in output when SkipPhantom=true, got %q", out)
	}

	// Now test SkipPhantom = false
	buf.Reset()
	globals.SkipPhantom = false
	printResultsFormattedToWriter(&buf, "test", "query", "query", results, globals)
	out2 := buf.String()

	if !strings.Contains(out2, "Real Note") || !strings.Contains(out2, "Phantom Note") {
		t.Errorf("Expected both notes when SkipPhantom=false, got %q", out2)
	}
	if !strings.Contains(out2, "[phantom]") {
		t.Errorf("Expected [phantom] marker when SkipPhantom=false, got %q", out2)
	}
}

func TestPrintResultsFormatted_ChunkDisambiguationAndFooter(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{NoteSlug: "openchoreo", Title: "OpenChoreo", Score: 0.4568, HeadingPath: "Architecture > Overview"},
		{NoteSlug: "openchoreo", Title: "OpenChoreo", Score: 0.4250, ChunkIndex: 1},
	}
	globals := &Globals{
		Format: "text",
	}

	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "OpenChoreo (§ Architecture > Overview)") {
		t.Errorf("Expected heading path disambiguation in output, got %q", out)
	}
	if !strings.Contains(out, "OpenChoreo (chunk #2)") {
		t.Errorf("Expected chunk index disambiguation in output, got %q", out)
	}
	if !strings.Contains(out, "Note: Results are matching text chunks;") {
		t.Errorf("Expected footer description note in output, got %q", out)
	}
}

func TestPrintResultsFormatted_Truncation(t *testing.T) {
	oldWidthFunc := getTerminalWidth
	getTerminalWidth = func() int { return 50 }
	defer func() { getTerminalWidth = oldWidthFunc }()

	var buf bytes.Buffer

	results := []store.Result{
		{
			NoteSlug: "long-note",
			Title:    "Very Long Title That Is Extremely Long And Exceeds Fifty Characters Easily",
			Score:    0.9999,
			Tags:     []string{"tag1", "tag2", "tag3", "tag4", "tag5"},
		},
	}
	globals := &Globals{
		Format: "text",
	}

	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "…") {
		t.Errorf("Expected ellipsis truncation marker in output, got %q", out)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "1.") {
			width := ansi.StringWidth(line)
			if width > 50 {
				t.Errorf("Expected line width <= 50, got %d for line %q", width, line)
			}
		}
	}
}

func TestPrintResultsFormatted_MultiQuery(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{
			NoteSlug:       "openchoreo",
			Title:          "OpenChoreo",
			Score:          0.8540,
			MatchedQueries: []string{"redis", "broker"},
		},
	}
	globals := &Globals{
		Format:  "text",
		Queries: []string{"redis", "broker"},
	}

	printResultsFormattedToWriter(&buf, "search", "redis | broker", "redis | broker", results, globals)
	out := buf.String()

	if !strings.Contains(out, `[hits: "redis", "broker"]`) {
		t.Errorf("Expected matched queries attribution in output, got %q", out)
	}
}

func TestPrintResultsFormatted_HideTags(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{NoteSlug: "redis-note", Title: "Redis Note", Score: 0.9, Tags: []string{"Database/Redis", "Backend"}},
	}
	globals := &Globals{
		Format:   "text",
		HideTags: true,
	}

	printResultsFormattedToWriter(&buf, "test", "query", "query", results, globals)
	out := buf.String()

	if strings.Contains(out, "#Database/Redis") || strings.Contains(out, "#Backend") {
		t.Errorf("Did not expect tags in output when HideTags=true, got %q", out)
	}
	if !strings.Contains(out, "Redis Note") {
		t.Errorf("Expected note title in output, got %q", out)
	}

	// Now test HideTags = false
	buf.Reset()
	globals.HideTags = false
	printResultsFormattedToWriter(&buf, "test", "query", "query", results, globals)
	out2 := buf.String()

	if !strings.Contains(out2, "#Database/Redis") || !strings.Contains(out2, "#Backend") {
		t.Errorf("Expected tags in output when HideTags=false, got %q", out2)
	}
}

func TestPrintResultsFormatted_Deep(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{
			NoteSlug:       "memory-safety",
			Title:          "Memory Safety",
			Score:          0.8812,
			HeadingPath:    "Borrow Checker",
			Tags:           []string{"rust", "memory"},
			MatchedQueries: []string{"§ Ownership", "§ Lifetimes"},
			Text:           "In safe Rust every reference must obey borrowing rules",
		},
	}
	globals := &Globals{
		Format:      "text",
		IncludeText: true,
	}

	printResultsFormattedToWriter(&buf, "hidden --deep", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "├─ Matched target sections (2):") || !strings.Contains(out, "\"§ Ownership\", \"§ Lifetimes\"") {
		t.Errorf("Expected tree branch with matched sections, got %q", out)
	}
	if !strings.Contains(out, "└─ Tags:") || !strings.Contains(out, "#rust #memory") {
		t.Errorf("Expected tree branch with tags, got %q", out)
	}
	if strings.Contains(out, "Text:") {
		t.Errorf("Did not expect chunk text snippet in --deep output, got %q", out)
	}
}

func TestPrintResultsFormatted_Deep_SmartCapping(t *testing.T) {
	var buf bytes.Buffer

	results := []store.Result{
		{
			NoteSlug:       "perf-book",
			Title:          "System Performance Book",
			Score:          0.9162,
			MatchedQueries: []string{"§ Methodologies", "§ Latency", "§ Queueing", "§ Profiling", "§ Caching"},
		},
	}
	globals := &Globals{
		Format:  "text",
		Verbose: false,
	}

	printResultsFormattedToWriter(&buf, "hidden --deep", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "└─ Matched target sections (5):") || !strings.Contains(out, "\"§ Methodologies\", \"§ Latency\", \"§ Queueing\"") || !strings.Contains(out, "(+2 more)") {
		t.Errorf("Expected capped top-3 queries with (+2 more), got %q", out)
	}

	// Now verify --verbose shows all 5
	buf.Reset()
	globals.Verbose = true
	printResultsFormattedToWriter(&buf, "hidden --deep", "query", "query", results, globals)
	out2 := buf.String()

	if !strings.Contains(out2, "└─ Matched target sections (5):") || !strings.Contains(out2, "\"§ Methodologies\", \"§ Latency\", \"§ Queueing\", \"§ Profiling\", \"§ Caching\"") {
		t.Errorf("Expected all 5 queries when Verbose=true, got %q", out2)
	}
}

func TestPrintJSONPathResult(t *testing.T) {
	var buf bytes.Buffer

	obj := map[string]any{
		"title": "My Note",
		"count": 42,
		"valid": true,
		"items": []string{"a", "b"},
		"nested": map[string]any{
			"key": "value",
		},
	}

	if err := printJSONPathResultToWriter(&buf, obj, "{.title}"); err != nil {
		t.Errorf("printJSONPathResult {.title} failed: %v", err)
	}
	if err := printJSONPathResultToWriter(&buf, obj, "$.count"); err != nil {
		t.Errorf("printJSONPathResult $.count failed: %v", err)
	}
	if err := printJSONPathResultToWriter(&buf, obj, "valid"); err != nil {
		t.Errorf("printJSONPathResult valid failed: %v", err)
	}
	if err := printJSONPathResultToWriter(&buf, obj, "items"); err != nil {
		t.Errorf("printJSONPathResult items failed: %v", err)
	}
	if err := printJSONPathResultToWriter(&buf, obj, "nested"); err != nil {
		t.Errorf("printJSONPathResult nested failed: %v", err)
	}
	_ = printJSONPathResultToWriter(&buf, obj, "$.nonexistent") // Verify non-existent key handling does not panic
	if err := printJSONPathResultToWriter(&buf, obj, "invalid[syntax[["); err == nil {
		t.Errorf("Expected error for invalid jsonpath syntax, got nil")
	}

	out := buf.String()

	if !strings.Contains(out, "My Note") || !strings.Contains(out, "42") || !strings.Contains(out, "true") || !strings.Contains(out, "a\nb\n") || !strings.Contains(out, `"key":"value"`) {
		t.Errorf("Expected jsonpath outputs in buffer, got %q", out)
	}
}

func TestPrintResultsFormatted_Formats(t *testing.T) {
	var buf bytes.Buffer
	results := []store.Result{
		{
			NoteSlug: "json-note",
			Title:    "JSON Note",
			Score:    0.95,
			FilePath: "notes/json.md",
			Text:     "sample text",
			Tags:     []string{"tag1", "tag2"},
		},
	}

	// Test JSON format
	globals := &Globals{Format: "json"}
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	if !strings.Contains(buf.String(), `"note_slug": "json-note"`) && !strings.Contains(buf.String(), `"note_slug":"json-note"`) {
		t.Errorf("Expected json output containing note_slug, got %q", buf.String())
	}

	// Test TSV format
	buf.Reset()
	globals.Format = "tsv"
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	outTSV := buf.String()
	if !strings.Contains(outTSV, "slug\ttitle\tfile_path") || !strings.Contains(outTSV, "json-note\tJSON Note") {
		t.Errorf("Expected tsv header and row, got %q", outTSV)
	}

	// Test NDJSON format
	buf.Reset()
	globals.Format = "ndjson"
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	outNDJSON := buf.String()
	if !strings.Contains(outNDJSON, `"note_slug":"json-note"`) && !strings.Contains(outNDJSON, `"note_slug": "json-note"`) {
		t.Errorf("Expected ndjson output containing note_slug, got %q", outNDJSON)
	}
}

func TestPrintResultsFormatted_MinScore(t *testing.T) {
	var buf bytes.Buffer
	results := []store.Result{
		{NoteSlug: "high-score", Title: "High Score Note", Score: 0.9},
		{NoteSlug: "low-score", Title: "Low Score Note", Score: 0.3},
	}

	globals := &Globals{
		Format:   "text",
		MinScore: 0.5,
	}

	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	out := buf.String()

	if !strings.Contains(out, "High Score Note") {
		t.Errorf("Expected high score note in output when MinScore=0.5, got %q", out)
	}
	if strings.Contains(out, "Low Score Note") {
		t.Errorf("Did not expect low score note in output when MinScore=0.5, got %q", out)
	}
}

func TestPrintResultsFormatted_ScoreRounding(t *testing.T) {
	var buf bytes.Buffer
	results := []store.Result{
		{NoteSlug: "test-note", Title: "Test Note", Score: 0.7655627727508545},
	}
	globals := &Globals{Format: "json"}
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	out := buf.String()
	if !strings.Contains(out, "0.7656") {
		t.Errorf("Expected rounded score 0.7656 in json output, got %q", out)
	}
	if strings.Contains(out, "0.765562") {
		t.Errorf("Expected score not to have long decimals in json output, got %q", out)
	}
}

func TestPrintResultsFormatted_RawQuery(t *testing.T) {
	var buf bytes.Buffer
	results := []store.Result{
		{NoteSlug: "test-note", Title: "Test Note", Score: 0.9},
	}
	globals := &Globals{Format: "json"}
	printResultsFormattedToWriter(&buf, "search", "Semantic Search: \"my query\"", "my query", results, globals)
	out := buf.String()
	if !strings.Contains(out, `"query": "my query"`) {
		t.Errorf("Expected raw query in json output, got %q", out)
	}
	if strings.Contains(out, "Semantic Search:") {
		t.Errorf("Did not expect decorated header in json query field, got %q", out)
	}

	// But in text format, decorated header should be used
	buf.Reset()
	globals.Format = "text"
	printResultsFormattedToWriter(&buf, "search", "Semantic Search: \"my query\"", "my query", results, globals)
	outText := buf.String()
	if !strings.Contains(outText, "Semantic Search:") {
		t.Errorf("Expected decorated header in text output, got %q", outText)
	}
}

func TestPrintResultsFormatted_Compact(t *testing.T) {
	var buf bytes.Buffer
	results := []store.Result{
		{NoteSlug: "test-note", Title: "Test Note", FilePath: "notes/test.md", Score: 0.9},
	}
	globals := &Globals{Format: "json", Compact: true}
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	out := buf.String()
	if strings.Contains(out, `"command":`) {
		t.Errorf("Did not expect command field when Compact=true, got %q", out)
	}
	if strings.Contains(out, `"file_path":`) {
		t.Errorf("Did not expect file_path field when Compact=true, got %q", out)
	}
	if !strings.Contains(out, `"note_slug": "test-note"`) {
		t.Errorf("Expected note_slug field when Compact=true, got %q", out)
	}

	// Verify Compact=false includes both
	buf.Reset()
	globals.Compact = false
	printResultsFormattedToWriter(&buf, "search", "query", "query", results, globals)
	outFull := buf.String()
	if !strings.Contains(outFull, `"command": "search"`) {
		t.Errorf("Expected command field when Compact=false, got %q", outFull)
	}
	if !strings.Contains(outFull, `"file_path": "notes/test.md"`) {
		t.Errorf("Expected file_path field when Compact=false, got %q", outFull)
	}
}

func TestFilterResults_IncludeText(t *testing.T) {
	results := []store.Result{
		{NoteSlug: "test-note", Title: "Test Note", Text: "some long chunk text", Score: 0.9},
	}

	// Test IncludeText = false clears Text
	globals := &Globals{IncludeText: false}
	filtered := filterResults(results, globals)
	if len(filtered) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(filtered))
	}
	if filtered[0].Text != "" {
		t.Errorf("Expected Text to be empty when IncludeText=false, got %q", filtered[0].Text)
	}

	// Test IncludeText = true preserves Text
	globalsTrue := &Globals{IncludeText: true}
	filteredTrue := filterResults(results, globalsTrue)
	if len(filteredTrue) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(filteredTrue))
	}
	if filteredTrue[0].Text != "some long chunk text" {
		t.Errorf("Expected Text to be preserved when IncludeText=true, got %q", filteredTrue[0].Text)
	}
}
