package eval

import (
	"strings"
	"testing"
)

func TestSystemPromptFor(t *testing.T) {
	if !strings.Contains(SystemPromptFor(TypeVocab), "VOCABULARY exercise") {
		t.Error("vocab prompt missing vocab rules")
	}
	if strings.Contains(SystemPromptFor(TypeVocab), "SHORT WRITING exercise") {
		t.Error("vocab prompt must not contain writing rules")
	}
	if !strings.Contains(SystemPromptFor(TypeWriting), "Do not penalize a concise answer") {
		t.Error("writing prompt missing concise-answer rule")
	}
	if !strings.Contains(SystemPromptFor(TypeListening), "LISTENING DICTATION exercise") {
		t.Error("listening prompt missing listening rules")
	}
	if strings.Contains(SystemPromptFor(TypeWriting), "LISTENING DICTATION") {
		t.Error("writing prompt must not contain listening rules")
	}
	for _, c := range []string{"vocabulary_meaning", "missing_subject", "articles", "noun_number", "listening_spelling", "listening_number"} {
		if !strings.Contains(SystemPromptFor(TypeWriting), c) {
			t.Errorf("prompt missing category %s", c)
		}
	}
}

func TestBuildUserPrompt(t *testing.T) {
	v := BuildUserPrompt(Request{Type: TypeVocab, Prompt: "P", TargetWord: "allocate", IntendedMeaning: "assign", Answer: "A"})
	for _, want := range []string{"Target word: allocate", "Intended meaning in this context: assign", "<<<\nA\n>>>"} {
		if !strings.Contains(v, want) {
			t.Errorf("vocab user prompt missing %q", want)
		}
	}
	w := BuildUserPrompt(Request{Type: TypeWriting, Prompt: "P", ExpectedLength: "2 to 3 sentences", Criteria: []string{"opinion", "reason"}, Answer: "A"})
	for _, want := range []string{"Expected length: 2 to 3 sentences", "Assessment criteria: opinion; reason"} {
		if !strings.Contains(w, want) {
			t.Errorf("writing user prompt missing %q", want)
		}
	}
	if strings.Contains(w, "Target word") {
		t.Error("writing prompt must not mention target word")
	}
	l := BuildUserPrompt(Request{Type: TypeListening, Prompt: "P", ListeningMode: "numeric",
		ExpectedText: "020 7946 0123", AcceptedForms: []string{"02079460123"}, Answer: "020 7946 0123"})
	for _, want := range []string{"Listening mode: numeric", "Reference transcript: 020 7946 0123", "Other accepted forms: 02079460123"} {
		if !strings.Contains(l, want) {
			t.Errorf("listening user prompt missing %q", want)
		}
	}
}

func TestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		r       Request
		wantErr bool
	}{
		{"vocab ok", Request{Type: TypeVocab, Prompt: "p", TargetWord: "w", IntendedMeaning: "m", Answer: "a"}, false},
		{"vocab missing meaning", Request{Type: TypeVocab, Prompt: "p", TargetWord: "w", Answer: "a"}, true},
		{"writing ok", Request{Type: TypeWriting, Prompt: "p", Answer: "a"}, false},
		{"listening ok", Request{Type: TypeListening, Prompt: "p", ExpectedText: "e", ListeningMode: "text", Answer: "a"}, false},
		{"listening missing transcript", Request{Type: TypeListening, Prompt: "p", ListeningMode: "text", Answer: "a"}, true},
		{"listening bad mode", Request{Type: TypeListening, Prompt: "p", ExpectedText: "e", ListeningMode: "sound", Answer: "a"}, true},
		{"unknown type", Request{Type: "grammar", Prompt: "p", Answer: "a"}, true},
		{"empty answer", Request{Type: TypeWriting, Prompt: "p", Answer: " "}, true},
		{"empty prompt", Request{Type: TypeWriting, Answer: "a"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.r.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
