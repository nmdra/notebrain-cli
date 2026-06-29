package obsidian

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nmdra/notebrain-cli/internal/store"
)

// EditorCommand constructs an *exec.Cmd to launch the user's $EDITOR for filePath.
// Automatically appends "--wait" for GUI editors if not already specified.
func EditorCommand(filePath string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	parts := strings.Fields(editor)
	bin := parts[0]
	userArgs := parts[1:]

	var args []string
	args = append(args, userArgs...)

	// Check if GUI editor that requires --wait
	baseLower := strings.ToLower(filepath.Base(bin))
	needsWait := strings.Contains(baseLower, "code") ||
		strings.Contains(baseLower, "vscode") ||
		strings.Contains(baseLower, "subl") ||
		strings.Contains(baseLower, "atom") ||
		strings.Contains(baseLower, "mate")

	if needsWait {
		hasWait := false
		for _, a := range userArgs {
			if a == "--wait" || a == "-w" {
				hasWait = true
				break
			}
		}
		if !hasWait {
			args = append(args, "--wait")
		}
	}

	args = append(args, filePath)
	return exec.Command(bin, args...)
}

// OpenInObsidian shells out to the OS opener with an obsidian:// URI.
func OpenInObsidian(vaultName, filePath string) error {
	if filePath == "" {
		return nil
	}
	uri := store.ObsidianURI(vaultName, filePath)
	cmd := exec.Command(openCommand(), uri)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func openCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "start"
	default:
		return "xdg-open"
	}
}
