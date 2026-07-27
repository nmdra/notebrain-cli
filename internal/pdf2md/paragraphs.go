package pdf2md

import (
	"strings"
	"unicode"
)

// GroupBlocks groups classified lines into Markdown Blocks (Headings, Paragraphs, Lists).
func GroupBlocks(pages [][]Line, stats DocumentStats) []Block {
	var blocks []Block
	var currentParagraph []string
	var currentList []string
	var currentCode []string

	flushCode := func() {
		if len(currentCode) > 0 {
			code := strings.Join(currentCode, "\n")
			blocks = append(blocks, CodeBlock{Code: code})
			currentCode = nil
		}
	}

	flushParagraph := func() {
		if len(currentParagraph) > 0 {
			text := strings.Join(currentParagraph, " ")
			blocks = append(blocks, ParagraphBlock{Text: text})
			currentParagraph = nil
		}
	}

	flushList := func() {
		if len(currentList) > 0 {
			blocks = append(blocks, ListBlock{Items: currentList})
			currentList = nil
		}
	}

	for pageIdx, page := range pages {
		for i, line := range page {
			text := strings.TrimSpace(line.FullText())
			if text == "" {
				continue
			}

			if line.IsCode() {
				flushParagraph()
				flushList()
				currentCode = append(currentCode, text)
				continue
			}
			flushCode()

			// If it's a heading, flush current blocks
			if line.HeadingLevel > 0 {
				flushParagraph()
				flushList()
				blocks = append(blocks, HeadingBlock{
					Level: line.HeadingLevel,
					Text:  text,
				})
				continue
			}

			// Check for list item
			if IsListItem(text) {
				flushParagraph()
				// This starts a new list item, but might continue an existing ListBlock
				currentList = append(currentList, text)
				continue
			}

			// If we are currently in a list, check if this line is a continuation of the list item
			if len(currentList) > 0 {
				// Continuation condition: no large gap, doesn't start with a bullet.
				// For simplicity, if it's right under the last list item and no big gap, merge it into the last item.
				if i > 0 {
					prevLine := page[i-1]
					gap := prevLine.Bottom - line.Top
					if stats.MedianLineGap > 0 && gap > stats.MedianLineGap*1.8 {
						// Large gap means the list has ended.
						flushList()
					} else {
						// Append to the last item
						lastIdx := len(currentList) - 1
						currentList[lastIdx] += " " + text
						continue
					}
				} else {
					// new page continuation is tricky, let's just flush list for now if new page doesn't start with bullet
					// or we can append. Let's just flush it.
					flushList()
				}
			}

			// Determine if we should start a new paragraph
			startNewParagraph := false

			if len(currentParagraph) == 0 {
				startNewParagraph = true
			} else {
				if i > 0 {
					prevLine := page[i-1]
					gap := prevLine.Bottom - line.Top
					if stats.MedianLineGap > 0 && gap > stats.MedianLineGap*1.8 {
						startNewParagraph = true
					}
				} else if pageIdx > 0 {
					prevPage := pages[pageIdx-1] //nolint:gosec // pageIdx > 0
					if len(prevPage) > 0 {
						prevLineText := strings.TrimSpace(prevPage[len(prevPage)-1].FullText())
						if len(prevLineText) > 0 {
							lastChar := prevLineText[len(prevLineText)-1]
							if lastChar == '.' || lastChar == '!' || lastChar == '?' || lastChar == ':' {
								startNewParagraph = true
							} else {
								runes := []rune(text)
								if len(runes) > 0 && unicode.IsUpper(runes[0]) {
									startNewParagraph = true
								}
							}
						}
					}
				}
			}

			if startNewParagraph {
				flushParagraph()
			}
			currentParagraph = append(currentParagraph, text)
		}
	}

	flushParagraph()
	flushList()
	flushCode()
	return blocks
}
