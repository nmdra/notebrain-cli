package llmparse

import "strings"

const systemPrompt = `You are an expert document structure analyzer. Convert the raw text extracted from a PDF into clean, well-structured Markdown.

Rules:
1. Reconstruct logical heading hierarchy using #, ##, ### based on document structure.
2. Merge lines that belong to the same paragraph into flowing text.
3. Format code blocks with ` + "```" + ` fences and appropriate language tags.
4. Format tables using GitHub Flavored Markdown table syntax.
5. Convert bullet points and numbered lists to standard Markdown lists.
6. Format mathematical expressions using LaTeX ($...$ or $$...$$).
7. REMOVE page numbers, running headers/footers, watermarks, and navigation artifacts.
8. Do NOT add, summarize, or modify any content — preserve the original text exactly.
9. Output ONLY raw Markdown. No explanations, no wrapping fences, no preamble.`

// splitByTokenBudget splits PDF pages into chunks that fit within maxTokens.
// Uses page boundaries as natural split points to avoid mid-sentence breaks.
func splitByTokenBudget(pages []string, maxTokens int) []string {
	if len(pages) == 0 {
		return nil
	}

	// Estimate: 1 token ≈ 4 characters (conservative for English)
	maxChars := maxTokens * 4

	var chunks []string
	var currentChunk strings.Builder
	var currentLen int

	for _, page := range pages {
		pageLen := len(page)

		// If a single page exceeds the max, we have to chunk it directly (rare for a single PDF page)
		if pageLen > maxChars {
			if currentLen > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
				currentLen = 0
			}
			// Just add it as one huge chunk and hope the LLM accepts it, or split it arbitrarily.
			// Given maxChars is usually 120k+ (DeepSeek) or 900k+ (Gemini), a single page will never exceed this.
			chunks = append(chunks, page)
			continue
		}

		if currentLen+pageLen > maxChars {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
			currentLen = 0
		}

		if currentLen > 0 {
			currentChunk.WriteString("\n\n---\n\n") // Page separator
			currentLen += 9
		}
		currentChunk.WriteString(page)
		currentLen += pageLen
	}

	if currentLen > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}
