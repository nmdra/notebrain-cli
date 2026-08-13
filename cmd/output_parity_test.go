package cmd

import (
	"bytes"
	"strings"
	"testing"
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
