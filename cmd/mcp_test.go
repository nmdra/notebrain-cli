package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPStartupFailureKeepsStdoutClean(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{
		{"mcp", "serve", "--bad-option", "--format=json"},
		{"mcp", "serve", "--max-concurrent-requests=0", "--format=json"},
	} {
		var err error
		out := captureStdout(t, func() { err = ParseAndRun(context.Background(), "test", "none", "now", nil, args) })
		if err == nil {
			t.Fatalf("%v: expected startup failure", args)
		}
		if out != "" {
			t.Errorf("%v: protocol stdout contaminated: %q", args, out)
		}
		if _, ok := errors.AsType[*JSONEnvelopeError](err); ok {
			t.Errorf("%v: returned CLI JSON error on MCP route", args)
		}
	}
}

func TestMCPMalformedDefaultConfigKeepsStdoutClean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".notebrain", "config")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("not = [valid"), 0600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := ParseAndRun(context.Background(), "test", "none", "now", nil, []string{"mcp", "serve", "--format=json"}); err == nil {
			t.Error("expected config error")
		}
	})
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
}
