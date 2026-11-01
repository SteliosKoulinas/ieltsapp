package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ielts/backend/internal/eval"
	"ielts/backend/internal/httpapi"
	"ielts/backend/internal/store"
	"ielts/backend/internal/store/storetest"
)

// fakeEvaluator stands in for the AI provider so tests are deterministic and
// free. It records every call so tests can prove no duplicate evaluation ran.
type fakeEvaluator struct {
	mu    sync.Mutex
	calls int
	last  eval.Request
	fn    func(req eval.Request) (*eval.Evaluation, error)
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, req eval.Request) (*eval.Evaluation, error) {
	f.mu.Lock()
	f.calls++
	f.last = req
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(req)
	}
	return &eval.Evaluation{Verdict: "mostly_correct", Summary: "Good.", Strengths: []string{"clear"},
		Issues:          []eval.Issue{{Category: "articles", Severity: "error", Original: "a", Correction: "the", Explanation: "e"}},
		ImprovedVersion: "better", Confidence: "high", Raw: "{}", Calls: 1}, nil
}

type env struct {
	t  *testing.T
	h  http.Handler
	ev *fakeEvaluator
	st *store.Store
}

func newEnv(t *testing.T, perMinute int) *env {
	t.Helper()
	st := storetest.New(t)
	if _, err := st.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	ev := &fakeEvaluator{}
	api := httpapi.New(st, ev, "test-model", perMinute, 8, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &env{t: t, h: api.Handler(), ev: ev, st: st}
}

func (e *env) do(method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const id1 = "11111111-1111-4111-8111-111111111111"
const id2 = "22222222-2222-4222-8222-222222222222"

func (e *env) exercise(typ string) map[string]any {
	e.t.Helper()
	code, ex := e.do("GET", "/api/exercises/next?type="+typ, nil)
	if code != 200 {
		e.t.Fatalf("next exercise: %d %v", code, ex)
	}
	return ex
}

func exID(ex map[string]any) float64 { return ex["id"].(float64) }

func TestHealthAndUnknownRoute(t *testing.T) {
	e := newEnv(t, 60)
	if code, body := e.do("GET", "/api/health", nil); code != 200 || body["status"] != "ok" {
		t.Errorf("health: %d %v", code, body)
	}
	if code, body := e.do("GET", "/api/nope", nil); code != 404 || body["error"] != "not_found" {
		t.Errorf("unknown route: %d %v", code, body)
	}
}

func TestNextExercise(t *testing.T) {
	e := newEnv(t, 60)
	vocab := e.exercise("vocab")
	if vocab["type"] != "vocab" || vocab["target_word"] == nil || vocab["context"] == nil {
		t.Errorf("vocab exercise incomplete: %v", vocab)
	}
	if _, leaked := vocab["intended_meaning"]; leaked {
		t.Error("the reference meaning must never reach the browser")
	}
	if w := e.exercise("writing"); w["type"] != "writing" || w["expected_length"] == nil {
		t.Errorf("writing exercise incomplete: %v", w)
	}
	if code, _ := e.do("GET", "/api/exercises/next?type=grammar", nil); code != 400 {
		t.Errorf("invalid type should be 400, got %d", code)
	}
	if code, _ := e.do("GET", "/api/exercises/next?exclude=abc", nil); code != 400 {
		t.Errorf("invalid exclude should be 400, got %d", code)
	}
	if code, _ := e.do("GET", "/api/exercises/next?unseen=maybe", nil); code != 400 {
		t.Errorf("invalid unseen should be 400, got %d", code)
	}
	a := e.exercise("any")
	b := func() map[string]any {
		_, ex := e.do("GET", "/api/exercises/next?exclude="+strings.TrimSuffix(strings.TrimPrefix(jsonNum(a["id"]), ""), ""), nil)
		return ex
	}()
	if exID(a) == exID(b) {
		t.Error("exclude should avoid repeating the same exercise")
	}
	// unseen=1 asks only for material never seen, so it must not return the
	// excluded exercise even though that exercise has no review row yet.
	u := func() map[string]any {
		_, ex := e.do("GET", "/api/exercises/next?type=vocab&unseen=1&exclude="+jsonNum(a["id"]), nil)
		return ex
	}()
	if u["type"] != "vocab" {
		t.Errorf("unseen=1 should return a vocabulary word: %v", u)
	}
	if exID(u) == exID(a) {
		t.Error("unseen=1 should not return the excluded exercise")
	}
}

func jsonNum(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestSubmitAnswerEvaluatesStoresAndShowsInHistory(t *testing.T) {
	e := newEnv(t, 60)
	ex := e.exercise("vocab")
	code, res := e.do("POST", "/api/attempts", map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "  To give resources.  "})
	if code != 200 || res["status"] != "evaluated" || res["reused"] != false {
		t.Fatalf("submit: %d %v", code, res)
	}
	evaluation := res["evaluation"].(map[string]any)
	if evaluation["verdict"] != "mostly_correct" || evaluation["improved_version"] != "better" {
		t.Errorf("evaluation: %v", evaluation)
	}
	if _, leaked := evaluation["Raw"]; leaked {
		t.Error("audit fields must not reach the client")
	}
	if e.ev.calls != 1 {
		t.Fatalf("expected exactly one AI call, got %d", e.ev.calls)
	}
	// The evaluator received the reference meaning from the database, not from the client.
	if e.ev.last.IntendedMeaning == "" || e.ev.last.Answer != "To give resources." {
		t.Errorf("evaluation request: %+v", e.ev.last)
	}
	if !strings.Contains(e.ev.last.Prompt, "Context:") {
		t.Errorf("vocab prompt should include the context sentence: %q", e.ev.last.Prompt)
	}

	code, detail := e.do("GET", "/api/attempts/"+id1, nil)
	if code != 200 || detail["answer"] != "To give resources." || detail["reference_meaning"] == "" {
		t.Errorf("detail: %d %v", code, detail)
	}
	code, list := e.do("GET", "/api/attempts", nil)
	items := list["items"].([]any)
	if code != 200 || len(items) != 1 || list["total"].(float64) != 1 {
		t.Errorf("history: %d %v", code, list)
	}
	code, st := e.do("GET", "/api/stats?tz=Europe/Athens", nil)
	totals := st["totals"].(map[string]any)
	if code != 200 || totals["first_attempts"].(float64) != 1 || totals["evaluated"].(float64) != 1 {
		t.Errorf("stats: %d %v", code, st)
	}
}

func TestDuplicateSubmitDoesNotEvaluateTwice(t *testing.T) {
	e := newEnv(t, 60)
	ex := e.exercise("writing")
	body := map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "I think so."}
	e.do("POST", "/api/attempts", body)
	code, res := e.do("POST", "/api/attempts", body)
	if code != 200 || res["reused"] != true {
		t.Fatalf("second submit should reuse the stored result: %d %v", code, res)
	}
	if e.ev.calls != 1 {
		t.Errorf("a duplicate submit must not call the AI again, calls=%d", e.ev.calls)
	}
	if code, res := e.do("POST", "/api/attempts", map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "Something else."}); code != 409 || res["error"] != "id_conflict" {
		t.Errorf("same id with another answer: %d %v", code, res)
	}
}

