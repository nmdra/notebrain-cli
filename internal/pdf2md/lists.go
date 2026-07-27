package pdf2md

import "regexp"

var listPattern = regexp.MustCompile(`^(?:[•\-*]|\d+\.)\s+`)

// IsListItem returns true if the text starts with a list bullet or number.
func IsListItem(text string) bool {
	return listPattern.MatchString(text)
}
