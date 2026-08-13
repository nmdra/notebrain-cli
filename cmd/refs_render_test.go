package cmd

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestPrintRefsFormatted_Rendering(t *testing.T) {
	env := refsEnvelope{
		Command:  groupRefs,
		NoteSlug: "router",
		Title:    "Router",
		Total:    3,
		Refs: []refEntry{
			{Path: "/vault/Notes/cover.png", RelativePath: "Notes/cover.png", Kind: kindImage},
			{Path: "/vault/Attachments/att.pdf", RelativePath: "Attachments/att.pdf", Kind: kindPDF, Missing: true},
			{Path: "https://example.com/docs", Kind: kindExternal},
		},
	}
	globals := &Globals{Format: formatText}

	t.Run("plain", func(t *testing.T) {
		stdoutColorEnabled = stdoutAllowsColor
		stylesOnce = sync.Once{}

		var buf bytes.Buffer
		if err := printRefsFormattedToWriter(&buf, env, globals); err != nil {
			t.Fatalf("printRefsFormattedToWriter: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "\x1b[") {
			t.Errorf("unexpected ANSI codes in plain output: %q", out)
		}
		if !strings.Contains(out, "[image] Notes/cover.png") {
			t.Errorf("expected image row, got: %q", out)
		}
		if !strings.Contains(out, "[pdf] Attachments/att.pdf (missing)") {
			t.Errorf("expected missing pdf row, got: %q", out)
		}
		if !strings.Contains(out, "[external-links] https://example.com/docs") {
			t.Errorf("expected external link row, got: %q", out)
		}
		if !strings.Contains(out, "Router") {
			t.Errorf("expected note title header, got: %q", out)
		}
	})

	t.Run("colored", func(t *testing.T) {
		old := stdoutColorEnabled
		stdoutColorEnabled = func() bool { return true }
		stylesOnce = sync.Once{}
		defer func() {
			stdoutColorEnabled = old
			stylesOnce = sync.Once{}
		}()

		var buf bytes.Buffer
		if err := printRefsFormattedToWriter(&buf, env, globals); err != nil {
			t.Fatalf("printRefsFormattedToWriter: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("expected ANSI codes in colored output: %q", out)
		}
		for _, want := range []string{"[image]", "[pdf]", "[external-links]", "(missing)", "Router"} {
			if !strings.Contains(out, want) {
				t.Errorf("colored output missing %q: %q", want, out)
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		stdoutColorEnabled = stdoutAllowsColor
		stylesOnce = sync.Once{}

		var buf bytes.Buffer
		empty := env
		empty.Refs = nil
		empty.Total = 0
		if err := printRefsFormattedToWriter(&buf, empty, globals); err != nil {
			t.Fatalf("printRefsFormattedToWriter: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "No references found") {
			t.Errorf("expected empty hint, got: %q", out)
		}
	})
}
