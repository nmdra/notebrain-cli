package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestPrintResultsFormatted_Phantom(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []store.Result{
		{NoteSlug: "real-note", Title: "Real Note", Score: 1.0, IsPhantom: false},
		{NoteSlug: "phantom-note", Title: "Phantom Note", Score: 0.8, IsPhantom: true},
	}
	globals := &Globals{
		Format:      "text",
		SkipPhantom: true,
	}

	printResultsFormatted("test", "query", results, globals)
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Real Note") {
		t.Errorf("Expected Real Note in output when SkipPhantom=true, got %q", out)
	}
	if strings.Contains(out, "Phantom Note") {
		t.Errorf("Did not expect Phantom Note in output when SkipPhantom=true, got %q", out)
	}

	// Now test SkipPhantom = false
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	globals.SkipPhantom = false
	printResultsFormatted("test", "query", results, globals)
	_ = w2.Close()
	os.Stdout = oldStdout

	buf.Reset()
	_, _ = buf.ReadFrom(r2)
	out2 := buf.String()

	if !strings.Contains(out2, "Real Note") || !strings.Contains(out2, "Phantom Note") {
		t.Errorf("Expected both notes when SkipPhantom=false, got %q", out2)
	}
	if !strings.Contains(out2, "[phantom]") {
		t.Errorf("Expected [phantom] marker when SkipPhantom=false, got %q", out2)
	}
}

func TestPrintResultsFormatted_ChunkDisambiguationAndFooter(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []store.Result{
		{NoteSlug: "openchoreo", Title: "OpenChoreo", Score: 0.4568, HeadingPath: "Architecture > Overview"},
		{NoteSlug: "openchoreo", Title: "OpenChoreo", Score: 0.4250, ChunkIndex: 1},
	}
	globals := &Globals{
		Format: "text",
	}

	printResultsFormatted("search", "query", results, globals)
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
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

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

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

	printResultsFormatted("search", "query", results, globals)
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
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
