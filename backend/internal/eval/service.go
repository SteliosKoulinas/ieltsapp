package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// At most 2 provider calls per submission: either one retry after a provider
// error (5xx, timeout), or one repair call after invalid output.
// A rate limit (429) is NOT retried: free-tier limits are per minute, so an
// immediate retry only fails again. The caller tells the user to wait.
const maxCalls = 2

var retryDelay = 3 * time.Second

type LLM interface {
	Generate(ctx context.Context, system, user string) (string, error)
}

type Service struct{ llm LLM }

func NewService(llm LLM) *Service { return &Service{llm: llm} }

func (s *Service) Evaluate(ctx context.Context, req Request) (*Evaluation, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	system := SystemPromptFor(req.Type)
	user := BuildUserPrompt(req)
	var lastErr error

	for i := 0; i < maxCalls; i++ {
		raw, err := s.llm.Generate(ctx, system, user)
		if err != nil {
			if errors.Is(err, ErrProvider) && i < maxCalls-1 {
				select {
				case <-time.After(retryDelay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		ev, verr := parse(raw, req.Type)
		if verr == nil {
			n := ev.DropUnverifiableIssues(req.Answer)
			if n > 0 {
				slog.Warn("dropped issues quoting text the learner did not write", "count", n)
			}
			// For listening, the reference transcript is the answer to show.
			// The verdict and issues still come from the model; this only keeps
			// the displayed transcript identical to what was actually spoken.
			if req.Type == TypeListening && strings.TrimSpace(req.ExpectedText) != "" {
				ev.ImprovedVersion = req.ExpectedText
			}
			ev.Raw, ev.Calls, ev.Dropped = raw, i+1, n
			return ev, nil
		}
		lastErr = verr
		user = BuildUserPrompt(req) + "\nYour previous response was invalid: " + verr.Error() +
			". Return a corrected JSON object only."
	}
	return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, lastErr)
}

func parse(raw, exerciseType string) (*Evaluation, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")

	var ev Evaluation
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ev); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	if err := ev.Validate(exerciseType); err != nil {
		return nil, err
	}
	return &ev, nil
}
