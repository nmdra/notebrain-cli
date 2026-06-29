package obsidian_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nmdra/notebrain-cli/internal/obsidian"
)

func TestEditorCommand(t *testing.T) {
	tests := []struct {
		name         string
		editorEnv    string
		filePath     string
		expectedBin  string
		expectedArgs []string
	}{
		{
			name:         "default editor when env empty",
			editorEnv:    "",
			filePath:     "/vault/note.md",
			expectedBin:  "vim",
			expectedArgs: []string{"vim", "/vault/note.md"},
		},
		{
			name:         "custom terminal editor with args",
			editorEnv:    "emacsclient -t",
			filePath:     "/vault/note.md",
			expectedBin:  "emacsclient",
			expectedArgs: []string{"emacsclient", "-t", "/vault/note.md"},
		},
		{
			name:         "vscode auto wait",
			editorEnv:    "code",
			filePath:     "/vault/note.md",
			expectedBin:  "code",
			expectedArgs: []string{"code", "--wait", "/vault/note.md"},
		},
		{
			name:         "vscode with existing wait",
			editorEnv:    "code --wait",
			filePath:     "/vault/note.md",
			expectedBin:  "code",
			expectedArgs: []string{"code", "--wait", "/vault/note.md"},
		},
		{
			name:         "sublime auto wait",
			editorEnv:    "/usr/local/bin/subl",
			filePath:     "note.md",
			expectedBin:  "/usr/local/bin/subl",
			expectedArgs: []string{"/usr/local/bin/subl", "--wait", "note.md"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EDITOR", tc.editorEnv)
			cmd := obsidian.EditorCommand(tc.filePath)
			if cmd.Path != tc.expectedBin && filepath.Base(cmd.Path) != filepath.Base(tc.expectedBin) {
				t.Errorf("expected bin %q, got %q", tc.expectedBin, cmd.Path)
			}
			if !reflect.DeepEqual(cmd.Args, tc.expectedArgs) {
				t.Errorf("expected args %v, got %v", tc.expectedArgs, cmd.Args)
			}
		})
	}
}
