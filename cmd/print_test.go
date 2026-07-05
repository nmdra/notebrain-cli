package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/internal/store"
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
