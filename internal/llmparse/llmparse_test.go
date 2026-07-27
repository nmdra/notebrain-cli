package llmparse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewFactory(t *testing.T) {
	// Setup env
	os.Setenv("DEEPSEEK_API_KEY", "test-deepseek")
	os.Setenv("OPENROUTER_API_KEY", "test-or")
	defer os.Clearenv()

	tests := []struct {
		model    string
		wantName string
		wantErr  bool
	}{
		{"deepseek-chat", "deepseek", false},
		{"openrouter/anthropic/claude-sonnet", "openrouter", false},
		{"unknown-model", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			conv, err := New(tt.model)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for model %s, got nil", tt.model)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for model %s: %v", tt.model, err)
			}
			if conv.Name() != tt.wantName {
				t.Errorf("expected name %s, got %s", tt.wantName, conv.Name())
			}
		})
	}
}

func TestOpenAICompatConverter(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	conv := newOpenAICompatConverter(mockServer.URL, "test-key", "test-model", "test-backend", nil)

	result, err := conv.Convert(context.Background(), []string{"raw text"})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	if result != "structured markdown" {
		t.Errorf("expected 'structured markdown', got %q", result)
	}
}

func TestSplitByTokenBudget(t *testing.T) {
	// A chunk that exceeds the budget to test splitting
	// budget is maxTokens, characters ~ maxTokens * 4
	// Let's test with maxTokens = 10, so maxChars = 40

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
