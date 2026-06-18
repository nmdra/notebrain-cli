// Package obsidian wraps the Obsidian CLI for vault interaction.
package obsidian

// LinkRecord represents a link found in an Obsidian note.
type LinkRecord struct {
	Path        string
	DisplayText string
}
