package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed seed/exercises.json
var seedJSON []byte

//go:embed seed/listening.json
var listeningSeedJSON []byte

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

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

	// Listening only. AudioText is what the browser speaks. AnswerText is the
	// reference transcript used for evaluation and never sent before submit.
	AudioText     string   `json:"audio_text"`
	AnswerText    string   `json:"answer_text"`
	ListeningMode string   `json:"listening_mode"` // text or numeric
	ListeningKind string   `json:"listening_kind"`
	AcceptedForms []string `json:"accepted_forms"`
}

var validCEFR = map[string]bool{"A2": true, "B1": true, "B2": true, "C1": true}

var validListeningKinds = map[string]map[string]bool{
	"text": {
		"word": true, "phrase": true, "sentence": true,
	},
	"numeric": {
		"phone": true, "address": true, "price": true, "date": true,
		"time": true, "postcode": true, "reference": true, "quantity": true,
		"percentage": true, "distance": true, "weight": true, "duration": true,
		"room": true,
	},
}

func loadSeed() ([]seedExercise, error) {
	return parseSeed(seedJSON, "seed")
}

func loadListeningSeed() ([]seedExercise, error) {
	return parseSeed(listeningSeedJSON, "listening seed")
}

// ValidateSeedJSON checks a batch without inserting it. The generator uses it
// per item, so one bad item does not throw away a whole AI batch.
func ValidateSeedJSON(data []byte, source string) error {
	_, err := parseSeed(data, source)
	return err
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
		if !slugRe.MatchString(it.Slug) {
			return nil, fmt.Errorf("%s: slug %q must be lowercase letters, digits and hyphens", source, it.Slug)
		}
		seen[it.Slug] = true
		if err := validateContentText(it); err != nil {
			return nil, fmt.Errorf("%s %s: %w", source, it.Slug, err)
		}
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
		case "listening":
			if err := validateListeningItem(it); err != nil {
				return nil, fmt.Errorf("%s %s: %w", source, it.Slug, err)
			}
		default:
			return nil, fmt.Errorf("%s %s: unknown type %q", source, it.Slug, it.Type)
		}
		if it.Criteria == nil {
			it.Criteria = []string{}
		}
		if it.AcceptedForms == nil {
			it.AcceptedForms = []string{}
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

// validateContentText enforces two content rules that apply to every item:
// no em dash, and no claim that the material is official IELTS content.
func validateContentText(it *seedExercise) error {
	fields := []string{it.Context, it.Prompt, it.TargetWord, it.IntendedMeaning,
		it.ExpectedLength, it.AudioText, it.AnswerText}
	fields = append(fields, it.Criteria...)
	fields = append(fields, it.AcceptedForms...)
	for _, s := range fields {
		if strings.ContainsRune(s, '\u2014') {
			return fmt.Errorf("text contains an em dash")
		}
		if strings.Contains(strings.ToLower(s), "official ielts") {
			return fmt.Errorf("text must not claim to be official IELTS material")
		}
	}
	return nil
}

func validateListeningItem(it *seedExercise) error {
	if strings.TrimSpace(it.AudioText) == "" {
		return fmt.Errorf("listening needs audio_text")
	}
	kinds, ok := validListeningKinds[it.ListeningMode]
	if !ok {
		return fmt.Errorf("listening_mode must be text or numeric")
	}
	if !kinds[it.ListeningKind] {
		return fmt.Errorf("listening_kind %q is not valid for mode %q", it.ListeningKind, it.ListeningMode)
	}
	if it.ListeningMode == "text" {
		if it.AnswerText == "" {
			it.AnswerText = it.AudioText
		}
		if it.Prompt == "" {
			it.Prompt = "Type the " + it.ListeningKind + " you hear."
		}
		return nil
	}
	if strings.TrimSpace(it.AnswerText) == "" {
		return fmt.Errorf("numeric listening needs answer_text")
	}
	// The browser speaks audio_text directly. Digits and currency symbols are
	// read unpredictably by speech synthesis, so numeric items spell them out.
	if strings.ContainsAny(it.AudioText, "0123456789£$€%") {
		return fmt.Errorf("numeric audio_text must spell out digits and symbols")
	}
	if it.Prompt == "" {
		it.Prompt = numericPrompt(it.ListeningKind)
	}
	return nil
}

func numericPrompt(kind string) string {
	switch kind {
	case "phone":
		return "Type the phone number you hear. Digits are fine."
	case "address":
		return "Type the address you hear."
	case "price":
		return "Type the price you hear."
	case "date":
		return "Type the date you hear."
	case "time":
		return "Type the time you hear."
	case "postcode":
		return "Type the postcode you hear."
	case "reference":
		return "Type the reference number you hear."
	case "percentage":
		return "Type the percentage you hear."
	case "distance":
		return "Type the distance you hear."
	case "weight":
		return "Type the weight you hear."
	case "duration":
		return "Type the duration you hear."
	case "room":
		return "Type the room number you hear."
	default:
		return "Type the number you hear."
	}
}

// Seed inserts the bundled vocabulary and writing exercises. It is idempotent:
// an exercise whose slug already exists keeps its text, so editing the seed
// file never rewrites content that learners may already have attempts on. Its
// tier and source may still be corrected, because those are curriculum
// metadata. It returns how many new exercises were inserted.
func (s *Store) Seed(ctx context.Context) (int, error) {
	items, err := loadSeed()
	if err != nil {
		return 0, err
	}
	return s.seedItems(ctx, items)
}

// SeedListening inserts the bundled listening bank.
func (s *Store) SeedListening(ctx context.Context) (int, error) {
	items, err := loadListeningSeed()
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

// SeedDir loads every .json batch in a directory, in file name order. A missing
// directory is reported as os.ErrNotExist so startup can treat it as optional.
func (s *Store) SeedDir(ctx context.Context, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		n, err := s.SeedFile(ctx, filepath.Join(dir, name))
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
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
		accepted, _ := json.Marshal(it.AcceptedForms)
		res, err := tx.ExecContext(ctx, `
INSERT INTO exercises (slug, type, context, prompt, target_word, intended_meaning, expected_length, criteria, topic, cefr, tier, source,
                       audio_text, answer_text, listening_mode, listening_kind, accepted_forms)
VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8::jsonb, $9, $10, $11, $12,
        NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''), $17::jsonb)
ON CONFLICT (slug) DO NOTHING`,
			it.Slug, it.Type, it.Context, it.Prompt, it.TargetWord, it.IntendedMeaning,
			it.ExpectedLength, string(criteria), it.Topic, it.CEFR, it.Tier, it.Source,
			it.AudioText, it.AnswerText, it.ListeningMode, it.ListeningKind, string(accepted))
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