func TestProviderFailureKeepsAnswerAndRetrySucceeds(t *testing.T) {
	e := newEnv(t, 60)
	ex := e.exercise("writing")
	body := map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "My careful answer."}

	e.ev.fn = func(eval.Request) (*eval.Evaluation, error) { return nil, eval.ErrProvider }
	code, res := e.do("POST", "/api/attempts", body)
	if code != 503 || res["error"] != "evaluation_unavailable" || res["retryable"] != true || res["attempt_id"] != id1 {
		t.Fatalf("provider failure: %d %v", code, res)
	}
	// The answer must be saved even though evaluation failed.
	if code, d := e.do("GET", "/api/attempts/"+id1, nil); code != 200 || d["status"] != "failed" || d["answer"] != "My careful answer." {
		t.Fatalf("failed attempt should be stored: %d %v", code, d)
	}
	// Failed attempts are excluded from quality stats.
	if _, st := e.do("GET", "/api/stats", nil); st["first_attempts_evaluated"].(float64) != 0 {
		t.Errorf("failed attempt counted as evaluated: %v", st)
	}

	e.ev.fn = nil
	code, res = e.do("POST", "/api/attempts", body)
	if code != 200 || res["status"] != "evaluated" || res["reused"] != false {
		t.Fatalf("retry: %d %v", code, res)
	}
	_, list := e.do("GET", "/api/attempts", nil)
	if list["total"].(float64) != 1 {
		t.Errorf("a retry must not create a second attempt, total=%v", list["total"])
	}
}

func TestFailureKindsMapToStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"rate limited", &eval.RateLimitError{Quota: "RequestsPerMinute"}, 429, "rate_limited"},
		{"daily quota", &eval.RateLimitError{Quota: "RequestsPerDay-FreeTier"}, 429, "rate_limited"},
		{"invalid output", eval.ErrInvalidOutput, 503, "evaluation_unavailable"},
		{"timeout", context.DeadlineExceeded, 503, "evaluation_unavailable"},
		{"unknown error", errors.New("boom"), 503, "evaluation_unavailable"},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, 60)
			ex := e.exercise("writing")
			e.ev.fn = func(eval.Request) (*eval.Evaluation, error) { return nil, c.err }
			id := "33333333-3333-4333-8333-33333333333" + string(rune('0'+i))
			code, res := e.do("POST", "/api/attempts", map[string]any{"id": id, "exercise_id": ex["id"], "answer": "x"})
			if code != c.want || res["error"] != c.code {
				t.Fatalf("%d %v", code, res)
			}
			if strings.Contains(c.name, "daily") && !strings.Contains(res["message"].(string), "daily quota") {
				t.Errorf("daily quota message: %v", res["message"])
			}
		})
	}
}

func TestRewriteIsLinkedAndMustMatchExercise(t *testing.T) {
	e := newEnv(t, 60)
	ex := e.exercise("writing")
	e.do("POST", "/api/attempts", map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "First try."})

	code, res := e.do("POST", "/api/attempts", map[string]any{"id": id2, "exercise_id": ex["id"], "answer": "Second try.", "parent_attempt_id": id1})
	if code != 200 {
		t.Fatalf("rewrite: %d %v", code, res)
	}
	_, d := e.do("GET", "/api/attempts/"+id2, nil)
	if d["parent_attempt_id"] != id1 {
		t.Errorf("rewrite should reference its parent: %v", d)
	}
	_, st := e.do("GET", "/api/stats", nil)
	totals := st["totals"].(map[string]any)
	if totals["first_attempts"].(float64) != 1 || totals["rewrites"].(float64) != 1 {
		t.Errorf("a rewrite must not count as a first attempt: %v", totals)
	}

	other := e.exercise("vocab")
	code, res = e.do("POST", "/api/attempts", map[string]any{"id": "44444444-4444-4444-8444-444444444444", "exercise_id": other["id"], "answer": "x", "parent_attempt_id": id1})
	if code != 400 || res["error"] != "invalid_parent" {
		t.Errorf("parent from another exercise: %d %v", code, res)
	}
	code, res = e.do("POST", "/api/attempts", map[string]any{"id": "55555555-5555-4555-8555-555555555555", "exercise_id": ex["id"], "answer": "x", "parent_attempt_id": "66666666-6666-4666-8666-666666666666"})
	if code != 400 {
		t.Errorf("unknown parent: %d %v", code, res)
	}
}

