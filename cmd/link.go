package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// hyperlinkSupported returns true if the current terminal supports OSC 8.
func hyperlinkSupported() bool {
	if noHyperlinks || os.Getenv("NO_HYPERLINKS") != "" {
		return false
	}
	term := os.Getenv("TERM")
	prog := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	color := strings.ToLower(os.Getenv("COLORTERM"))

	// Known-good terminals
	switch prog {
	case "iterm.app", "wezterm", "ghostty", "hyper":
		return true
	}
	if color == "truecolor" || color == "24bit" {
		return true
	}
	if strings.HasPrefix(term, "xterm-kitty") || strings.HasPrefix(term, "foot") {
		return true
	}
	// Windows Terminal
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	return false
}

// obsidianURI builds an obsidian://open URI for a vault-relative file path.
func obsidianURI(vaultName, filePath string) string {
	filePath = strings.TrimSuffix(filePath, ".md")

	escape := func(s string) string {
		return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
	}

	if vaultName != "" {
		return "obsidian://open?vault=" + escape(vaultName) + "&file=" + escape(filePath)
	}
	return "obsidian://open?file=" + escape(filePath)
}

// hyperlink wraps visible text in an OSC 8 terminal hyperlink.
func hyperlink(uri, text string) string {
	if !hyperlinkSupported() {
		return text
	}
	// OSC 8 format: ESC ] 8 ; params ; uri ST  text  ESC ] 8 ; ; ST
	return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", uri, text)
}
