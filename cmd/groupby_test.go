package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestGroupByNote(t *testing.T) {
	results := []store.Result{
		{NoteSlug: "note-a", Title: "Note A", Score: 0.5, HeadingPath: "Intro"},
		{NoteSlug: "note-a", Title: "Note A", Score: 0.8, HeadingPath: "Deep Dive"},
		{NoteSlug: "note-b", Title: "Note B", Score: 0.6},
	}

	grouped := groupByNote(results)
	if len(grouped) != 2 {
		t.Fatalf("groupByNote = %d rows, want 2", len(grouped))
	}
	if grouped[0].NoteSlug != "note-a" || grouped[0].Score != 0.8 {
		t.Errorf("grouped row = %+v, want note-a with best score 0.8", grouped[0])
	}
	if grouped[0].Extra != "2 matching chunks" {
		t.Errorf("grouped row extra = %q, want %q", grouped[0].Extra, "2 matching chunks")
	}
	if grouped[1].NoteSlug != "note-b" || grouped[1].Extra != "" {
		t.Errorf("single-chunk note must stay untouched, got %+v", grouped[1])
	}
}

func TestGroupByNoteSingleRowUnchanged(t *testing.T) {
	results := []store.Result{
		{NoteSlug: "note-a", Title: "Note A", Score: 0.8},
		{NoteSlug: "note-b", Title: "Note B", Score: 0.6},
	}
	grouped := groupByNote(results)
	if len(grouped) != 2 || grouped[0].Extra != "" || grouped[1].Extra != "" {
		t.Errorf("groupByNote must not annotate single-row notes: %+v", grouped)
	}
}

func TestPrintGroupByNoteJSON(t *testing.T) {
	results := []store.Result{
		{NoteSlug: "note-a", Title: "Note A", Score: 0.5},
		{NoteSlug: "note-a", Title: "Note A", Score: 0.8},
		{NoteSlug: "note-b", Title: "Note B", Score: 0.6},
	}

	var buf bytes.Buffer
	if err := printResultsFormattedToWriter(&buf, "search", "q", "q", nil, results, &Globals{Format: formatJSON}, &ChunkDisplayFlags{GroupByNote: true}); err != nil {
		t.Fatalf("print: %v", err)
	}

	var env struct {
		Total   int `json:"total"`
		Results []struct {
			NoteSlug string `json:"note_slug"`
			Extra    string `json:"extra"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	if env.Total != 2 {
		t.Errorf("grouped total = %d, want 2", env.Total)
	}
	if env.Results[0].Extra != "2 matching chunks" {
		t.Errorf("grouped extra = %q, want %q", env.Results[0].Extra, "2 matching chunks")
	}
}

func TestPrintGroupByNoteOffByDefault(t *testing.T) {
	results := []store.Result{
		{NoteSlug: "note-a", Title: "Note A", Score: 0.5},
		{NoteSlug: "note-a", Title: "Note A", Score: 0.8},
	}

	var buf bytes.Buffer
	if err := printResultsFormattedToWriter(&buf, "search", "q", "q", nil, results, &Globals{Format: formatTSV}, &ChunkDisplayFlags{}); err != nil {
		t.Fatalf("print: %v", err)
	}
	if strings.Count(buf.String(), "note-a") != 2 {
		t.Errorf("default output must keep all chunk rows, got:\n%s", buf.String())
	}
}