func TestInputValidation(t *testing.T) {
	e := newEnv(t, 60)
	ex := e.exercise("writing")
	valid := func() map[string]any { return map[string]any{"id": id1, "exercise_id": ex["id"], "answer": "ok"} }
	mutate := func(f func(m map[string]any)) map[string]any { m := valid(); f(m); return m }

	tests := []struct {
		name string
		body any
		want int
	}{
		{"not json", "nope", 400},
		{"bad id", mutate(func(m map[string]any) { m["id"] = "abc" }), 400},
		{"missing exercise", mutate(func(m map[string]any) { m["exercise_id"] = 0 }), 400},
		{"unknown exercise", mutate(func(m map[string]any) { m["exercise_id"] = 999999 }), 404},
		{"empty answer", mutate(func(m map[string]any) { m["answer"] = "   " }), 400},
		{"answer too long", mutate(func(m map[string]any) { m["answer"] = strings.Repeat("a", 1001) }), 400},
		{"answer with NUL", mutate(func(m map[string]any) { m["answer"] = "a\u0000b" }), 400},
		{"body too large", map[string]any{"id": id1, "exercise_id": ex["id"], "answer": strings.Repeat("a", 20000)}, 400},
		{"bad parent id", mutate(func(m map[string]any) { m["parent_attempt_id"] = "zzz" }), 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code, res := e.do("POST", "/api/attempts", tc.body); code != tc.want {
				t.Errorf("want %d, got %d %v", tc.want, code, res)
			}
		})
	}
	if e.ev.calls != 0 {
		t.Errorf("invalid requests must never reach the AI, calls=%d", e.ev.calls)
	}
	// Exactly 1000 characters is allowed.
	m := valid()
	m["answer"] = strings.Repeat("a", 1000)
	if code, res := e.do("POST", "/api/attempts", m); code != 200 {
		t.Errorf("1000 characters should be accepted: %d %v", code, res)
	}
}

func TestLocalRateLimitProtectsProviderQuota(t *testing.T) {
	e := newEnv(t, 3) // burst of 3, then about one per 20 seconds
	ex := e.exercise("writing")
	submit := func(n int) (int, map[string]any) {
		id := "77777777-7777-4777-8777-77777777777" + string(rune('0'+n))
		return e.do("POST", "/api/attempts", map[string]any{"id": id, "exercise_id": ex["id"], "answer": "x"})
	}
	for i := 0; i < 3; i++ {
		if code, res := submit(i); code != 200 {
			t.Fatalf("request %d: %d %v", i, code, res)
		}
	}
	code, res := submit(3)
	if code != 429 || res["retryable"] != true {
		t.Fatalf("4th request in a burst should be limited: %d %v", code, res)
	}
	if e.ev.calls != 3 {
		t.Errorf("a limited request must not reach the AI, calls=%d", e.ev.calls)
	}
	// The limited answer is still saved and can be retried later.
	if code, d := e.do("GET", "/api/attempts/77777777-7777-4777-8777-777777777773", nil); code != 200 || d["status"] != "failed" {
		t.Errorf("limited attempt should be stored as failed: %d %v", code, d)
	}
}

func TestHistoryAndStatsParameters(t *testing.T) {
	e := newEnv(t, 60)
	for _, p := range []string{"/api/attempts?limit=0", "/api/attempts?limit=1000", "/api/attempts?offset=-1", "/api/attempts?limit=x"} {
		if code, _ := e.do("GET", p, nil); code != 400 {
			t.Errorf("%s should be 400, got %d", p, code)
		}
	}
	if code, _ := e.do("GET", "/api/stats?tz=Not/AZone", nil); code != 400 {
		t.Errorf("invalid time zone should be 400, got %d", code)
	}
	if code, res := e.do("GET", "/api/attempts/not-a-uuid", nil); code != 400 {
		t.Errorf("bad attempt id: %d %v", code, res)
	}
	if code, _ := e.do("GET", "/api/attempts/99999999-9999-4999-8999-999999999999", nil); code != 404 {
		t.Errorf("unknown attempt should be 404, got %d", code)
	}
	// An empty history is valid and returns empty lists, not null.
	code, res := e.do("GET", "/api/stats", nil)
	if code != 200 || res["by_type"] == nil || res["top_categories"] == nil || res["recent"] == nil {
		t.Errorf("empty stats should still have list fields: %d %v", code, res)
	}
}
