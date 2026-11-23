package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNoteCompletionTimeout(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "suggest")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if got := predictNoteSlugsExec(exe); len(got) != 0 {
		t.Fatalf("timeout yielded candidates: %v", got)
	}
	if elapsed := time.Since(start); elapsed < time.Second || elapsed > 3*time.Second {
		t.Fatalf("completion took %s, want two-second deadline", elapsed)
	}
}
