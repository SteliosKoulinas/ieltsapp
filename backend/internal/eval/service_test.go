package eval

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeLLM returns scripted responses in order, so the retry policy can be tested
// without calling the real provider.
type fakeLLM struct {
	responses []string
	errs      []error
	calls     int
}

func (f *fakeLLM) Generate(ctx context.Context, system, user string) (string, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	return f.responses[i], nil
}

const goodJSON = `{"verdict":"incorrect","summary":"s","strengths":[],"issues":[],"improved_version":"x","confidence":"high"}`

func TestEvaluate(t *testing.T) {
	retryDelay = time.Millisecond
	req := Request{Type: TypeWriting, Prompt: "p", Answer: "a"}

	t.Run("valid first time", func(t *testing.T) {
		f := &fakeLLM{responses: []string{goodJSON}}
		if _, err := NewService(f).Evaluate(context.Background(), req); err != nil || f.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("invalid then repaired", func(t *testing.T) {
		f := &fakeLLM{responses: []string{"not json", goodJSON}}
		if _, err := NewService(f).Evaluate(context.Background(), req); err != nil || f.calls != 2 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("invalid twice gives ErrInvalidOutput and stops at 2 calls", func(t *testing.T) {
		f := &fakeLLM{responses: []string{"bad", "still bad"}}
		_, err := NewService(f).Evaluate(context.Background(), req)
		if !errors.Is(err, ErrInvalidOutput) || f.calls != 2 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("rate limit is not retried", func(t *testing.T) {
		f := &fakeLLM{errs: []error{ErrRateLimited, nil}, responses: []string{"", goodJSON}}
		_, err := NewService(f).Evaluate(context.Background(), req)
		if !errors.Is(err, ErrRateLimited) || f.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("provider error then success", func(t *testing.T) {
		f := &fakeLLM{errs: []error{ErrProvider, nil}, responses: []string{"", goodJSON}}
		if _, err := NewService(f).Evaluate(context.Background(), req); err != nil || f.calls != 2 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("vocab: correct verdict with a grammar error is accepted", func(t *testing.T) {
		resp := `{"verdict":"correct","summary":"s","strengths":[],"improved_version":"x","confidence":"high",
		"issues":[{"category":"subject_verb_agreement","severity":"error","original":"mean","correction":"means","explanation":"e"}]}`
		f := &fakeLLM{responses: []string{resp}}
		v := Request{Type: TypeVocab, Prompt: "p", TargetWord: "w", IntendedMeaning: "m", Answer: "Allocate mean to give"}
		ev, err := NewService(f).Evaluate(context.Background(), v)
		if err != nil || f.calls != 1 || len(ev.Issues) != 1 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("provider error twice stops at 2 calls", func(t *testing.T) {
		f := &fakeLLM{errs: []error{ErrProvider, ErrProvider}, responses: []string{"", ""}}
		_, err := NewService(f).Evaluate(context.Background(), req)
		if !errors.Is(err, ErrProvider) || f.calls != 2 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
	t.Run("non transient error is not retried", func(t *testing.T) {
		f := &fakeLLM{errs: []error{errors.New("bad request")}, responses: []string{""}}
		_, err := NewService(f).Evaluate(context.Background(), req)
		if err == nil || f.calls != 1 {
			t.Fatalf("err=%v calls=%d", err, f.calls)
		}
	})
}

func TestEvaluateRejectsInvalidRequestWithoutCallingLLM(t *testing.T) {
	f := &fakeLLM{responses: []string{goodJSON}}
	_, err := NewService(f).Evaluate(context.Background(), Request{Type: "nope", Prompt: "p", Answer: "a"})
	if !errors.Is(err, ErrInvalidRequest) || f.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, f.calls)
	}
}

func TestEvaluateDropsHallucinatedQuote(t *testing.T) {
	resp := `{"verdict":"partially_correct","summary":"s","strengths":[],"improved_version":"x","confidence":"high",
	"issues":[{"category":"articles","severity":"error","original":"text not in answer","correction":"c","explanation":"e"}]}`
	f := &fakeLLM{responses: []string{resp}}
	ev, err := NewService(f).Evaluate(context.Background(), Request{Type: TypeWriting, Prompt: "p", Answer: "my real answer"})
	if err != nil || len(ev.Issues) != 0 || ev.Confidence != "medium" {
		t.Fatalf("err=%v ev=%+v", err, ev)
	}
}

func TestEvaluateUsesTypeSpecificPrompt(t *testing.T) {
	var gotSystem string
	llm := llmFunc(func(ctx context.Context, system, user string) (string, error) {
		gotSystem = system
		return goodJSON, nil
	})
	_, err := NewService(llm).Evaluate(context.Background(), Request{Type: TypeVocab, Prompt: "p", TargetWord: "w", IntendedMeaning: "m", Answer: "a"})
	if err != nil || gotSystem != SystemPromptFor(TypeVocab) {
		t.Fatalf("err=%v", err)
	}
}

func TestListeningEvaluationShowsReferenceTranscript(t *testing.T) {
	f := &fakeLLM{responses: []string{goodJSON}}
	req := Request{Type: TypeListening, Prompt: "Type the phone number you hear.", ListeningMode: "numeric",
		ExpectedText: "020 7946 0123", AcceptedForms: []string{"02079460123"}, Answer: "020 7946 0123"}
	ev, err := NewService(f).Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ImprovedVersion != req.ExpectedText {
		t.Errorf("improved version should be the reference transcript, got %q", ev.ImprovedVersion)
	}
}

type llmFunc func(ctx context.Context, system, user string) (string, error)

func (f llmFunc) Generate(ctx context.Context, system, user string) (string, error) {
	return f(ctx, system, user)
}
