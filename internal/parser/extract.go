package parser

import (
	"path/filepath"
	"regexp"
	"strings"
)

// TitleFromPath derives a fallback title from the relative file path.
func TitleFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}

// Link represents an extracted Obsidian wikilink.
type Link struct {
	Target      string
	DisplayText string
}

var wikiLinkRe = regexp.MustCompile(`\[\[(.*?)\]\]`)

// ExtractLinks finds all Obsidian wikilinks [[Target|Display Text]] in the text.
// It deduplicates links to the same target to prevent graph edge conflicts.
func ExtractLinks(text string) []Link {
	var links []Link
	seen := make(map[string]bool)
	matches := wikiLinkRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		inner := match[1]
		parts := strings.SplitN(inner, "|", 2)
		target := strings.TrimSpace(parts[0])

		if seen[target] {
			continue
		}
		seen[target] = true

		display := target
		if len(parts) > 1 {
			display = strings.TrimSpace(parts[1])
		}
		// target in obsidian is often a relative path or just a note name
		// for now we store it exactly as written.
		links = append(links, Link{
			Target:      target,
			DisplayText: display,
		})
	}
	return links
}

var hashTagRe = regexp.MustCompile(`(?i)\B#([a-z0-9_/-]+)\b`)

// ExtractTags finds #tags in the text.
func ExtractTags(text string, frontmatter string) []string {
	seen := map[string]bool{}
	var tags []string

	// Basic regex tag extraction
	matches := hashTagRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tag := strings.ToLower(match[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}

	// Could extract from frontmatter here too, but skipping for brevity
	return tags
}
