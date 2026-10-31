package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ielts/backend/internal/eval"
)

// ErrDailyLimit means the learner has already been introduced to the maximum
// number of new items for today. Scheduled reviews are not affected by it.
var ErrDailyLimit = errors.New("daily new item limit reached")

// ErrNothingDue means every item is scheduled for the future. The caller can
// use NextDueAt to tell the learner when to come back.
var ErrNothingDue = errors.New("nothing is due right now")

// ErrInvalidMode means a listening request asked for a mode other than text or
// numeric.
var ErrInvalidMode = errors.New("listening mode must be text or numeric")

// ScheduleOptions controls selection. Now and DayStart are passed in so the
// server's local-day boundary follows the learner's time zone.
type ScheduleOptions struct {
	Now       time.Time
	DayStart  time.Time
	NewPerDay int
}

// Exercise is shared content. Reference answers are never serialized to the
// client: IntendedMeaning, AnswerText and AcceptedForms are loaded for
// evaluation only. AudioText is the exception, because the browser's speech
// synthesiser needs the script locally to speak it.
type Exercise struct {
	ID              int64    `json:"id"`
	Slug            string   `json:"-"`
	Type            string   `json:"type"`
	Context         string   `json:"context,omitempty"`
	Prompt          string   `json:"prompt"`
	TargetWord      string   `json:"target_word,omitempty"`
	IntendedMeaning string   `json:"-"`
	ExpectedLength  string   `json:"expected_length,omitempty"`
	Criteria        []string `json:"criteria,omitempty"`
	Topic           string   `json:"topic"`
	CEFR            string   `json:"cefr"`

	// Listening only.
	AudioText     string   `json:"audio_text,omitempty"`
	AnswerText    string   `json:"-"`
	ListeningMode string   `json:"listening_mode,omitempty"`
	ListeningKind string   `json:"listening_kind,omitempty"`
	AcceptedForms []string `json:"-"`
	// Accent is chosen per delivery by the API, not stored. The learner does
	// not pick it, like in a real listening test.
	Accent string `json:"accent,omitempty"`

	// Mode is "review" for a scheduled item and "new" for one the learner has
	// not answered before. Writing exercises have no mode.
	Mode  string     `json:"mode,omitempty"`
	DueAt *time.Time `json:"due_at,omitempty"`
}

// EvalRequest builds the request for the evaluator from the stored exercise and
// the learner's answer. The exercise content always comes from the database,
// never from the client.
func (e Exercise) EvalRequest(answer string) eval.Request {
	if e.Type == eval.TypeListening {
		return eval.Request{
			Type:          e.Type,
			Prompt:        e.Prompt,
			Answer:        answer,
			ExpectedText:  e.AnswerText,
			AcceptedForms: e.AcceptedForms,
			ListeningMode: e.ListeningMode,
		}
	}
	prompt := e.Prompt
	if e.Context != "" {
		prompt = fmt.Sprintf("Context: %q\n%s", e.Context, e.Prompt)
	}
	return eval.Request{
		Type:            e.Type,
		Prompt:          prompt,
		TargetWord:      e.TargetWord,
		IntendedMeaning: e.IntendedMeaning,
		ExpectedLength:  e.ExpectedLength,
		Criteria:        e.Criteria,
		Answer:          answer,
	}
}

const exerciseColumns = `e.id, e.slug, e.type, COALESCE(e.context, ''), e.prompt, COALESCE(e.target_word, ''),
	COALESCE(e.intended_meaning, ''), COALESCE(e.expected_length, ''), e.criteria, e.topic, e.cefr,
	COALESCE(e.audio_text, ''), COALESCE(e.answer_text, ''), COALESCE(e.listening_mode, ''),
	COALESCE(e.listening_kind, ''), e.accepted_forms`

type rowScanner interface{ Scan(dest ...any) error }

func scanExercise(r rowScanner) (*Exercise, error) {
	var e Exercise
	var criteria, accepted []byte
	if err := r.Scan(&e.ID, &e.Slug, &e.Type, &e.Context, &e.Prompt, &e.TargetWord,
		&e.IntendedMeaning, &e.ExpectedLength, &criteria, &e.Topic, &e.CEFR,
		&e.AudioText, &e.AnswerText, &e.ListeningMode, &e.ListeningKind, &accepted); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(criteria, &e.Criteria); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(accepted, &e.AcceptedForms); err != nil {
		return nil, err
	}
	return &e, nil
}

// scanExerciseWithDue is scanExercise for queries that also select the row's
// next review time.
func scanExerciseWithDue(r rowScanner) (*Exercise, error) {
	var e Exercise
	var criteria, accepted []byte
	var due time.Time
	if err := r.Scan(&e.ID, &e.Slug, &e.Type, &e.Context, &e.Prompt, &e.TargetWord,
		&e.IntendedMeaning, &e.ExpectedLength, &criteria, &e.Topic, &e.CEFR,
		&e.AudioText, &e.AnswerText, &e.ListeningMode, &e.ListeningKind, &accepted, &due); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(criteria, &e.Criteria); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(accepted, &e.AcceptedForms); err != nil {
		return nil, err
	}
	e.DueAt = &due
	return &e, nil
}

