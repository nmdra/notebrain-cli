package pdfextract

import (
	"strings"
)

// PDFChunk represents a chunk of text extracted from a PDF.
type PDFChunk struct {
	PageNum int
	Text    string
}

// ChunkPages takes a slice of page texts, strips repetitive headers/footers,
// drops sparse pages, and splits long pages into smaller overlapping chunks.
func ChunkPages(pages []string, minWords, maxRunes, overlap int) []PDFChunk {
	pages = stripHeadersFooters(pages)
	var chunks []PDFChunk

	for i, page := range pages {
		page = strings.TrimSpace(page)
		if len(strings.Fields(page)) < minWords && len([]rune(page)) < minWords*5 {
			continue // skip empty or very sparse pages
		}

		if len(page) <= maxRunes {
			chunks = append(chunks, PDFChunk{PageNum: i + 1, Text: page})
			continue
		}

		// Split long page
		pageChunks := splitPageText(page, maxRunes, overlap)
		for _, pc := range pageChunks {
			chunks = append(chunks, PDFChunk{PageNum: i + 1, Text: pc})
		}
	}

	return chunks
}

func splitPageText(text string, maxRunes, overlap int) []string {
	var chunks []string
	runes := []rune(text)
	length := len(runes)

	for start := 0; start < length; {
		end := start + maxRunes
		if end > length {
			end = length
		} else {
			// try to break at space
			spaceIdx := -1
			for i := end; i > start+maxRunes/2; i-- {
				if i < length && (runes[i] == ' ' || runes[i] == '\n' || runes[i] == '\t') {
					spaceIdx = i
					break
				}
			}
			if spaceIdx != -1 {
				end = spaceIdx
			}
		}

		chunks = append(chunks, strings.TrimSpace(string(runes[start:end])))

		if end == length {
			break
		}

		start = max(end-overlap, start+1)
	}

	return chunks
}

func stripHeadersFooters(pages []string) []string {
	if len(pages) < 3 {
		return pages // Not enough pages to confidently detect repeats
	}

	// Very simple heuristic: look at the first and last line of every page.
	// If the same line appears on >70% of pages, it's a header/footer.
	firstLines := make(map[string]int)
	lastLines := make(map[string]int)

	type pageLines struct {
		first string
		last  string
		lines []string
	}

	parsed := make([]pageLines, len(pages))

	for i, p := range pages {
		lines := strings.Split(p, "\n")
		var validLines []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" {
				validLines = append(validLines, l)
			}
		}

		if len(validLines) > 0 {
			first := validLines[0]
			last := validLines[len(validLines)-1]
			firstLines[first]++
			lastLines[last]++
			parsed[i] = pageLines{first: first, last: last, lines: validLines}
		}
	}

	threshold := int(float64(len(pages)) * 0.7)

	var result []string
	for _, p := range parsed {
		if len(p.lines) == 0 {
			result = append(result, "")
			continue
		}

		startIdx := 0
		endIdx := len(p.lines)

		if firstLines[p.first] >= threshold {
			startIdx = 1
		}
		if len(p.lines) > 1 && lastLines[p.last] >= threshold {
			endIdx = len(p.lines) - 1
		}

		if startIdx >= endIdx {
			result = append(result, "")
		} else {
			result = append(result, strings.Join(p.lines[startIdx:endIdx], "\n"))
		}
	}

	return result
}
