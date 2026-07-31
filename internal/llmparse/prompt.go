package llmparse

import (
	"strings"
	"unicode/utf8"
)

const systemPrompt = `You are an expert document structure analyzer. Convert the raw text extracted from a PDF into clean, well-structured Markdown.

Rules:
1. Reconstruct logical heading hierarchy using #, ##, ### based on document structure.
2. Merge lines that belong to the same paragraph into flowing text.
3. Format code blocks with ` + "```" + ` fences and appropriate language tags.
4. Format tables using GitHub Flavored Markdown table syntax.
5. Convert bullet points and numbered lists to standard Markdown lists.
6. Format mathematical expressions using LaTeX ($...$ or $$...$$).
7. REMOVE page numbers, running headers/footers, watermarks, and navigation artifacts.
8. REMOVE gibberish text, garbled characters, or obvious OCR artifacts. Only remove clearly non-linguistic noise (e.g. repeated random character strings or scanning artifacts); when in doubt, keep the text to preserve unfamiliar technical notation or formulas.
9. Do NOT add, summarize, or modify any content — preserve the original text exactly, except for removing garbage/gibberish.
10. Output ONLY raw Markdown. No explanations, no wrapping fences, no preamble.`

const pageSeparator = "\n\n---\n\n"

// splitByTokenBudget splits PDF pages into chunks that fit within maxTokens.
// Uses page boundaries as natural split points to avoid mid-sentence breaks.
// Sizes are counted in runes, not bytes, so multi-byte text (accents, CJK)
// is charged the same as ASCII. A page larger than the budget is split on
// rune boundaries instead of being sent whole.
func splitByTokenBudget(pages []string, maxTokens int) []string {
	if len(pages) == 0 {
		return nil
	}

	// Estimate: 1 token ≈ 4 characters (conservative for English)
	maxChars := maxTokens * 4
	sepLen := utf8.RuneCountInString(pageSeparator)

	var chunks []string
	var currentChunk strings.Builder
	var currentLen int

	for _, page := range pages {
		pageLen := utf8.RuneCountInString(page)

		if pageLen > maxChars {
			// A single page exceeds the budget: flush what we have and
			// split the page on rune boundaries.
			if currentLen > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
				currentLen = 0
			}
			runes := []rune(page)
			for start := 0; start < len(runes); start += maxChars {
				end := min(start+maxChars, len(runes))
				chunks = append(chunks, string(runes[start:end]))
			}
			continue
		}

		if currentLen > 0 && currentLen+sepLen+pageLen > maxChars {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
			currentLen = 0
		}

		if currentLen > 0 {
			currentChunk.WriteString(pageSeparator)
			currentLen += sepLen
		}
		currentChunk.WriteString(page)
		currentLen += pageLen
	}

	if currentLen > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}
