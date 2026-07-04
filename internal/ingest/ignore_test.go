package ingest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nmdra/notebrain-cli/internal/ingest"
)

func TestLoadExcludedPaths(t *testing.T) {
	vaultDir := t.TempDir()
	obsidianDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsidianDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	appJSONPath := filepath.Join(obsidianDir, "app.json")
	content := []byte(`{
		"userIgnoreFilters": ["Archive", "*.pdf", "**/drafts"],
		"attachmentFolderPath": "99.Storage-Shed/Attachments"
	}`)
	if err := os.WriteFile(appJSONPath, content, 0644); err != nil {
		t.Fatalf("failed to write app.json: %v", err)
	}

	filters := ingest.LoadExcludedPaths(vaultDir)
	expected := []string{"Archive", "*.pdf", "**/drafts", "99.Storage-Shed/Attachments"}

	if len(filters) != len(expected) {
		t.Fatalf("expected %d filters, got %d (%v)", len(expected), len(filters), filters)
	}
	for i, exp := range expected {
		if filters[i] != exp {
			t.Errorf("expected filter[%d] = %q, got %q", i, exp, filters[i])
		}
	}
}

func TestLoadExcludedPaths_MissingFile(t *testing.T) {
	vaultDir := t.TempDir()
	filters := ingest.LoadExcludedPaths(vaultDir)
	if filters != nil {
		t.Errorf("expected nil filters for missing app.json, got %v", filters)
	}
}

func TestIsExcluded(t *testing.T) {
	filters := []string{
		"Archive",
		"Templates/",
		"*.pdf",
		"**/drafts",
		"99.Storage-Shed/Attachments",
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"Archive", true},
		{"Archive/old_note.md", true},
		{"Archive/subfolder/note.md", true},
		{"Templates/meeting.md", true},
		{"Notes/meeting.pdf", true},
		{"drafts/work.md", true},
		{"Projects/2026/drafts/idea.md", true},
		{"99.Storage-Shed/Attachments/image.png", true},
		{"99.Storage-Shed/Attachments/sub/file.md", true},
		{"Notes/daily.md", false},
		{"Projects/active.md", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := ingest.IsExcluded(tc.path, filters)
			if got != tc.expected {
				t.Errorf("IsExcluded(%q) = %v, expected %v", tc.path, got, tc.expected)
			}
		})
	}
}

func BenchmarkIsExcluded(b *testing.B) {
	filters := []string{
		"Archive",
		"Templates/",
		"*.pdf",
		"**/drafts",
		"99.Storage-Shed/Attachments",
	}
	path := "Projects/2026/drafts/subfolder/deep/idea.md"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ingest.IsExcluded(path, filters)
	}
}
