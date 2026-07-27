package llmparse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAICompatConverter struct {
	baseURL string
	apiKey  string
	model   string
	name    string
	headers map[string]string
}

func newOpenAICompatConverter(baseURL, apiKey, model, name string, headers map[string]string) *openAICompatConverter {
	return &openAICompatConverter{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		name:    name,
		headers: headers,
	}
}

func (o *openAICompatConverter) Name() string {
	return o.name
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *openAICompatConverter) Convert(ctx context.Context, pages []string) (string, error) {
	// DeepSeek has 128k context. Most OpenRouter models have at least 128k.
	chunks := splitByTokenBudget(pages, 120_000)

	var results []string
	for _, chunk := range chunks {
		reqBody := chatRequest{
			Model: o.model,
			Messages: []chatMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: chunk},
			},
			Temperature: 0.1,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return "", fmt.Errorf("%s marshal request: %w", o.name, err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("%s create request: %w", o.name, err)
		}

		req.Header.Set("Authorization", "Bearer "+o.apiKey)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range o.headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("%s request failed: %w", o.name, err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("%s read response: %w", o.name, err)
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return "", fmt.Errorf("%s unmarshal response: %w (body: %s)", o.name, err, string(respBody))
		}

		if resp.StatusCode != http.StatusOK {
			errMsg := "unknown error"
			if chatResp.Error != nil && chatResp.Error.Message != "" {
				errMsg = chatResp.Error.Message
			}
			return "", fmt.Errorf("%s API error (status %d): %s", o.name, resp.StatusCode, errMsg)
		}

		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("%s returned no choices. body: %s", o.name, string(respBody))
		}

		results = append(results, chatResp.Choices[0].Message.Content)
	}

	return strings.Join(results, "\n\n"), nil
}
