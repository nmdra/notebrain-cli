package cmd

import (
	"fmt"
	"net/url"
	"strings"
)

// formatObsidianLink creates a markdown-style link for terminal output.
// If vaultName is provided, it returns an obsidian:// URI link.
func formatObsidianLink(vaultName, title, filePath string) string {
	if vaultName == "" || vaultName == "." {
		return fmt.Sprintf("[%s](%s)", title, strings.ReplaceAll(filePath, " ", "%20"))
	}

	// Trim the .md extension for the Obsidian URI as per convention
	f := strings.TrimSuffix(filePath, ".md")

	v := url.QueryEscape(vaultName)
	v = strings.ReplaceAll(v, "+", "%20")

	fEnc := url.QueryEscape(f)
	fEnc = strings.ReplaceAll(fEnc, "+", "%20")

	return fmt.Sprintf("[%s](obsidian://open?vault=%s&file=%s)", title, v, fEnc)
}
