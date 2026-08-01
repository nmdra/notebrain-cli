package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newCollectFilesVault builds a vault tree and returns its root:
//
//	<root>/a.md
//	<root>/sub/b.md
//	<root>/sub/deep/c.md
//	<root>/sub/x.pdf
func newCollectFilesVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.md"), "# A")
	writeTestFile(t, filepath.Join(root, "sub", "b.md"), "# B")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "c.md"), "# C")
	writeTestFile(t, filepath.Join(root, "sub", "x.pdf"), "pdf")
	return root
}

func relPaths(t *testing.T, root string, files []string) []string {
	t.Helper()
	out := make([]string, len(files))
	for i, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

func TestCollectFiles_RecursiveGlob(t *testing.T) {
	root := newCollectFilesVault(t)
	p := &Pipeline{RespectExclude: false}

	files, err := p.collectFiles(root, "**/*.md", false)
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	got := relPaths(t, root, files)
	want := []string{"a.md", "sub/b.md", "sub/deep/c.md"}
	if !slicesEqual(got, want) {
		t.Errorf("**/*.md matched %v, want %v", got, want)
	}
}

func TestCollectFiles_SubtreeGlob(t *testing.T) {
	root := newCollectFilesVault(t)
	p := &Pipeline{RespectExclude: false}

	files, err := p.collectFiles(root, "sub/**", false)
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	got := relPaths(t, root, files)
	want := []string{"sub/b.md", "sub/deep/c.md"}
	if !slicesEqual(got, want) {
		t.Errorf("sub/** matched %v, want %v", got, want)
	}
}

func TestCollectFiles_GlobWithPDFs(t *testing.T) {
	root := newCollectFilesVault(t)
	p := &Pipeline{RespectExclude: false, EnablePDF: true}

	files, err := p.collectFiles(root, "**/*.pdf", false)
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	got := relPaths(t, root, files)
	want := []string{"sub/x.pdf"}
	if !slicesEqual(got, want) {
		t.Errorf("**/*.pdf matched %v, want %v", got, want)
	}
}

func TestCollectFiles_SingleLevelGlobStillMatches(t *testing.T) {
	root := newCollectFilesVault(t)
	p := &Pipeline{RespectExclude: false}

	files, err := p.collectFiles(root, "*.md", false)
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	got := relPaths(t, root, files)
	want := []string{"a.md"}
	if !slicesEqual(got, want) {
		t.Errorf("*.md matched %v, want %v", got, want)
	}
}

func TestCollectFiles_InvalidGlobReturnsError(t *testing.T) {
	root := newCollectFilesVault(t)
	p := &Pipeline{RespectExclude: false}

	if _, err := p.collectFiles(root, "[", false); err == nil {
		t.Fatal("expected error for invalid glob, got nil")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
