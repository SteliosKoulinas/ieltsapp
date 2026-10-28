package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"ielts/backend/internal/eval"
)

// The golden file is data the live check depends on, so keep it well formed.
func TestGoldenFileIsWellFormed(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []testCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("expected at least 10 cases, got %d", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("missing or duplicate id %q", c.ID)
		}
		seen[c.ID] = true
		if err := c.Request.Validate(); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
		if len(c.Expect.Verdicts) == 0 {
			t.Errorf("%s: no expected verdicts", c.ID)
		}
	}
}

func TestCheck(t *testing.T) {
	zero := 0
	ev := &eval.Evaluation{Verdict: "correct", Issues: []eval.Issue{{Category: "articles", Severity: "error"}}}
	if p := check(expectation{Verdicts: []string{"correct"}}, ev); len(p) != 0 {
		t.Errorf("unexpected problems: %v", p)
	}
	if p := check(expectation{Verdicts: []string{"incorrect"}}, ev); len(p) != 1 {
		t.Errorf("wrong verdict not reported: %v", p)
	}
	if p := check(expectation{IncludeAnyCategory: []string{"prepositions"}}, ev); len(p) != 1 {
		t.Errorf("missing category not reported: %v", p)
	}
	if p := check(expectation{MaxErrorIssues: &zero}, ev); len(p) != 1 {
		t.Errorf("too many errors not reported: %v", p)
	}
}

func TestSelected(t *testing.T) {
	tests := []struct {
		id, only string
		want     bool
	}{
		{"w1-x", "", true},
		{"w1-x", "w", true},
		{"v1-wrong-definition", "w", false}, // "wrong" contains w but the id does not start with it
		{"v1-x", "v1,w3", true},
		{"w3-x", "v1, w3", true},
		{"w2-x", "v1,w3", false},
	}
	for _, tc := range tests {
		if got := selected(tc.id, tc.only); got != tc.want {
			t.Errorf("selected(%q,%q)=%v", tc.id, tc.only, got)
		}
	}
}

func TestSummarize(t *testing.T) {
	avg, max := summarize([]time.Duration{2 * time.Second, 4 * time.Second, 9 * time.Second})
	if avg != 5*time.Second || max != 9*time.Second {
		t.Fatalf("avg=%v max=%v", avg, max)
	}
}

func TestCheckIncludeAll(t *testing.T) {
	ev := &eval.Evaluation{Verdict: "mostly_correct", Issues: []eval.Issue{{Category: "prepositions", Severity: "error"}}}
	if p := check(expectation{IncludeAllCategories: []string{"prepositions", "articles"}}, ev); len(p) != 1 {
		t.Errorf("expected exactly one missing category, got %v", p)
	}
	ev.Issues = append(ev.Issues, eval.Issue{Category: "articles", Severity: "error"})
	if p := check(expectation{IncludeAllCategories: []string{"prepositions", "articles"}}, ev); len(p) != 0 {
		t.Errorf("unexpected problems: %v", p)
	}
}