func (s *Store) GetExercise(ctx context.Context, id int64) (*Exercise, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+exerciseColumns+` FROM exercises e WHERE e.id = $1 AND e.status = 'published'`, id)
	e, err := scanExercise(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// NextExercise picks the next vocabulary or writing exercise.
//
// Vocabulary follows spaced repetition. A review that is due comes first, then
// a new word if the daily limit has not been reached. New words are introduced
// by tier (1 is the most useful), then at random. Writing exercises keep the
// simpler rule: never attempted first, then the one attempted longest ago.
//
// typ is "vocab", "writing", or "" for either. Listening has its own endpoint
// and its own schedule, so it never appears here.
func (s *Store) NextExercise(ctx context.Context, typ string, excludeID int64, opts ScheduleOptions) (*Exercise, error) {
	s.fillScheduleDefaults(&opts)
	switch typ {
	case eval.TypeVocab:
		return s.nextVocab(ctx, excludeID, opts)
	case eval.TypeWriting:
		return s.nextWriting(ctx, excludeID)
	default:
		// Mixed practice: prefer due reviews, then new vocabulary, and fall
		// back to writing when the vocabulary schedule is finished for now.
		e, err := s.nextVocab(ctx, excludeID, opts)
		if err == nil {
			return e, nil
		}
		if errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if errors.Is(err, ErrDailyLimit) || errors.Is(err, ErrNothingDue) {
			if w, werr := s.nextWriting(ctx, excludeID); werr == nil {
				return w, nil
			}
			return nil, err
		}
		return nil, err
	}
}

func (s *Store) fillScheduleDefaults(opts *ScheduleOptions) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.DayStart.IsZero() {
		n := opts.Now
		opts.DayStart = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	}
}

func (s *Store) nextVocab(ctx context.Context, excludeID int64, opts ScheduleOptions) (*Exercise, error) {
	// 1. A due review. Due items are shuffled so one word does not lead every
	// session; they still all come before any new word.
	row := s.db.QueryRowContext(ctx, `
SELECT `+exerciseColumns+`, r.due_at
FROM exercises e
JOIN review_state r ON r.exercise_id = e.id
WHERE e.status = 'published' AND e.type = 'vocab' AND r.due_at <= $1
ORDER BY (e.id = $2) ASC, random()
LIMIT 1`, opts.Now, excludeID)
	e, err := scanExerciseWithDue(row)
	if err == nil {
		e.Mode = "review"
		return e, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 2. A new word, if today's allowance is not used up.
	if opts.NewPerDay > 0 {
		var introduced int
		if err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM review_state WHERE created_at >= $1`, opts.DayStart).Scan(&introduced); err != nil {
			return nil, err
		}
		if introduced >= opts.NewPerDay {
			// Distinguish "limit reached" from "there is nothing new left".
			var hasNew bool
			if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM exercises e
    LEFT JOIN review_state r ON r.exercise_id = e.id
    WHERE e.status = 'published' AND e.type = 'vocab' AND r.exercise_id IS NULL
)`).Scan(&hasNew); err != nil {
				return nil, err
			}
			if !hasNew {
				return nil, ErrNothingDue
			}
			return nil, ErrDailyLimit
		}
	}

	row = s.db.QueryRowContext(ctx, `
SELECT `+exerciseColumns+`
FROM exercises e
LEFT JOIN review_state r ON r.exercise_id = e.id
WHERE e.status = 'published' AND e.type = 'vocab' AND r.exercise_id IS NULL
ORDER BY (e.id = $1) ASC, e.tier ASC, random()
LIMIT 1`, excludeID)
	e, err = scanExercise(row)
	if err == nil {
		e.Mode = "new"
		return e, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		var bank int
		if err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM exercises WHERE status = 'published' AND type = 'vocab'`).Scan(&bank); err != nil {
			return nil, err
		}
		if bank == 0 {
			return nil, ErrNotFound
		}
		return nil, ErrNothingDue
	}
	return nil, err
}

func (s *Store) nextWriting(ctx context.Context, excludeID int64) (*Exercise, error) {
	const q = `
SELECT ` + exerciseColumns + `
FROM exercises e
LEFT JOIN (
    SELECT exercise_id, MAX(created_at) AS last_at
    FROM attempts
    WHERE parent_attempt_id IS NULL AND status <> 'failed'
    GROUP BY exercise_id
) a ON a.exercise_id = e.id
WHERE e.status = 'published' AND e.type = 'writing'
ORDER BY (e.id = $1) ASC,      -- skip the previous exercise unless it is the only choice
         (a.last_at IS NOT NULL) ASC,
         a.last_at ASC NULLS FIRST,
         random()
LIMIT 1`
	e, err := scanExercise(s.db.QueryRowContext(ctx, q, excludeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// NextListening picks a listening item of one mode. Due reviews come first,
// then new items under the daily cap. The schedule is separate from vocabulary.
func (s *Store) NextListening(ctx context.Context, mode string, excludeID int64, opts ScheduleOptions) (*Exercise, error) {
	if mode != "text" && mode != "numeric" {
		return nil, ErrInvalidMode
	}
	s.fillScheduleDefaults(&opts)

	// 1. A due review of the requested mode.
	row := s.db.QueryRowContext(ctx, `
SELECT `+exerciseColumns+`, r.due_at
FROM exercises e
JOIN listening_review_state r ON r.exercise_id = e.id
WHERE e.status = 'published' AND e.type = 'listening' AND e.listening_mode = $3 AND r.due_at <= $1
ORDER BY (e.id = $2) ASC, random()
LIMIT 1`, opts.Now, excludeID, mode)
	e, err := scanExerciseWithDue(row)
	if err == nil {
		e.Mode = "review"
		return e, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// 2. A new item, if today's listening allowance is not used up. The cap is
	// shared by both modes; reviews are never capped.
	if opts.NewPerDay > 0 {
		var introduced int
		if err := s.db.QueryRowContext(ctx, `
SELECT count(*)
FROM listening_review_state r
JOIN exercises e ON e.id = r.exercise_id
WHERE e.type = 'listening' AND r.created_at >= $1`, opts.DayStart).Scan(&introduced); err != nil {
			return nil, err
		}
		if introduced >= opts.NewPerDay {
			var hasNew bool
			if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM exercises e
    LEFT JOIN listening_review_state r ON r.exercise_id = e.id
    WHERE e.status = 'published' AND e.type = 'listening'
      AND e.listening_mode = $1 AND r.exercise_id IS NULL
)`, mode).Scan(&hasNew); err != nil {
				return nil, err
			}
			if !hasNew {
				return nil, ErrNothingDue
			}
			return nil, ErrDailyLimit
		}
	}

	row = s.db.QueryRowContext(ctx, `
SELECT `+exerciseColumns+`
FROM exercises e
LEFT JOIN listening_review_state r ON r.exercise_id = e.id
WHERE e.status = 'published' AND e.type = 'listening'
  AND e.listening_mode = $1 AND r.exercise_id IS NULL
ORDER BY (e.id = $2) ASC, e.tier ASC, random()
LIMIT 1`, mode, excludeID)
	e, err = scanExercise(row)
	if err == nil {
		e.Mode = "new"
		return e, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		var bank int
		if err := s.db.QueryRowContext(ctx, `
SELECT count(*) FROM exercises
WHERE status = 'published' AND type = 'listening' AND listening_mode = $1`, mode).Scan(&bank); err != nil {
			return nil, err
		}
		if bank == 0 {
			return nil, ErrNotFound
		}
		return nil, ErrNothingDue
	}
	return nil, err
}

