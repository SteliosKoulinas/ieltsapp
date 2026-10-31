package eval

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

const defaultBaseURL = "https://generativelanguage.googleapis.com"

type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

// NewClient creates a Gemini client. baseURL is optional and defaults to the
// public Google endpoint; it exists so tests can point at a local server.
func NewClient(apiKey, model, baseURL string) *Client {
	return NewClientWithTimeout(apiKey, model, baseURL, 30*time.Second)
}

// NewClientWithTimeout is NewClient with a custom HTTP timeout, for long
// content-generation requests.
func NewClientWithTimeout(apiKey, model, baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

type part struct {
	Text string `json:"text"`
}
type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}
type genConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMIMEType string  `json:"responseMimeType"`
}
type genRequest struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
	GenerationConfig  genConfig `json:"generationConfig"`
}
type genResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
}

// Generate sends one request to the Gemini REST API and returns the raw text.
// The API key goes in a header, never in the URL, so it cannot leak into logs.
func (c *Client) Generate(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(genRequest{
		SystemInstruction: &content{Parts: []part{{Text: system}}},
		Contents:          []content{{Role: "user", Parts: []part{{Text: user}}}},
		GenerationConfig:  genConfig{Temperature: 0.2, ResponseMIMEType: "application/json"},
	})
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", parseRateLimit(data)
	case resp.StatusCode >= 500:
		return "", fmt.Errorf("%w: status %d", ErrProvider, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("provider rejected request: status %d: %.300s", resp.StatusCode, data)
	}

	var gr genResponse
	if err := json.Unmarshal(data, &gr); err != nil || len(gr.Candidates) == 0 {
		return "", fmt.Errorf("%w: empty or unreadable response", ErrProvider)
	}
	var sb strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String(), nil
}

// parseRateLimit extracts the quota name and retry delay from a Google API 429
// body. If the body is not in the expected shape it returns an empty
// RateLimitError, which still behaves as ErrRateLimited.
func parseRateLimit(body []byte) *RateLimitError {
	rl := &RateLimitError{}
	var env struct {
		Error struct {
			Details []map[string]any `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &env) != nil {
		return rl
	}
	for _, d := range env.Error.Details {
		t, _ := d["@type"].(string)
		switch {
		case strings.HasSuffix(t, "QuotaFailure"):
			if vs, ok := d["violations"].([]any); ok && len(vs) > 0 {
				// Prefer a per-day violation if several are listed.
				for _, v := range vs {
					m, _ := v.(map[string]any)
					id, _ := m["quotaId"].(string)
					if rl.Quota == "" || strings.Contains(strings.ToLower(id), "perday") {
						rl.Quota = id
					}
				}
			}
		case strings.HasSuffix(t, "RetryInfo"):
			rl.RetryDelay, _ = d["retryDelay"].(string)
		}
	}
	return rl
}
