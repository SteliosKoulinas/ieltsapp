package eval

import (
	"fmt"
	"strings"
	"unicode"
)

var verdicts = map[string]bool{"correct": true, "mostly_correct": true, "partially_correct": true, "incorrect": true}
var confidences = map[string]bool{"high": true, "medium": true, "low": true}
var severities = map[string]bool{"error": true, "unnatural": true, "style": true}

// The fixed category list. In Phase B this becomes the concepts table.
var categories = map[string]bool{
	"articles": true, "verb_tense": true, "subject_verb_agreement": true,
	"prepositions": true, "missing_subject": true, "word_choice": true,
	"collocations": true, "sentence_structure": true, "linking": true, "clarity": true,
	"vocabulary_meaning": true, "noun_number": true,
	"listening_spelling": true, "listening_number": true, "listening_word": true,
	"listening_omission": true, "listening_extra": true,
}

// Validate checks the evaluation against the contract. exerciseType matters for
// one rule: in a vocabulary exercise the verdict reflects meaning only, so a
// "correct" verdict may still list language errors. In writing the verdict
// includes language accuracy, so "correct" with an error issue is contradictory.
func (e *Evaluation) Validate(exerciseType string) error {
	if !verdicts[e.Verdict] {
		return fmt.Errorf("unknown verdict %q", e.Verdict)
	}
	if !confidences[e.Confidence] {
		return fmt.Errorf("unknown confidence %q", e.Confidence)
	}
	if e.Summary == "" || e.ImprovedVersion == "" {
		return fmt.Errorf("summary and improved_version are required")
	}
	for i, is := range e.Issues {
		if !categories[is.Category] {
			return fmt.Errorf("issue %d: unknown category %q", i, is.Category)
		}
		if !severities[is.Severity] {
			return fmt.Errorf("issue %d: unknown severity %q", i, is.Severity)
		}
		if is.Explanation == "" {
			return fmt.Errorf("issue %d: explanation is required", i)
		}
		if (exerciseType == TypeWriting || exerciseType == TypeListening) && e.Verdict == "correct" && is.Severity == "error" {
			return fmt.Errorf("verdict is correct but issue %d is an error", i)
		}
	}
	return nil
}

// DropUnverifiableIssues removes issues whose quoted "original" text does not
// appear in the learner's answer. A quote the learner never wrote is a sign
// of a hallucinated correction. An empty original is allowed (it means the
// issue is about the answer as a whole). It returns how many issues were
// dropped. If any were dropped, a "high" confidence is lowered to "medium".
func (e *Evaluation) DropUnverifiableIssues(answer string) int {
	norm := normalize(answer)
	kept := e.Issues[:0]
	dropped := 0
	for _, is := range e.Issues {
		o := normalize(is.Original)
		if o == "" || strings.Contains(norm, o) {
			kept = append(kept, is)
		} else {
			dropped++
		}
	}
	e.Issues = kept
	if dropped > 0 && e.Confidence == "high" {
		e.Confidence = "medium"
	}
	return dropped
}

// normalize lowercases, unifies apostrophes, and turns every non letter or
// digit into a space, so quotes match despite spacing or punctuation changes.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\u2019", "'")
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// VerdictScore maps a verdict to a number so results can be compared over time:
// incorrect 0, partially_correct 1, mostly_correct 2, correct 3. It is a
// practice-tracking convenience, not an IELTS band score.
func VerdictScore(verdict string) int {
	switch verdict {
	case "correct":
		return 3
	case "mostly_correct":
		return 2
	case "partially_correct":
		return 1
	default:
		return 0
	}
}
