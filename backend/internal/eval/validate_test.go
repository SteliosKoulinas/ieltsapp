package eval

import "testing"

func TestValidate(t *testing.T) {
	valid := func() Evaluation {
		return Evaluation{
			Verdict: "partially_correct", Summary: "s", ImprovedVersion: "x", Confidence: "high",
			Issues: []Issue{{Category: "articles", Severity: "error", Original: "a", Correction: "the", Explanation: "e"}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*Evaluation)
		wantErr bool
	}{
		{"valid", func(e *Evaluation) {}, false},
		{"bad verdict", func(e *Evaluation) { e.Verdict = "great" }, true},
		{"bad confidence", func(e *Evaluation) { e.Confidence = "sure" }, true},
		{"empty summary", func(e *Evaluation) { e.Summary = "" }, true},
		{"unknown category", func(e *Evaluation) { e.Issues[0].Category = "grammar" }, true},
		{"unknown severity", func(e *Evaluation) { e.Issues[0].Severity = "bad" }, true},
		{"empty explanation", func(e *Evaluation) { e.Issues[0].Explanation = "" }, true},
		{"correct but has error issue", func(e *Evaluation) { e.Verdict = "correct" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := valid()
			tc.mutate(&e)
			if err := e.Validate(TypeWriting); (err != nil) != tc.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestDropUnverifiableIssues(t *testing.T) {
	answer := "I think technology is good. Is very easy to send message."
	ev := Evaluation{
		Verdict: "partially_correct", Summary: "s", ImprovedVersion: "x", Confidence: "high",
		Issues: []Issue{
			{Category: "missing_subject", Severity: "error", Original: "Is very easy to send message", Explanation: "e"},
			{Category: "articles", Severity: "error", Original: "send a message", Explanation: "quote the learner never wrote"},
			{Category: "clarity", Severity: "error", Original: "", Explanation: "about the whole answer"},
			{Category: "articles", Severity: "error", Original: "  SEND   message. ", Explanation: "matches after normalizing"},
		},
	}
	dropped := ev.DropUnverifiableIssues(answer)
	if dropped != 1 || len(ev.Issues) != 3 {
		t.Fatalf("dropped=%d kept=%d", dropped, len(ev.Issues))
	}
	if ev.Confidence != "medium" {
		t.Errorf("confidence should drop to medium, got %s", ev.Confidence)
	}
}

func TestDropUnverifiableKeepsConfidenceWhenNothingDropped(t *testing.T) {
	ev := Evaluation{Verdict: "correct", Summary: "s", ImprovedVersion: "x", Confidence: "high"}
	if n := ev.DropUnverifiableIssues("anything"); n != 0 || ev.Confidence != "high" {
		t.Fatalf("n=%d conf=%s", n, ev.Confidence)
	}
}

func TestValidateCorrectVerdictWithErrorsDependsOnType(t *testing.T) {
	ev := Evaluation{
		Verdict: "correct", Summary: "s", ImprovedVersion: "x", Confidence: "high",
		Issues: []Issue{{Category: "subject_verb_agreement", Severity: "error", Original: "mean", Correction: "means", Explanation: "e"}},
	}
	if err := ev.Validate(TypeVocab); err != nil {
		t.Errorf("vocab: correct meaning with a grammar error must be allowed, got %v", err)
	}
	if err := ev.Validate(TypeWriting); err == nil {
		t.Error("writing: correct verdict with an error issue must be rejected")
	}
	if err := ev.Validate(TypeListening); err == nil {
		t.Error("listening: correct verdict with an error issue must be rejected")
	}
}
