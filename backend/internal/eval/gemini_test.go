package eval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These bodies follow the shape of Google API error responses as documented
// (error.details with QuotaFailure and RetryInfo). They are fixtures written
// for the tests, not captured from a live call.
const dailyBody = `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[
 {"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[
   {"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"},
   {"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]},
 {"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"34s"}]}}`

const minuteBody = `{"error":{"code":429,"details":[
 {"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerMinutePerProjectPerModel-FreeTier"}]},
 {"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"12s"}]}}`

func TestParseRateLimit(t *testing.T) {
	d := parseRateLimit([]byte(dailyBody))
	if !d.Daily() || d.RetryDelay != "34s" {
		t.Errorf("daily body: %+v", d)
	}
	m := parseRateLimit([]byte(minuteBody))
	if m.Daily() || m.RetryDelay != "12s" || m.Quota == "" {
		t.Errorf("minute body: %+v", m)
	}
	for _, junk := range []string{"", "not json", `{"error":{}}`, `{"error":{"details":[{"@type":5}]}}`} {
		r := parseRateLimit([]byte(junk))
		if r.Daily() || !errors.Is(r, ErrRateLimited) {
			t.Errorf("junk %q: %+v", junk, r)
		}
	}
}

func TestGenerateStatusHandling(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(t *testing.T, text string, err error)
	}{
		{"ok", 200, `{"candidates":[{"content":{"parts":[{"text":"hello "},{"text":"world"}]}}]}`,
			func(t *testing.T, text string, err error) {
				if err != nil || text != "hello world" {
					t.Fatalf("text=%q err=%v", text, err)
				}
			}},
		{"rate limited daily", 429, dailyBody,
			func(t *testing.T, _ string, err error) {
				var rl *RateLimitError
				if !errors.Is(err, ErrRateLimited) || !errors.As(err, &rl) || !rl.Daily() {
					t.Fatalf("err=%v", err)
				}
			}},
		{"provider unavailable", 503, `{}`,
			func(t *testing.T, _ string, err error) {
				if !errors.Is(err, ErrProvider) {
					t.Fatalf("err=%v", err)
				}
			}},
		{"no candidates", 200, `{"candidates":[]}`,
			func(t *testing.T, _ string, err error) {
				if !errors.Is(err, ErrProvider) {
					t.Fatalf("err=%v", err)
				}
			}},
		{"bad request is not retried as provider error", 400, `{"error":"bad"}`,
			func(t *testing.T, _ string, err error) {
				if err == nil || errors.Is(err, ErrProvider) || errors.Is(err, ErrRateLimited) {
					t.Fatalf("err=%v", err)
				}
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-goog-api-key") != "secret-key" {
					t.Error("api key must be sent in the x-goog-api-key header")
				}
				if r.URL.RawQuery != "" {
					t.Errorf("api key must never be in the URL, got query %q", r.URL.RawQuery)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := NewClient("secret-key", "test-model", srv.URL)
			text, err := c.Generate(context.Background(), "sys", "user")
			tc.check(t, text, err)
		})
	}
}

func TestThinkingBudgetIsOptional(t *testing.T) {
	tests := []struct {
		name   string
		budget *int
		want   bool
	}{
		{"default", nil, false},
		{"off", intPtr(0), true},
		{"fixed", intPtr(1024), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got genRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
			}))
			defer srv.Close()

			c := NewClient("secret-key", "test-model", srv.URL)
			if tc.budget != nil {
				c.SetThinkingBudget(*tc.budget)
			}
			if _, err := c.Generate(context.Background(), "sys", "user"); err != nil {
				t.Fatal(err)
			}
			if got.GenerationConfig.ThinkingConfig != nil != tc.want {
				t.Fatalf("thinking config present = %v, want %v", got.GenerationConfig.ThinkingConfig != nil, tc.want)
			}
			if tc.budget != nil && got.GenerationConfig.ThinkingConfig.ThinkingBudget != *tc.budget {
				t.Errorf("budget = %d, want %d", got.GenerationConfig.ThinkingConfig.ThinkingBudget, *tc.budget)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