// ContentIndex is the set of existing slugs and content keys for one exercise
// type. The manual generator uses it to avoid adding the same word, prompt or
// recording twice.
type ContentIndex struct {
	Slugs map[string]bool
	Keys  map[string]bool
}

// ContentIndex reads the existing slugs and the key field for a type: the
// target word for vocabulary, the prompt for writing, and the spoken text for
// listening. Keys are lowercased and trimmed.
func (s *Store) ContentIndex(ctx context.Context, typ string) (*ContentIndex, error) {
	var q string
	switch typ {
	case eval.TypeVocab:
		q = `SELECT slug, COALESCE(target_word, '') FROM exercises WHERE type = 'vocab'`
	case eval.TypeWriting:
		q = `SELECT slug, prompt FROM exercises WHERE type = 'writing'`
	case eval.TypeListening:
		q = `SELECT slug, COALESCE(audio_text, '') FROM exercises WHERE type = 'listening'`
	default:
		return nil, fmt.Errorf("content index: unknown type %q", typ)
	}
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idx := &ContentIndex{Slugs: map[string]bool{}, Keys: map[string]bool{}}
	for rows.Next() {
		var slug, key string
		if err := rows.Scan(&slug, &key); err != nil {
			return nil, err
		}
		idx.Slugs[slug] = true
		if k := strings.ToLower(strings.TrimSpace(key)); k != "" {
			idx.Keys[k] = true
		}
	}
	return idx, rows.Err()
}

// NextDueAt returns when the next scheduled vocabulary review becomes due.
// It returns nil when nothing is scheduled.
func (s *Store) NextDueAt(ctx context.Context, now time.Time) (*time.Time, error) {
	var due sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT min(due_at) FROM review_state WHERE due_at > $1`, now).Scan(&due); err != nil {
		return nil, err
	}
	if !due.Valid {
		return nil, nil
	}
	return &due.Time, nil
}

// NextListeningDueAt returns when the next listening review of one mode becomes
// due, or nil when nothing is scheduled.
func (s *Store) NextListeningDueAt(ctx context.Context, mode string, now time.Time) (*time.Time, error) {
	var due sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
SELECT min(r.due_at)
FROM listening_review_state r
JOIN exercises e ON e.id = r.exercise_id
WHERE e.type = 'listening' AND e.listening_mode = $2 AND r.due_at > $1`, now, mode).Scan(&due); err != nil {
		return nil, err
	}
	if !due.Valid {
		return nil, nil
	}
	return &due.Time, nil
}
