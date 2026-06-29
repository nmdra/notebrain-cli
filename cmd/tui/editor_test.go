package tui

import (
	"testing"
)

func TestOpenNote(t *testing.T) {
	t.Run("empty path returns nil", func(t *testing.T) {
		cmd := openNote("vault", "", false)
		if cmd != nil {
			t.Errorf("expected nil cmd for empty path, got %v", cmd)
		}
	})

	t.Run("useEditor returns non-nil command", func(t *testing.T) {
		t.Setenv("EDITOR", "echo")
		cmd := openNote("/vault", "note.md", true)
		if cmd == nil {
			t.Errorf("expected non-nil tea.Cmd when useEditor is true")
		}
	})
}
