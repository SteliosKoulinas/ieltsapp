package eval

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrRateLimited    = errors.New("provider rate limited")
	ErrProvider       = errors.New("provider error")
	ErrInvalidOutput  = errors.New("invalid evaluation output")
	ErrInvalidRequest = errors.New("invalid evaluation request")
)

// RateLimitError is a 429 from the provider, with whatever the response said
// about WHICH quota was hit. A per-minute limit recovers in about a minute; a
// per-day limit does not, so callers should not keep waiting on it.
type RateLimitError struct {
	Quota      string // for example "GenerateRequestsPerDayPerProjectPerModel-FreeTier"
	RetryDelay string // for example "34s"
}

func (e *RateLimitError) Error() string {
	msg := "provider rate limited"
	if e.Quota != "" {
		msg += " (" + e.Quota + ")"
	}
	if e.RetryDelay != "" {
		msg += ", retry after " + e.RetryDelay
	}
	return msg
}

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// Daily reports whether the quota that was hit is a per-day quota.
func (e *RateLimitError) Daily() bool {
	return strings.Contains(strings.ToLower(e.Quota), "perday")
}

const (
	TypeVocab   = "vocab"
	TypeWriting = "writing"
)

// Request is everything the evaluator needs. Nothing else is sent to the LLM.
// In production it is assembled by the backend from the stored exercise plus
// the learner's answer, never taken from the client.
type Request struct {
	Type            string   `json:"type"`
	Prompt          string   `json:"prompt"`
	TargetWord      string   `json:"target_word,omitempty"`      // vocab
	IntendedMeaning string   `json:"intended_meaning,omitempty"` // vocab
	ExpectedLength  string   `json:"expected_length,omitempty"`  // writing
	Criteria        []string `json:"criteria,omitempty"`         // writing
	Answer          string   `json:"answer"`
}

func (r Request) Validate() error {
	switch r.Type {
	case TypeVocab:
		if strings.TrimSpace(r.TargetWord) == "" || strings.TrimSpace(r.IntendedMeaning) == "" {
			return fmt.Errorf("%w: vocab needs target_word and intended_meaning", ErrInvalidRequest)
		}
	case TypeWriting:
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidRequest, r.Type)
	}
	if strings.TrimSpace(r.Prompt) == "" || strings.TrimSpace(r.Answer) == "" {
		return fmt.Errorf("%w: prompt and answer are required", ErrInvalidRequest)
	}
	return nil
}

type Evaluation struct {
	Verdict         string   `json:"verdict"`
	Summary         string   `json:"summary"`
	Strengths       []string `json:"strengths"`
	Issues          []Issue  `json:"issues"`
	ImprovedVersion string   `json:"improved_version"`
	Confidence      string   `json:"confidence"`

	// Audit data. Never sent to the client (json:"-"), stored for inspection.
	Raw     string `json:"-"` // the raw model text that passed validation
	Calls   int    `json:"-"` // provider calls used for this evaluation (1 or 2)
	Dropped int    `json:"-"` // issues removed because their quote was not in the answer
}

type Issue struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Original    string `json:"original"`
	Correction  string `json:"correction"`
	Explanation string `json:"explanation"`
}
