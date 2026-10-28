// evalcheck runs a golden set of exercises and learner answers through the REAL
// evaluation service (live Gemini calls) and compares each result with soft
// expectations. LLM output varies, so a mismatch is a signal to read the
// feedback and possibly adjust the prompt, not an automatic failure.
//
// Usage (from the backend folder):
//
//	go run ./cmd/evalcheck            run all cases
//	go run ./cmd/evalcheck -v         also print summary, issues and improved version
//	go run ./cmd/evalcheck -only w    run cases whose id starts with "w" (the writing ones)
//	go run ./cmd/evalcheck -only v1,w3   several prefixes, comma separated
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"ielts/backend/internal/config"
	"ielts/backend/internal/eval"
)

type expectation struct {
	Verdicts             []string `json:"verdicts"`
	IncludeAnyCategory   []string `json:"include_any_category"`
	IncludeAllCategories []string `json:"include_all_categories"`
	MaxErrorIssues       *int     `json:"max_error_issues"`
}

type testCase struct {
	ID string `json:"id"`
	eval.Request
	Expect expectation `json:"expect"`
}

func main() {
	file := flag.String("file", "testdata/golden.json", "golden cases file")
	verbose := flag.Bool("v", false, "print full feedback")
	only := flag.String("only", "", "run only cases whose id starts with one of these comma separated prefixes")
	delay := flag.Duration("delay", 6*time.Second, "pause between cases, to stay under free-tier rate limits (use 15s or more for models limited to 5 requests per minute)")
	wait := flag.Duration("retry-wait", 45*time.Second, "wait before retrying a case that hit a rate limit or provider error")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal(err)
	}
	var cases []testCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		log.Fatalf("reading %s: %v", *file, err)
	}

	svc := eval.NewService(eval.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL))
	fmt.Printf("model: %s | cases: %d | pause: %s\n", cfg.GeminiModel, len(cases), *delay)

	var ran, matched, failed int
	var latencies []time.Duration
	for _, tc := range cases {
		if !selected(tc.ID, *only) {
			continue
		}
		if ran > 0 {
			time.Sleep(*delay)
		}
		ran++

		start := time.Now()
		ev, err := evaluate(svc, tc.Request)
		took := time.Since(start)
		if isDailyQuota(err) {
			fmt.Printf("\n[STOP ] %s: %v\n        A per-day quota does not recover in a minute. Stopping here.\n", tc.ID, err)
			failed++
			break
		}
		if transient(err) {
			fmt.Printf("\n[WAIT ] %s: %v. Waiting %s, then trying once more\n", tc.ID, err, *wait)
			time.Sleep(*wait)
			start = time.Now()
			ev, err = evaluate(svc, tc.Request)
			took = time.Since(start)
		}
		if err != nil {
			failed++
			fmt.Printf("\n[ERROR] %s: %v\n", tc.ID, err)
			continue
		}

		latencies = append(latencies, took)
		problems := check(tc.Expect, ev)
		status := "OK   "
		if len(problems) > 0 {
			status = "CHECK"
		} else {
			matched++
		}
		fmt.Printf("\n[%s] %s -> %s (confidence %s, %.1fs) categories: %s\n",
			status, tc.ID, ev.Verdict, ev.Confidence, took.Seconds(), categoriesOf(ev))
		fmt.Printf("        answer: %q\n", tc.Answer)
		for _, p := range problems {
			fmt.Printf("        expectation not met: %s\n", p)
		}
		if *verbose || len(problems) > 0 {
			printFeedback(ev)
		}
	}
	fmt.Printf("\n%d run, %d matched expectations, %d need a look, %d errors\n",
		ran, matched, ran-matched-failed, failed)
	if len(latencies) > 0 {
		avg, max := summarize(latencies)
		fmt.Printf("latency per call (includes the provider's own retries): average %.1fs, slowest %.1fs, over %d answered cases\n",
			avg.Seconds(), max.Seconds(), len(latencies))
	}
}

func evaluate(svc *eval.Service, req eval.Request) (*eval.Evaluation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return svc.Evaluate(ctx, req)
}

// transient reports errors worth waiting out: a rate limit or a provider
// hiccup (for example a temporary 503 "high demand").
func transient(err error) bool {
	return errors.Is(err, eval.ErrRateLimited) || errors.Is(err, eval.ErrProvider)
}

func summarize(ds []time.Duration) (avg, max time.Duration) {
	var total time.Duration
	for _, d := range ds {
		total += d
		if d > max {
			max = d
		}
	}
	return total / time.Duration(len(ds)), max
}

func isDailyQuota(err error) bool {
	var rl *eval.RateLimitError
	return errors.As(err, &rl) && rl.Daily()
}

// selected reports whether a case id matches one of the comma separated
// prefixes in only. An empty only selects everything.
func selected(id, only string) bool {
	if strings.TrimSpace(only) == "" {
		return true
	}
	for _, p := range strings.Split(only, ",") {
		if p = strings.TrimSpace(p); p != "" && strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

func check(e expectation, ev *eval.Evaluation) []string {
	var problems []string
	if len(e.Verdicts) > 0 && !slices.Contains(e.Verdicts, ev.Verdict) {
		problems = append(problems, fmt.Sprintf("verdict %q not in %v", ev.Verdict, e.Verdicts))
	}
	if len(e.IncludeAnyCategory) > 0 {
		found := false
		for _, is := range ev.Issues {
			if slices.Contains(e.IncludeAnyCategory, is.Category) {
				found = true
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf("none of the categories %v were reported", e.IncludeAnyCategory))
		}
	}
	for _, want := range e.IncludeAllCategories {
		found := false
		for _, is := range ev.Issues {
			if is.Category == want {
				found = true
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf("category %q was not reported", want))
		}
	}
	if e.MaxErrorIssues != nil {
		n := 0
		for _, is := range ev.Issues {
			if is.Severity == "error" {
				n++
			}
		}
		if n > *e.MaxErrorIssues {
			problems = append(problems, fmt.Sprintf("%d error issues, expected at most %d", n, *e.MaxErrorIssues))
		}
	}
	return problems
}

func categoriesOf(ev *eval.Evaluation) string {
	if len(ev.Issues) == 0 {
		return "none"
	}
	var parts []string
	for _, is := range ev.Issues {
		parts = append(parts, is.Category+"/"+is.Severity)
	}
	return strings.Join(parts, ", ")
}

func printFeedback(ev *eval.Evaluation) {
	fmt.Printf("        summary: %s\n", ev.Summary)
	for _, is := range ev.Issues {
		fmt.Printf("        issue [%s/%s] %q -> %q: %s\n", is.Category, is.Severity, is.Original, is.Correction, is.Explanation)
	}
	fmt.Printf("        improved: %s\n", ev.ImprovedVersion)
}
