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

// minContextWindow is the smallest context window accepted by New:
// 8192 tokens reserved for the system prompt and output, plus the
// 4096-token minimum chunk size.
const minContextWindow = 12288

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

// backendHintFromModel returns the backend name suggested by the model
// string, e.g. "ollama/llama3" -> "ollama", "deepseek-v4-flash" -> "deepseek".
// It returns "" when the model does not name a backend.
func backendHintFromModel(model string) string {
	for _, b := range supportedBackends {
		if strings.HasPrefix(model, b.Name+"/") || strings.HasPrefix(model, b.Name+"-") {
			return b.Name
		}
	}
	return ""
}

// normalizeOllamaHost appends the OpenAI-compatible /v1 suffix to an
// OLLAMA_HOST value.
func normalizeOllamaHost(host string) string {
	host = strings.TrimSuffix(strings.TrimSpace(host), "/")
	if !strings.HasSuffix(host, "/v1") {
		host += "/v1"
	}
	return host
}

// newBackendConverter builds a Converter for the explicitly named backend.
func newBackendConverter(name, model string, contextWindow int) (Converter, error) {
	var b *BackendConfig
	for i := range supportedBackends {
		if supportedBackends[i].Name == name {
			b = &supportedBackends[i]
			break
		}
	}
	if b == nil {
		return nil, fmt.Errorf("unsupported backend %q for model %s", name, model)
	}

	key := sanitizeAPIKey(os.Getenv(b.EnvKey))
	if b.Name == backendOllama {
		if key == "" && os.Getenv("OLLAMA_HOST") == "" {
			return nil, fmt.Errorf("%w: %s or OLLAMA_HOST is required for model %s", ErrNoAPIKey, b.EnvKey, model)
		}
		if key == "" {
			key = "dummy"
		}
	} else if key == "" {
		return nil, fmt.Errorf("%w: %s is required for model %s", ErrNoAPIKey, b.EnvKey, model)
	}

	baseURL := b.BaseURL
	if b.Name == backendOllama {
		if host := os.Getenv("OLLAMA_HOST"); host != "" {
			baseURL = normalizeOllamaHost(host)
		}
	}

	// The backend prefix is a namespace, not part of the model id
	// ("openrouter/tencent/hy3" -> "tencent/hy3"). Dash-prefixed model
	// names ("deepseek-v4-flash") are real model ids and stay intact.
	actualModel := strings.TrimPrefix(model, b.Name+"/")

	slog.Info("using LLM backend", "backend", b.Name, "model", actualModel)
	return newOpenAICompatConverter(baseURL, key, actualModel, b.Name, contextWindow, b.Headers), nil
}

// New creates a Converter for the given model name.
func New(model string, contextWindow int) (Converter, error) {
	if contextWindow < minContextWindow {
		return nil, fmt.Errorf("context window %d is too small (needs at least %d tokens)", contextWindow, minContextWindow)
	}

	// An explicit backend prefix in the model wins over environment
	// detection, e.g. --llm-model="ollama/llama3".
	if hint := backendHintFromModel(model); hint != "" {
		conv, err := newBackendConverter(hint, model, contextWindow)
		if err == nil {
			return conv, nil
		}
		if !errors.Is(err, ErrNoAPIKey) {
			return nil, err
		}
		slog.Warn("backend named by model is not configured, falling back to environment detection",
			"backend", hint, "model", model, "err", err)
	}

	// Auto-detect based on API key presence.
	for i := range supportedBackends {
		b := &supportedBackends[i]
		key := sanitizeAPIKey(os.Getenv(b.EnvKey))
		host := ""
		if b.Name == backendOllama && key == "" {
			if h := os.Getenv("OLLAMA_HOST"); h != "" {
				key = "dummy"
				host = h
			}
		}
		if key == "" {
			continue
		}

		// Honor OLLAMA_HOST even when OLLAMA_API_KEY is set.
		baseURL := b.BaseURL
		if b.Name == backendOllama {
			if host != "" {
				baseURL = normalizeOllamaHost(host)
			} else if h := os.Getenv("OLLAMA_HOST"); h != "" {
				baseURL = normalizeOllamaHost(h)
			}
		}

		if hint := backendHintFromModel(model); hint != "" && hint != b.Name {
			slog.Warn("backend auto-detected from environment does not match the model name",
				"backend", b.Name, "model", model, "expected", hint)
		}

		actualModel := strings.TrimPrefix(model, backendOpenRouter+"/")
		slog.Info("using LLM backend", "backend", b.Name, "model", actualModel)
		return newOpenAICompatConverter(baseURL, key, actualModel, b.Name, contextWindow, b.Headers), nil
	}

	return nil, fmt.Errorf("%w for auto-detection (checked DEEPSEEK_API_KEY, OPENROUTER_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY, OLLAMA_API_KEY/OLLAMA_HOST) for model %s", ErrNoAPIKey, model)
}
