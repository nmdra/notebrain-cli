package cmd

import (
	"os"
	"testing"
)

func TestObsidianURI(t *testing.T) {
	tests := []struct {
		vault    string
		filePath string
		expected string
	}{
		{"My Vault", "Projects/Alpha.md", "obsidian://open?file=Projects%2FAlpha&vault=My+Vault"},
		{"", "Projects/Alpha", "obsidian://open?file=Projects%2FAlpha"},
		{"Test Vault", "Folder/Note.md", "obsidian://open?file=Folder%2FNote&vault=Test+Vault"},
	}

	for _, tt := range tests {
		actual := obsidianURI(tt.vault, tt.filePath)
		// url.Values.Encode() sorts keys, so 'file' is before 'vault'
		if actual != tt.expected {
			t.Errorf("obsidianURI(%q, %q) = %q, expected %q", tt.vault, tt.filePath, actual, tt.expected)
		}
	}
}

func TestHyperlink(t *testing.T) {
	// Temporarily set an environment variable to force no hyperlinks
	originalNoHyperlinks := noHyperlinks
	noHyperlinks = true
	defer func() { noHyperlinks = originalNoHyperlinks }()

	// With noHyperlinks = true, it should just return the text
	res := hyperlink("obsidian://open?file=Note", "My Note")
	if res != "My Note" {
		t.Errorf("Expected hyperlink to return plain text 'My Note', got %q", res)
	}

	noHyperlinks = false
	// Force hyperlink supported by setting TERM_PROGRAM
	_ = os.Setenv("TERM_PROGRAM", "iTerm.app")
	defer func() { _ = os.Unsetenv("TERM_PROGRAM") }()

	res = hyperlink("obsidian://open?file=Note", "My Note")
	expected := "\x1b]8;;obsidian://open?file=Note\x07My Note\x1b]8;;\x07"
	if res != expected {
		t.Errorf("Expected hyperlink to return %q, got %q", expected, res)
	}
}
