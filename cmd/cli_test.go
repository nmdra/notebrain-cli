package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestResolveLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		flagLevel string
		env       string
		want      slog.Level
	}{
		{"flag wins over env", "warn", "debug", slog.LevelWarn},
		{"env used when flag empty", "", "debug", slog.LevelDebug},
		{"default info", "", "", slog.LevelInfo},
		{"unknown env falls back to info", "", "verbose", slog.LevelInfo},
		{"unknown flag falls back to info", "verbose", "debug", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NOTEBRAIN_LOG_LEVEL", tt.env)
			if got := resolveLogLevel(tt.flagLevel); got != tt.want {
				t.Errorf("resolveLogLevel(%q) = %v, want %v", tt.flagLevel, got, tt.want)
			}
		})
	}
}

func TestValidateLogLevel(t *testing.T) {
	for _, valid := range []string{"", "debug", "info", "warn", "error", "INFO", "Warn"} {
		if err := validateLogLevel(valid); err != nil {
			t.Errorf("validateLogLevel(%q) = %v, want nil", valid, err)
		}
	}
	for _, invalid := range []string{"verbose", "1", " warn"} {
		if err := validateLogLevel(invalid); err == nil {
			t.Errorf("validateLogLevel(%q) = nil, want error", invalid)
		}
	}
}

func TestArgsWantJSON(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"--format", "json", "search", "rust"}, true},
		{[]string{"--format=json"}, true},
		{[]string{"--format", "json"}, true},
		{[]string{"--format", "text", "--bogus"}, false},
		{[]string{"search", "rust", "--limit", "5"}, false},
		{[]string{"--bogus"}, false},
		{[]string{}, false},
	}
	for _, tt := range tests {
		if got := argsWantJSON(tt.args); got != tt.want {
			t.Errorf("argsWantJSON(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestBuildHandler(t *testing.T) {
	t.Run("colored text", func(t *testing.T) {
		var buf bytes.Buffer
		h := buildHandler(slog.LevelInfo, &buf, true, true)
		slog.New(h).Info("hello", "k", "v")
		out := buf.String()
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("expected ANSI codes in colored output, got: %q", out)
		}
		if !strings.Contains(out, "hello") {
			t.Errorf("expected message in output, got: %q", out)
		}
	})

	t.Run("plain text", func(t *testing.T) {
		var buf bytes.Buffer
		h := buildHandler(slog.LevelInfo, &buf, true, false)
		slog.New(h).Info("hello")
		if strings.Contains(buf.String(), "\x1b[") {
			t.Errorf("unexpected ANSI codes in plain output: %q", buf.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		h := buildHandler(slog.LevelInfo, &buf, false, false)
		slog.New(h).Info("hello", "k", "v")
		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("expected JSON output, got %q: %v", buf.String(), err)
		}
		if m["msg"] != "hello" || m["k"] != "v" {
			t.Errorf("unexpected JSON fields: %v", m)
		}
	})
}

func TestParseAndRunUsageError(t *testing.T) {
	err := parseAndRun(context.Background(), "v", "c", "d", nil, []string{"--bogus-flag"})
	var uerr *UsageError
	if !errors.As(err, &uerr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestParseAndRunJSONUsageError(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	parseErr := parseAndRun(context.Background(), "v", "c", "d", nil, []string{"--format", "json", "--bogus-flag"})

	os.Stdout = old
	_ = w.Close()
	out, _ := io.ReadAll(r)

	var uerr *UsageError
	if !errors.As(parseErr, &uerr) {
		t.Fatalf("expected UsageError, got %T: %v", parseErr, parseErr)
	}
	if !strings.Contains(string(out), `"error"`) {
		t.Errorf("expected JSON error on stdout, got: %s", out)
	}
}
