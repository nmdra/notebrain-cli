package ingest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ObsidianAppConfig represents relevant fields from .obsidian/app.json.
type ObsidianAppConfig struct {
	UserIgnoreFilters    []string `json:"userIgnoreFilters"`
	AttachmentFolderPath string   `json:"attachmentFolderPath"`
}

// LoadExcludedPaths reads the userIgnoreFilters and attachmentFolderPath from .obsidian/app.json.
// Returns nil if the file is absent or unreadable.
func LoadExcludedPaths(vaultPath string) []string {
	data, err := os.ReadFile(filepath.Join(vaultPath, ".obsidian", "app.json"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("failed to read .obsidian/app.json", "vault_path", vaultPath, "err", err)
		}
		return nil
	}
	var config ObsidianAppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		slog.Warn("failed to parse .obsidian/app.json", "vault_path", vaultPath, "err", err)
		return nil
	}
	filters := config.UserIgnoreFilters
	if config.AttachmentFolderPath != "" {
		filters = append(filters, config.AttachmentFolderPath)
	}
	return filters
}

// IsExcluded checks if the relative path matches any ignore filters.
func IsExcluded(relPath string, filters []string) bool {
	normalized := filepath.ToSlash(relPath)
	for _, filter := range filters {
		if matchFilter(normalized, filter) {
			return true
		}
	}
	return false
}

func matchFilter(normalizedPath, filter string) bool {
	filter = strings.TrimRight(filter, "/")
	if !strings.ContainsAny(filter, "*?[") {
		return normalizedPath == filter || strings.HasPrefix(normalizedPath, filter+"/")
	}
	if strings.HasPrefix(filter, "**/") {
		return matchPathOrSegments(normalizedPath, filter[3:])
	}
	return matchPathOrSegments(normalizedPath, filter)
}

func matchPathOrSegments(path, pattern string) bool {
	if matched, _ := filepath.Match(pattern, path); matched {
		return true
	}
	remaining := path
	for len(remaining) > 0 {
		var segment string
		segment, remaining, _ = strings.Cut(remaining, "/")
		if segment != "" {
			if matched, _ := filepath.Match(pattern, segment); matched {
				return true
			}
		}
	}
	return false
}
