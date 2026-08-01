package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSQLiteHealth(t *testing.T) {
	t.Run("missing database", func(t *testing.T) {
		exists, ok, detail := sqliteHealth(t.TempDir())
		if exists || ok {
			t.Errorf("missing db: got exists=%v ok=%v, want false/false", exists, ok)
		}
		if detail == "" {
			t.Error("expected a detail message")
		}
	})

	t.Run("valid sqlite file", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "chroma.sqlite3")
		content := make([]byte, 8192)
		copy(content, []byte("SQLite format 3\x00"))
		content[16], content[17] = 0x10, 0x00 // page size 4096
		if err := os.WriteFile(dbPath, content, 0600); err != nil {
			t.Fatal(err)
		}
		exists, ok, _ := sqliteHealth(dir)
		if !exists || !ok {
			t.Errorf("valid db: got exists=%v ok=%v, want true/true", exists, ok)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "chroma.sqlite3"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		exists, ok, _ := sqliteHealth(dir)
		if !exists || ok {
			t.Errorf("empty db: got exists=%v ok=%v, want true/false", exists, ok)
		}
	})

	t.Run("wrong magic", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "chroma.sqlite3"), []byte("not a database at all"), 0600); err != nil {
			t.Fatal(err)
		}
		exists, ok, _ := sqliteHealth(dir)
		if !exists || ok {
			t.Errorf("garbage db: got exists=%v ok=%v, want true/false", exists, ok)
		}
	})
}

func TestSegmentIssues(t *testing.T) {
	writeSegment := func(dir string, missing, empty []string) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, f := range chromaSegmentFiles {
			if contains(missing, f) {
				continue
			}
			if contains(empty, f) {
				if err := os.WriteFile(filepath.Join(dir, f), nil, 0600); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("intact segments", func(t *testing.T) {
		dir := t.TempDir()
		writeSegment(filepath.Join(dir, "aaaa"), nil, nil)
		writeSegment(filepath.Join(dir, "bbbb"), nil, nil)
		segments, issues := segmentIssues(dir)
		if segments != 2 {
			t.Errorf("segments = %d, want 2", segments)
		}
		if len(issues) != 0 {
			t.Errorf("issues = %v, want none", issues)
		}
	})

	t.Run("incomplete segment", func(t *testing.T) {
		dir := t.TempDir()
		writeSegment(filepath.Join(dir, "aaaa"), nil, nil)
		writeSegment(filepath.Join(dir, "bbbb"), []string{"index_metadata.pickle"}, nil)
		segments, issues := segmentIssues(dir)
		if segments != 2 {
			t.Errorf("segments = %d, want 2", segments)
		}
		if len(issues) != 1 {
			t.Fatalf("issues = %v, want 1", issues)
		}
		if !strings.Contains(issues[0], "bbbb") || !strings.Contains(issues[0], "index_metadata.pickle") {
			t.Errorf("issue %q should mention segment bbbb and the missing file", issues[0])
		}
	})

	t.Run("empty data file", func(t *testing.T) {
		dir := t.TempDir()
		writeSegment(filepath.Join(dir, "cccc"), nil, []string{"data_level0.bin"})
		segments, issues := segmentIssues(dir)
		if segments != 1 {
			t.Errorf("segments = %d, want 1", segments)
		}
		if len(issues) != 1 || !strings.Contains(issues[0], "empty") {
			t.Errorf("issues = %v, want empty-file issue", issues)
		}
	})

	t.Run("non-segment dirs ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeSegment(filepath.Join(dir, "aaaa"), nil, nil)
		if err := os.WriteFile(filepath.Join(dir, "someother.txt"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, "notasegment"), 0755); err != nil {
			t.Fatal(err)
		}
		segments, issues := segmentIssues(dir)
		if segments != 1 {
			t.Errorf("segments = %d, want 1", segments)
		}
		if len(issues) != 0 {
			t.Errorf("issues = %v, want none", issues)
		}
	})
}

func TestProbeStoreOpenExec(t *testing.T) {
	t.Run("successful probe", func(t *testing.T) {
		res := probeStoreOpenExec(t.TempDir(), "/bin/true")
		if !res.ok || res.signaled || res.detail == "" {
			t.Errorf("true probe: got %+v, want ok", res)
		}
	})

	t.Run("exit error", func(t *testing.T) {
		res := probeStoreOpenExec(t.TempDir(), "/bin/false")
		if res.ok || res.signaled {
			t.Errorf("false probe: got %+v, want non-ok non-signaled", res)
		}
		if !strings.Contains(res.detail, "exit") {
			t.Errorf("detail %q should mention exit code", res.detail)
		}
	})

	t.Run("signal abort", func(t *testing.T) {
		dir := t.TempDir()
		abortScript := filepath.Join(dir, "abort.sh")
		script := "#!/bin/sh\nkill -ABRT $$\n"
		if err := os.WriteFile(abortScript, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		res := probeStoreOpenExec(t.TempDir(), abortScript)
		if res.ok || !res.signaled {
			t.Fatalf("abort probe: got %+v, want signaled", res)
		}
		if !strings.Contains(res.detail, "aborted by signal") {
			t.Errorf("detail %q should mention the aborting signal", res.detail)
		}
	})

	t.Run("missing executable", func(t *testing.T) {
		res := probeStoreOpenExec(t.TempDir(), filepath.Join(t.TempDir(), "does-not-exist"))
		if res.ok || res.signaled {
			t.Errorf("missing exe: got %+v, want failure", res)
		}
	})
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}
