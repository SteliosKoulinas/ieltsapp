package store

import (
	"strings"
	"testing"
)

// The seed file is content, so these checks guard its structure.
func TestSeedFileIsValid(t *testing.T) {
	items, err := loadSeed()
	if err != nil {
		t.Fatal(err)
	}
	vocab, writing := 0, 0
	for _, it := range items {
		switch it.Type {
		case "vocab":
			vocab++
			if !strings.Contains(it.Prompt, it.TargetWord) {
				t.Errorf("%s: generated prompt should mention the target word", it.Slug)
			}
		case "writing":
			writing++
			if it.ExpectedLength == "" || len(it.Criteria) == 0 {
				t.Errorf("%s: writing needs expected_length and criteria", it.Slug)
			}
		}
		if strings.Contains(it.Context+it.Prompt+it.IntendedMeaning, "\u2014") {
			t.Errorf("%s: contains an em dash", it.Slug)
		}
	}
	if vocab < 15 || writing < 10 {
		t.Fatalf("expected a usable bank, got %d vocab and %d writing", vocab, writing)
	}
}
