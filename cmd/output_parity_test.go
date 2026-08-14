package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestWarnIgnoredOutputFlags(t *testing.T) {
	t.Run("silent when text and no jsonpath", func(t *testing.T) {
		var buf bytes.Buffer
		warnIgnoredOutputFlags(&buf, formatText, "", "reset")
		if buf.Len() != 0 {
			t.Errorf("expected no warning, got %q", buf.String())
		}
	})

	t.Run("warns once for format", func(t *testing.T) {
		var buf bytes.Buffer
		warnIgnoredOutputFlags(&buf, formatJSON, "", "reset")
		out := buf.String()
		if !strings.Contains(out, "--format") || !strings.Contains(out, "reset") {
			t.Errorf("expected --format warning for reset, got %q", out)
		}
		if strings.Contains(out, "--jsonpath") {
			t.Errorf("must not mention --jsonpath, got %q", out)
		}
	})

	t.Run("warns once for jsonpath", func(t *testing.T) {
		var buf bytes.Buffer
		warnIgnoredOutputFlags(&buf, formatText, "$.x", "ingest")
		out := buf.String()
		if !strings.Contains(out, "--jsonpath") || !strings.Contains(out, "ingest") {
			t.Errorf("expected --jsonpath warning for ingest, got %q", out)
		}
		if strings.Contains(out, "--format") {
			t.Errorf("must not mention --format, got %q", out)
		}
	})

	t.Run("warns for both", func(t *testing.T) {
		var buf bytes.Buffer
		warnIgnoredOutputFlags(&buf, formatTSV, "$.x", "doctor")
		out := buf.String()
		if !strings.Contains(out, "--format") || !strings.Contains(out, "--jsonpath") {
			t.Errorf("expected both warnings, got %q", out)
		}
	})
}

func TestGetStatsJSONPathEnvelope(t *testing.T) {
	withFakeStore(t, &fakeStore{
		noteMeta: &store.NoteContent{
			NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md",
		},
		stats: &store.Stats{Notes: 12, Chunks: 340, Links: 55},
	})

	t.Run("get $.command", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := (&GetCmd{Note: "router", Meta: true}).Run(&Globals{
				Ctx: context.Background(), JSONPath: "$.command",
			}); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
		if strings.TrimSpace(out) != "get" {
			t.Errorf("$.command = %q, want %q", out, "get")
		}
	})

	t.Run("get $.note.note_slug", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := (&GetCmd{Note: "router", Meta: true}).Run(&Globals{
				Ctx: context.Background(), JSONPath: "$.note.note_slug",
			}); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
		if strings.TrimSpace(out) != "router" {
			t.Errorf("$.note.note_slug = %q, want %q", out, "router")
		}
	})

	t.Run("stats $.command", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := (&StatsCmd{}).Run(&Globals{
				Ctx: context.Background(), JSONPath: "$.command",
			}); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
		if strings.TrimSpace(out) != "stats" {
			t.Errorf("$.command = %q, want %q", out, "stats")
		}
	})

	t.Run("stats $.notes", func(t *testing.T) {
		out := captureStdout(t, func() {
			if err := (&StatsCmd{}).Run(&Globals{
				Ctx: context.Background(), JSONPath: "$.notes",
			}); err != nil {
				t.Errorf("Run: %v", err)
			}
		})
		if strings.TrimSpace(out) != "12" {
			t.Errorf("$.notes = %q, want %q", out, "12")
		}
	})
}
