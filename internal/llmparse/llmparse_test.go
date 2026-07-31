package llmparse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewAutoDetection(t *testing.T) {
	os.Clearenv()
	defer os.Clearenv()

	// 1. No keys set -> Error
	_, err := New("some-model", 128000)
	if err == nil {
		t.Errorf("expected error when no API keys are set, got nil")
	} else if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("expected ErrNoAPIKey, got: %v", err)
	}

	// 2. OpenRouter set -> openrouter
	os.Setenv("OPENROUTER_API_KEY", "test-or")
	conv, err := New("some-model", 128000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Name() != "openrouter" {
		t.Errorf("expected backend openrouter, got %s", conv.Name())
	}

	// 3. DeepSeek set -> deepseek (takes precedence over openrouter)
	os.Setenv("DEEPSEEK_API_KEY", "  \"sk-deep seek\"  ")
	conv, err = New("some-model", 128000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Name() != "deepseek" {
		t.Errorf("expected backend deepseek, got %s", conv.Name())
	}

	// Verify key sanitization (space outside and inside quotes)
	cStruct, ok := conv.(*openAICompatConverter)
	if !ok {
		t.Fatalf("expected *openAICompatConverter")
	}
	if cStruct.apiKey != "sk-deepseek" {
		t.Errorf("expected sanitized apiKey 'sk-deepseek', got %q", cStruct.apiKey)
	}

	// 4. Ollama keyless via OLLAMA_HOST when no keys are set
	os.Clearenv()
	os.Setenv("OLLAMA_HOST", "http://localhost:11434")
	conv, err = New("some-model", 128000)
	if err != nil {
		t.Fatalf("unexpected error with OLLAMA_HOST: %v", err)
	}
	if conv.Name() != "ollama" {
		t.Errorf("expected backend ollama, got %s", conv.Name())
	}
}

func TestContextWindowValidation(t *testing.T) {
	os.Setenv("DEEPSEEK_API_KEY", "test")
	defer os.Clearenv()

	_, err := New("some-model", 8000)
	if err == nil {
		t.Errorf("expected error for context window < 12288, got nil")
	}
}

func TestOpenAICompatConverter(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		resp := chatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			Message: struct {
				Content string `json:"content"`
			}{
				Content: "structured markdown",
			},
		})

		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// Test trailing slash removal on base URL
	baseURLWithTrailingSlash := mockServer.URL + "/"
	conv := newOpenAICompatConverter(baseURLWithTrailingSlash, "test-key", "test-model", "test-backend", 128000, nil)

	result, err := conv.Convert(context.Background(), []string{"raw text"})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result != "structured markdown" {
		t.Errorf("expected 'structured markdown', got %q", result)
	}
}

func TestOpenAICompatConverter_RetryOn429(t *testing.T) {
	attempts := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "rate limit exceeded"}}`))
			return
		}

		resp := chatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			Message: struct {
				Content string `json:"content"`
			}{
				Content: "recovered markdown",
			},
		})
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	conv := newOpenAICompatConverter(mockServer.URL, "test-key", "test-model", "test-backend", 128000, nil)

	result, err := conv.Convert(context.Background(), []string{"raw text"})
	if err != nil {
		t.Fatalf("Convert failed on retry: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}

	if result != "recovered markdown" {
		t.Errorf("expected 'recovered markdown', got %q", result)
	}
}

func TestSplitByTokenBudget(t *testing.T) {
	pages := []string{
		strings.Repeat("a", 20), // 20 chars
		strings.Repeat("b", 15), // 15 chars (total 35) -> should fit in chunk 1
		strings.Repeat("c", 10), // 10 chars (total 45) -> exceeds 40, goes to chunk 2
		strings.Repeat("d", 45), // 45 chars -> exceeds 40, single huge page, goes to chunk 3
	}

	chunks := splitByTokenBudget(pages, 10)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	if chunks[0] != strings.Repeat("a", 20)+"\n\n---\n\n"+strings.Repeat("b", 15) {
		t.Errorf("unexpected chunk 0: %q", chunks[0])
	}

	if chunks[1] != strings.Repeat("c", 10) {
		t.Errorf("unexpected chunk 1: %q", chunks[1])
	}

	if chunks[2] != strings.Repeat("d", 45) {
		t.Errorf("unexpected chunk 2: %q", chunks[2])
	}
}

func TestSanitizeAPIKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  sk-12345  ", "sk-12345"},
		{"\"sk-12345\"", "sk-12345"},
		{"'sk-12345'", "sk-12345"},
		{"  \"  sk-12 345  \"  ", "sk-12345"},
		{"\tsk-123\n", "sk-123"},
	}

	for _, tt := range tests {
		got := sanitizeAPIKey(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeAPIKey(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewWithEnvLookup(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key-or")

	conv, err := New("model", 128000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Name() != "openrouter" {
		t.Errorf("expected openrouter, got %s", conv.Name())
	}
}

func TestOllamaHostPathSuffix(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"http://localhost:11434", "http://localhost:11434/v1"},
		{"http://remote-ollama:11434/", "http://remote-ollama:11434/v1"},
		{"http://remote-ollama:11434/v1", "http://remote-ollama:11434/v1"},
		{"http://remote-ollama:11434/v1/", "http://remote-ollama:11434/v1"},
	}

	for _, tt := range tests {
		t.Setenv("OLLAMA_HOST", tt.host)
		conv, err := New("llama3", 128000)
		if err != nil {
			t.Fatalf("New failed for host %q: %v", tt.host, err)
		}
		cStruct, ok := conv.(*openAICompatConverter)
		if !ok {
			t.Fatalf("expected *openAICompatConverter")
		}
		if cStruct.baseURL != tt.expected {
			t.Errorf("for OLLAMA_HOST %q, expected baseURL %q, got %q", tt.host, tt.expected, cStruct.baseURL)
		}
	}
}

func TestOpenAICompatConverter_MultiChunk(t *testing.T) {
	reqCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		resp := chatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			Message: struct {
				Content string `json:"content"`
			}{
				Content: "markdown part " + strings.Repeat("x", 5),
			},
		})
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	conv := newOpenAICompatConverter(mockServer.URL, "test-key", "test-model", "test-backend", 12288, nil)

	// Pass 2 distinct pages that force multi-chunk splitting with a small token budget
	pages := []string{
		strings.Repeat("a", 20000),
		strings.Repeat("b", 20000),
	}

	result, err := conv.Convert(context.Background(), pages)
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if reqCount != 2 {
		t.Errorf("expected 2 requests for multi-chunk, got %d", reqCount)
	}

	if !strings.Contains(result, "\n\n") {
		t.Errorf("expected multi-chunk output joined with newlines, got %q", result)
	}
}

func TestExtractErrorMessage(t *testing.T) {
	var resp chatResponse
	resp.Error = &struct {
		Message string `json:"message"`
	}{Message: "explicit error message"}

	msg := extractErrorMessage(resp, []byte(`{"error":{"message":"explicit error message"}}`))
	if msg != "explicit error message" {
		t.Errorf("expected 'explicit error message', got %q", msg)
	}

	// Fallback to body text
	var emptyResp chatResponse
	msgBody := extractErrorMessage(emptyResp, []byte("raw server error string"))
	if msgBody != "raw server error string" {
		t.Errorf("expected 'raw server error string', got %q", msgBody)
	}

	// Default unknown error
	msgUnknown := extractErrorMessage(emptyResp, nil)
	if msgUnknown != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", msgUnknown)
	}
}
