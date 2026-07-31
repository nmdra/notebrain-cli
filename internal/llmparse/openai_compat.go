package llmparse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	reservedTokens = 8192
	maxRetries     = 3
	initialBackoff = 2 * time.Second
)

// maxRetryAfter caps how long a Retry-After header can stall the retry.
// It is a var so tests can shrink it without sleeping.
var maxRetryAfter = 60 * time.Second

type openAICompatConverter struct {
	baseURL       string
	apiKey        string
	model         string
	name          string
	contextWindow int
	headers       map[string]string
	client        *http.Client
}

func newOpenAICompatConverter(baseURL, apiKey, model, name string, contextWindow int, headers map[string]string) *openAICompatConverter {
	return &openAICompatConverter{
		baseURL:       baseURL,
		apiKey:        apiKey,
		model:         model,
		name:          name,
		contextWindow: contextWindow,
		headers:       headers,
		client:        &http.Client{Timeout: 90 * time.Second},
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

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		return time.Until(t)
	}
	return 0
}

func (o *openAICompatConverter) Convert(ctx context.Context, pages []string) (string, error) {
	budget := o.contextWindow - reservedTokens

	chunks := splitByTokenBudget(pages, budget)
	totalChunks := len(chunks)

	var results []string
	for i, chunk := range chunks {
		if totalChunks > 1 {
			slog.Info("converting PDF chunk", "backend", o.name, "chunk", i+1, "total", totalChunks)
		}

		res, err := o.convertChunk(ctx, chunk)
		if err != nil {
			return "", fmt.Errorf("%s chunk %d/%d: %w", o.name, i+1, totalChunks, err)
		}
		results = append(results, res)
	}

	return strings.Join(results, "\n\n"), nil
}

func (o *openAICompatConverter) convertChunk(ctx context.Context, chunk string) (string, error) {
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
		return "", fmt.Errorf("marshal request: %w", err)
	}

	reqURL := strings.TrimSuffix(o.baseURL, "/") + "/chat/completions"

	respBody, err := o.doWithRetry(ctx, reqURL, bodyBytes)
	if err != nil {
		return "", err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("returned no choices. body: %s", string(respBody))
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (o *openAICompatConverter) doWithRetry(ctx context.Context, reqURL string, bodyBytes []byte) ([]byte, error) {
	backoff := initialBackoff

	for attempt := 0; ; attempt++ {
		if attempt > maxRetries {
			return nil, fmt.Errorf("request failed after %d attempts", maxRetries+1)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+o.apiKey)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range o.headers {
			req.Header.Set(k, v)
		}

		resp, err := o.client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				slog.Warn("request failed, retrying", "backend", o.name, "attempt", attempt+1, "err", err, "backoff", backoff)
				if sleepErr := sleepWithContext(ctx, backoff); sleepErr != nil {
					return nil, sleepErr
				}
				backoff *= 2
				continue
			}
			return nil, fmt.Errorf("request failed: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxRetries {
				slog.Warn("reading response body failed, retrying", "backend", o.name, "attempt", attempt+1, "err", err, "backoff", backoff)
				if sleepErr := sleepWithContext(ctx, backoff); sleepErr != nil {
					return nil, sleepErr
				}
				backoff *= 2
				continue
			}
			return nil, fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return respBody, nil
		}

		var chatResp chatResponse
		_ = json.Unmarshal(respBody, &chatResp)

		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < maxRetries {
			sleepDuration := backoff
			if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
				sleepDuration = min(ra, maxRetryAfter)
			}
			errMsg := extractErrorMessage(chatResp, respBody)
			slog.Warn("rate limited or server error, retrying", "backend", o.name, "status", resp.StatusCode, "err", errMsg, "attempt", attempt+1, "backoff", sleepDuration)
			if sleepErr := sleepWithContext(ctx, sleepDuration); sleepErr != nil {
				return nil, sleepErr
			}
			backoff *= 2
			continue
		}

		errMsg := extractErrorMessage(chatResp, respBody)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errMsg)
	}
}

func extractErrorMessage(chatResp chatResponse, respBody []byte) string {
	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return chatResp.Error.Message
	}
	if len(respBody) > 0 {
		return strings.TrimSpace(string(respBody))
	}
	return "unknown error"
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
