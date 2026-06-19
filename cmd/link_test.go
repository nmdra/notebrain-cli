package cmd

import (
	"testing"

	"github.com/nmdra/notebrain-cli/internal/store"
)

func TestObsidianURI(t *testing.T) {
	tests := []struct {
		vault    string
		filePath string
		expected string
	}{
		{"My Vault", "Projects/Alpha.md", "obsidian://open?vault=My%20Vault&file=Projects%2FAlpha"},
		{"", "Projects/Alpha", "obsidian://open?file=Projects%2FAlpha"},
		{"Test Vault", "Folder/Note.md", "obsidian://open?vault=Test%20Vault&file=Folder%2FNote"},
		{"Second Brain 2.0", "00.Fleeting Notes/Platform Engineering/OpenChoreo/OpenChoreo.md", "obsidian://open?vault=Second%20Brain%202.0&file=00.Fleeting%20Notes%2FPlatform%20Engineering%2FOpenChoreo%2FOpenChoreo"},
	}

	for _, tt := range tests {
		actual := store.ObsidianURI(tt.vault, tt.filePath)
		if actual != tt.expected {
			t.Errorf("ObsidianURI(%q, %q) = %q, expected %q", tt.vault, tt.filePath, actual, tt.expected)
		}
	}
}

func TestHyperlink(t *testing.T) {
	// With useLinks = false, it should just return the text
	res := hyperlink(false, "obsidian://open?file=Note", "My Note")
	if res != "My Note" {
		t.Errorf("Expected hyperlink to return plain text 'My Note', got %q", res)
	}

	res = hyperlink(true, "obsidian://open?file=Note", "My Note")
	expected := "\x1b]8;;obsidian://open?file=Note\x1b\\My Note\x1b]8;;\x1b\\"
	if res != expected {
		t.Errorf("Expected hyperlink to return %q, got %q", expected, res)
	}
}
