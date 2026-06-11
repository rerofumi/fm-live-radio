package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("llm not configured")

type OpenAICompat struct {
	BaseURL string
	APIKey  string
	Model   string

	Client *http.Client
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
}

func (c *OpenAICompat) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" || strings.TrimSpace(c.Model) == "" {
		return "", ErrNotConfigured
	}

	hc := c.Client
	if hc == nil {
		// Local models may need cold-start time before the first token arrives.
		hc = &http.Client{Timeout: 120 * time.Second}
	}

	body, err := json.Marshal(chatReq{
		Model: c.Model,
		Messages: []chatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.6,
		MaxTokens:   8192,
	})
	if err != nil {
		return "", err
	}

	url := base + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("llm http error")
	}

	var out chatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("llm empty choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
