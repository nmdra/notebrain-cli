package llmparse

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Converter converts raw PDF page text into structured Markdown via an LLM.
type Converter interface {
	// Convert takes raw text pages and returns a single Markdown string.
	Convert(ctx context.Context, pages []string) (string, error)
	// Name returns the backend name for logging (e.g. "gemini", "openrouter", "deepseek").
	Name() string
}

// New creates a Converter for the given model name.
// Backend is auto-detected:
//   - model starts with "deepseek-" -> DeepSeek API (needs DEEPSEEK_API_KEY)
//   - model starts with "openrouter/" -> OpenRouter API (needs OPENROUTER_API_KEY)
//
// Returns an error if the required API key is missing.
func New(model string) (Converter, error) {
	if strings.HasPrefix(model, "deepseek-") {
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		apiKey = strings.Trim(apiKey, "\"'\t\n")
		apiKey = strings.ReplaceAll(apiKey, " ", "")
		if apiKey == "" {
			return nil, fmt.Errorf("DEEPSEEK_API_KEY not set for model %s", model)
		}
		return newOpenAICompatConverter("https://api.deepseek.com", apiKey, model, "deepseek", nil), nil
	}

	if strings.HasPrefix(model, "openrouter/") {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		apiKey = strings.Trim(apiKey, "\"'\t\n")
		apiKey = strings.ReplaceAll(apiKey, " ", "")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY not set for model %s", model)
		}
		actualModel := strings.TrimPrefix(model, "openrouter/")
		headers := map[string]string{
			"HTTP-Referer": "https://github.com/nmdra/notebrain-cli",
			"X-Title":      "NoteBrain CLI",
		}
		return newOpenAICompatConverter("https://openrouter.ai/api/v1", apiKey, actualModel, "openrouter", headers), nil
	}

	return nil, fmt.Errorf("unknown model prefix for %s. Expected deepseek-* or openrouter/*", model)
}
