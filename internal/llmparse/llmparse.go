package llmparse

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ErrNoAPIKey is returned when no valid API key is found for LLM parsing.
var ErrNoAPIKey = errors.New("no valid API key found")

// API Endpoints
const (
	EndpointDeepSeek   = "https://api.deepseek.com"
	EndpointOpenRouter = "https://openrouter.ai/api/v1"
	EndpointOpenAI     = "https://api.openai.com/v1"
	EndpointGemini     = "https://generativelanguage.googleapis.com/v1beta/openai/"
	EndpointOllama     = "http://localhost:11434/v1"
)

const (
	backendDeepSeek   = "deepseek"
	backendOpenRouter = "openrouter"
	backendOpenAI     = "openai"
	backendGemini     = "gemini"
	backendOllama     = "ollama"
)

type BackendConfig struct {
	Name    string
	BaseURL string
	EnvKey  string
	Headers map[string]string
}

var supportedBackends = []BackendConfig{
	{Name: backendDeepSeek, BaseURL: EndpointDeepSeek, EnvKey: "DEEPSEEK_API_KEY"},
	{
		Name:    backendOpenRouter,
		BaseURL: EndpointOpenRouter,
		EnvKey:  "OPENROUTER_API_KEY",
		Headers: map[string]string{
			"HTTP-Referer": "https://github.com/nmdra/notebrain-cli",
			"X-Title":      "NoteBrain CLI",
		},
	},
	{Name: backendOpenAI, BaseURL: EndpointOpenAI, EnvKey: "OPENAI_API_KEY"},
	{Name: backendGemini, BaseURL: EndpointGemini, EnvKey: "GEMINI_API_KEY"},
	{Name: backendOllama, BaseURL: EndpointOllama, EnvKey: "OLLAMA_API_KEY"},
}

// Converter converts raw PDF page text into structured Markdown via an LLM.
type Converter interface {
	// Convert takes raw text pages and returns a single Markdown string.
	Convert(ctx context.Context, pages []string) (string, error)
	// Name returns the backend name for logging (e.g. "gemini", "openrouter", "deepseek").
	Name() string
}

// sanitizeAPIKey removes surrounding quotes, whitespace, and internal spaces.
func sanitizeAPIKey(raw string) string {
	val := strings.Trim(strings.TrimSpace(raw), "\"'\t\n")
	return strings.ReplaceAll(val, " ", "")
}

// New creates a Converter for the given model name.
func New(model string, contextWindow int) (Converter, error) {
	if contextWindow < 12288 { // 8192 reserve + 4096 min chunk size
		return nil, fmt.Errorf("context window %d is too small (needs at least 12288 tokens)", contextWindow)
	}

	var selected *BackendConfig
	var apiKey string
	var baseURL string

	// Auto-detect based on API key presence
	for i := range supportedBackends {
		b := &supportedBackends[i]
		val := os.Getenv(b.EnvKey)
		val = sanitizeAPIKey(val)

		// Special case for Ollama keyless API
		if b.Name == backendOllama && val == "" {
			if host := os.Getenv("OLLAMA_HOST"); host != "" {
				val = "dummy"
				host = strings.TrimSuffix(strings.TrimSpace(host), "/")
				if !strings.HasSuffix(host, "/v1") {
					host += "/v1"
				}
				baseURL = host
			}
		}

		if val != "" {
			selected = b
			apiKey = val
			if baseURL == "" {
				baseURL = b.BaseURL
			}
			break
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("%w for auto-detection (checked DEEPSEEK_API_KEY, OPENROUTER_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY, OLLAMA_API_KEY/OLLAMA_HOST) for model %s", ErrNoAPIKey, model)
	}

	actualModel := strings.TrimPrefix(model, backendOpenRouter+"/")

	slog.Info("using LLM backend", "backend", selected.Name, "model", actualModel)
	return newOpenAICompatConverter(baseURL, apiKey, actualModel, selected.Name, contextWindow, selected.Headers), nil
}
