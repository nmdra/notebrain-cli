package store

import (
	"net/url"
	"strings"
)

// ObsidianURI builds an obsidian://open URI for a vault-relative file path.
func ObsidianURI(vaultName, filePath string) string {
	filePath = strings.TrimSuffix(filePath, ".md")

	escape := func(s string) string {
		return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
	}

	if vaultName != "" {
		return "obsidian://open?vault=" + escape(vaultName) + "&file=" + escape(filePath)
	}
	return "obsidian://open?file=" + escape(filePath)
}
