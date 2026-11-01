package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ielts/backend/internal/eval"
)

// BeginOutcome says what BeginAttempt decided about a submission.
type BeginOutcome int

const (
	Started          BeginOutcome = iota // new attempt saved as pending; the caller must evaluate it
	Retrying                             // an earlier failed (or abandoned) attempt was reclaimed; evaluate it
	AlreadyEvaluated                     // same submission was already evaluated; return the stored result
	InProgress                           // another request is evaluating this attempt right now
	Conflict                             // the id exists but with a different answer or exercise
)

// staleAfter is how long a pending attempt is considered abandoned (for
// example the server crashed mid-evaluation) and may be reclaimed by a retry.
const staleAfter = "2 minutes"

type BeginParams struct {
	ID         string // client generated UUID, the idempotency key
	ExerciseID int64
	ParentID   string // optional: the attempt this one rewrites
	Answer     string
}

// BeginAttempt saves the submitted answer BEFORE any AI call, so an answer is
// never lost if the provider fails. Submitting the same id again is safe: it
// never creates a second row and never starts two evaluations at once.
func (s *Store) BeginAttempt(ctx context.Context, p BeginParams) (BeginOutcome, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	parent := sql.NullString{String: p.ParentID, Valid: p.ParentID != ""}
	res, err := tx.ExecContext(ctx, `
INSERT INTO attempts (id, exercise_id, parent_attempt_id, answer, status)
VALUES ($1, $2, $3, $4, 'pending')
ON CONFLICT (id) DO NOTHING`, p.ID, p.ExerciseID, parent, p.Answer)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return Started, tx.Commit()
	}

	var exerciseID int64
	var answer, status string
	var stale bool
	err = tx.QueryRowContext(ctx, `
SELECT exercise_id, answer, status, (status = 'pending' AND updated_at < now() - interval '`+staleAfter+`')
FROM attempts WHERE id = $1 FOR UPDATE`, p.ID).Scan(&exerciseID, &answer, &status, &stale)
	if err != nil {
		return 0, err
	}

	var outcome BeginOutcome
	switch {
	case exerciseID != p.ExerciseID || answer != p.Answer:
		outcome = Conflict
	case status == "evaluated":
		outcome = AlreadyEvaluated
	case status == "failed" || stale:
		if _, err := tx.ExecContext(ctx,
			`UPDATE attempts SET status = 'pending', failure_code = NULL, updated_at = now() WHERE id = $1`, p.ID); err != nil {
			return 0, err
		}
		outcome = Retrying
	default:
		outcome = InProgress
	}
	return outcome, tx.Commit()
}

var errNotPending = errors.New("attempt is not pending")

// CompleteAttempt stores an accepted evaluation and its issue rows in one
// transaction. Statistics only ever see evaluations that passed validation.
func (s *Store) CompleteAttempt(ctx context.Context, id string, ev *eval.Evaluation, model, promptVersion string) error {
	return s.CompleteAttemptAt(ctx, id, ev, model, promptVersion, time.Now())
}

