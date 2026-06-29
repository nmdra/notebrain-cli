package tui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/nmdra/notebrain-cli/internal/obsidian"
)

type editorFinishedMsg struct {
	err error
}

func openNote(vaultPath, filePath string, useEditor bool) tea.Cmd {
	if filePath == "" {
		return nil
	}
	if useEditor {
		return openEditorCmd(vaultPath, filePath)
	}
	_ = obsidian.OpenInObsidian(vaultPath, filePath)
	return nil
}

func openEditorCmd(vaultPath, filePath string) tea.Cmd {
	absPath := filepath.Join(vaultPath, filePath)
	cmd := obsidian.EditorCommand(absPath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}
