package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed seed/exercises.json
var seedJSON []byte

type seedExercise struct {
	Slug            string   `json:"slug"`
	Type            string   `json:"type"`
	Context         string   `json:"context"`
	Prompt          string   `json:"prompt"`
	TargetWord      string   `json:"target_word"`
	IntendedMeaning string   `json:"intended_meaning"`
	ExpectedLength  string   `json:"expected_length"`
	Criteria        []string `json:"criteria"`
	Topic           string   `json:"topic"`
	CEFR            string   `json:"cefr"`
	Tier            int      `json:"tier"`   // 1 is the most useful vocabulary for the goal
	Source          string   `json:"source"` // provenance, for example original or generated
}

var validCEFR = map[string]bool{"A2": true, "B1": true, "B2": true, "C1": true}

func loadSeed() ([]seedExercise, error) {
	return parseSeed(seedJSON, "seed")
}

// parseSeed validates a seed document. Source is a label for error messages,
// so a broken extra file points at the file that caused it.
func parseSeed(data []byte, source string) ([]seedExercise, error) {
	var items []seedExercise
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	seen := map[string]bool{}
	for i := range items {
		it := &items[i]
		if it.Slug == "" || seen[it.Slug] {
			return nil, fmt.Errorf("%s: missing or duplicate slug %q", source, it.Slug)
		}
		seen[it.Slug] = true
		if !validCEFR[it.CEFR] || it.Topic == "" {
			return nil, fmt.Errorf("%s %s: invalid cefr or topic", source, it.Slug)
		}
		switch it.Type {
		case "vocab":
			if it.TargetWord == "" || it.IntendedMeaning == "" || it.Context == "" {
				return nil, fmt.Errorf("%s %s: vocab needs target_word, intended_meaning and context", source, it.Slug)
			}
			if !strings.Contains(strings.ToLower(it.Context), strings.ToLower(it.TargetWord)) {
				return nil, fmt.Errorf("%s %s: context does not contain the target word", source, it.Slug)
			}
			if it.Prompt == "" {
				it.Prompt = fmt.Sprintf("Explain what %q means in this sentence, using your own words.", it.TargetWord)
			}
		case "writing":
			if it.Prompt == "" {
				return nil, fmt.Errorf("%s %s: writing needs a prompt", source, it.Slug)
			}
		default:
			return nil, fmt.Errorf("%s %s: unknown type %q", source, it.Slug, it.Type)
		}
		if it.Criteria == nil {
			it.Criteria = []string{}
		}
		if it.Tier == 0 {
			it.Tier = 2
		}
		if it.Tier < 1 || it.Tier > 4 {
			return nil, fmt.Errorf("%s %s: tier must be between 1 and 4", source, it.Slug)
		}
		if it.Source == "" {
			it.Source = "original"
		}
	}
	return items, nil
}

// Seed inserts the bundled exercises. It is idempotent: an exercise whose slug
// already exists keeps its text, so editing the seed file never rewrites
// content that learners may already have attempts on. Its tier and source may
// still be corrected, because those are curriculum metadata. It returns how
// many new exercises were inserted.
func (s *Store) Seed(ctx context.Context) (int, error) {
	items, err := loadSeed()
	if err != nil {
		return 0, err
	}
	return s.seedItems(ctx, items)
}

// SeedFile loads an extra JSON batch from disk, using the same format and
// validation as the bundled seed. It lets content grow by adding a file and
// restarting the API, without rebuilding the image.
func (s *Store) SeedFile(ctx context.Context, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	items, err := parseSeed(data, path)
	if err != nil {
		return 0, err
	}
	return s.seedItems(ctx, items)
}

func (s *Store) seedItems(ctx context.Context, items []seedExercise) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	inserted := 0
	for _, it := range items {
		criteria, _ := json.Marshal(it.Criteria)
		res, err := tx.ExecContext(ctx, `
INSERT INTO exercises (slug, type, context, prompt, target_word, intended_meaning, expected_length, criteria, topic, cefr, tier, source)
VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8::jsonb, $9, $10, $11, $12)
ON CONFLICT (slug) DO NOTHING`,
			it.Slug, it.Type, it.Context, it.Prompt, it.TargetWord, it.IntendedMeaning,
			it.ExpectedLength, string(criteria), it.Topic, it.CEFR, it.Tier, it.Source)
		if err != nil {
			return 0, fmt.Errorf("seed %s: %w", it.Slug, err)
		}
		n, _ := res.RowsAffected()
		inserted += int(n)

		// Tier and provenance are curriculum metadata, not learner-visible
		// content, so they may be corrected for an exercise that already
		// exists. The exercise text itself is never rewritten.
		if _, err := tx.ExecContext(ctx, `
UPDATE exercises SET tier = $2, source = $3
WHERE slug = $1 AND (tier <> $2 OR source <> $3)`,
			it.Slug, it.Tier, it.Source); err != nil {
			return 0, fmt.Errorf("seed %s: %w", it.Slug, err)
		}
	}
	return inserted, tx.Commit()
}