// CompleteAttemptAt is CompleteAttempt with an explicit clock, so tests can
// check the review schedule without waiting.
func (s *Store) CompleteAttemptAt(ctx context.Context, id string, ev *eval.Evaluation, model, promptVersion string, now time.Time) error {
	evJSON, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	// PostgreSQL JSONB and TEXT cannot hold NUL characters.
	evJSON = bytes.ReplaceAll(evJSON, []byte(`\u0000`), nil)
	raw := strings.ReplaceAll(ev.Raw, "\x00", "")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
UPDATE attempts SET status = 'evaluated', verdict = $2, confidence = $3, evaluation = $4::jsonb,
    raw_response = $5, model = $6, prompt_version = $7, provider_calls = $8, dropped_issues = $9,
    failure_code = NULL, evaluated_at = now(), updated_at = now()
WHERE id = $1 AND status = 'pending'`,
		id, ev.Verdict, ev.Confidence, string(evJSON), raw, model, promptVersion, ev.Calls, ev.Dropped)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errNotPending
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attempt_issues WHERE attempt_id = $1`, id); err != nil {
		return err
	}
	for i, is := range ev.Issues {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO attempt_issues (attempt_id, position, category, severity) VALUES ($1, $2, $3, $4)`,
			id, i, is.Category, is.Severity); err != nil {
			return err
		}
	}
	if err := updateReviewState(ctx, tx, id, ev.Verdict, now); err != nil {
		return err
	}
	return tx.Commit()
}

// updateReviewState advances the vocabulary schedule for a first attempt. A
// rewrite is made after seeing feedback, so it must not change the schedule.
func updateReviewState(ctx context.Context, tx *sql.Tx, attemptID, verdict string, now time.Time) error {
	var exerciseID int64
	var parent sql.NullString
	var typ string
	err := tx.QueryRowContext(ctx, `
SELECT a.exercise_id, a.parent_attempt_id, e.type
FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE a.id = $1`, attemptID).Scan(&exerciseID, &parent, &typ)
	if err != nil {
		return err
	}
	if parent.Valid || typ != eval.TypeVocab {
		return nil
	}

	// Make sure the row exists, then lock it so two completions for the same
	// word cannot both start from the same box.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO review_state (exercise_id, box, due_at, created_at, updated_at)
VALUES ($1, 1, $2, $2, $2)
ON CONFLICT (exercise_id) DO NOTHING`, exerciseID, now); err != nil {
		return err
	}

	var box, streak, lapses int
	if err := tx.QueryRowContext(ctx, `
SELECT box, streak, lapses FROM review_state WHERE exercise_id = $1 FOR UPDATE`, exerciseID).
		Scan(&box, &streak, &lapses); err != nil {
		return err
	}
	box, due, streak, lapses := scheduleReview(box, streak, lapses, verdict, now)
	_, err = tx.ExecContext(ctx, `
UPDATE review_state SET box = $2, due_at = $3, streak = $4, lapses = $5, last_verdict = $6, updated_at = $7
WHERE exercise_id = $1`, exerciseID, box, due, streak, lapses, verdict, now)
	return err
}

// reviewInterval is the waiting time after a successful review of a box.
// Box 1 is a relearning step measured in hours, not minutes: a word answered
// badly should return later the same day, so the learner can keep using the app
// instead of seeing the same word every few minutes. Box 5 counts as mastered
// by the stats.
func reviewInterval(box int) time.Duration {
	switch box {
	case 1:
		return 4 * time.Hour
	case 2:
		return 24 * time.Hour
	case 3:
		return 3 * 24 * time.Hour
	case 4:
		return 8 * 24 * time.Hour
	default:
		return 21 * 24 * time.Hour
	}
}

// scheduleReview applies a verdict to a Leitner box.
//
// The four verdicts form a graded scale:
//
//   - correct: the word moves up two boxes, so a word known well gets a
//     noticeably longer wait than one known approximately.
//   - mostly_correct: up one box. The right idea with a small imperfection still
//     counts as progress, so it is not repeated in the same session.
//   - partially_correct: down one box, but the next wait is capped at one day.
//     A half-known word returns within hours or the next day, not weeks.
//   - incorrect: back to box 1. The relearning step is four hours, so a mistake
//     returns later in the day and does not trap the learner in a short loop.
//
// A word answered correctly or mostly correctly is never due sooner than one
// day, which keeps it out of the same study session.
func scheduleReview(box, streak, lapses int, verdict string, now time.Time) (int, time.Time, int, int) {
	if box < 1 {
		box = 1
	}
	interval := reviewInterval(box)
	switch verdict {
	case "correct":
		box = min(box+2, 5)
		streak++
		interval = reviewInterval(box)
	case "mostly_correct":
		box = min(box+1, 5)
		streak++
		interval = reviewInterval(box)
	case "partially_correct":
		box = max(box-1, 1)
		streak = 0
		lapses++
		interval = min(reviewInterval(box), 24*time.Hour)
	case "incorrect":
		box = 1
		streak = 0
		lapses++
		interval = reviewInterval(box)
	}
	// A word the learner answered well must not return in the same session.
	if (verdict == "correct" || verdict == "mostly_correct") && interval < 24*time.Hour {
		interval = 24 * time.Hour
	}
	return box, now.Add(interval), streak, lapses
}

// FailAttempt marks a pending attempt as failed so it can be retried. The
// answer stays stored.
func (s *Store) FailAttempt(ctx context.Context, id, code string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE attempts SET status = 'failed', failure_code = $2, updated_at = now() WHERE id = $1 AND status = 'pending'`, id, code)
	return err
}

// AttemptExerciseID returns the exercise an attempt belongs to.
func (s *Store) AttemptExerciseID(ctx context.Context, id string) (int64, error) {
	var ex int64
	err := s.db.QueryRowContext(ctx, `SELECT exercise_id FROM attempts WHERE id = $1`, id).Scan(&ex)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return ex, err
}

type AttemptDetail struct {
	ID               string          `json:"id"`
	Exercise         Exercise        `json:"exercise"`
	ReferenceMeaning string          `json:"reference_meaning,omitempty"`
	ParentAttemptID  *string         `json:"parent_attempt_id"`
	Answer           string          `json:"answer"`
	Status           string          `json:"status"`
	Evaluation       json.RawMessage `json:"evaluation,omitempty"`
	FailureCode      string          `json:"failure_code,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

func (s *Store) GetAttempt(ctx context.Context, id string) (*AttemptDetail, error) {
	var d AttemptDetail
	var parent sql.NullString
	var evaluation []byte
	var failure sql.NullString
	var ex Exercise
	var criteria, accepted []byte
	err := s.db.QueryRowContext(ctx, `
SELECT a.id, a.parent_attempt_id, a.answer, a.status, a.evaluation, a.failure_code, a.created_at,
       e.id, e.slug, e.type, COALESCE(e.context, ''), e.prompt, COALESCE(e.target_word, ''),
       COALESCE(e.intended_meaning, ''), COALESCE(e.expected_length, ''), e.criteria, e.topic, e.cefr,
       COALESCE(e.audio_text, ''), COALESCE(e.answer_text, ''), COALESCE(e.listening_mode, ''),
       COALESCE(e.listening_kind, ''), e.accepted_forms
FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE a.id = $1 AND e.type <> 'listening'`, id).Scan(&d.ID, &parent, &d.Answer, &d.Status, &evaluation, &failure, &d.CreatedAt,
		&ex.ID, &ex.Slug, &ex.Type, &ex.Context, &ex.Prompt, &ex.TargetWord,
		&ex.IntendedMeaning, &ex.ExpectedLength, &criteria, &ex.Topic, &ex.CEFR,
		&ex.AudioText, &ex.AnswerText, &ex.ListeningMode, &ex.ListeningKind, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(criteria, &ex.Criteria); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(accepted, &ex.AcceptedForms); err != nil {
		return nil, err
	}
	if parent.Valid {
		d.ParentAttemptID = &parent.String
	}
	if len(evaluation) > 0 {
		d.Evaluation = evaluation
	}
	d.FailureCode = failure.String
	d.ReferenceMeaning = ex.IntendedMeaning
	// The recording script is not part of the history payload. The evaluated
	// transcript is already shown as the improved version, and a failed attempt
	// must not reveal the answer.
	ex.AudioText = ""
	d.Exercise = ex
	return &d, nil
}

type AttemptSummary struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Status     string    `json:"status"`
	Verdict    string    `json:"verdict,omitempty"`
	IsRewrite  bool      `json:"is_rewrite"`
	ExerciseID int64     `json:"exercise_id"`
	Type       string    `json:"type"`
	TargetWord string    `json:"target_word,omitempty"`
	Prompt     string    `json:"prompt"`
	Answer     string    `json:"answer"`
}

// ListAttempts returns attempts newest first, plus the total count for paging.
func (s *Store) ListAttempts(ctx context.Context, limit, offset int) ([]AttemptSummary, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `
SELECT count(*) FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE e.type <> 'listening'`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.created_at, a.status, COALESCE(a.verdict, ''), a.parent_attempt_id IS NOT NULL,
       e.id, e.type, COALESCE(e.target_word, ''), COALESCE(e.context, e.prompt), left(a.answer, 200)
FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE e.type <> 'listening'
ORDER BY a.created_at DESC, a.id
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []AttemptSummary{}
	for rows.Next() {
		var a AttemptSummary
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.Status, &a.Verdict, &a.IsRewrite,
			&a.ExerciseID, &a.Type, &a.TargetWord, &a.Prompt, &a.Answer); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
