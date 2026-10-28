package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ielts/backend/internal/eval"
	"ielts/backend/internal/store"
	"ielts/backend/internal/store/storetest"
)

func ev(verdict string, issues ...eval.Issue) *eval.Evaluation {
	return &eval.Evaluation{Verdict: verdict, Summary: "s", ImprovedVersion: "x", Confidence: "high",
		Issues: issues, Raw: "{}", Calls: 1}
}

func issue(category, severity string) eval.Issue {
	return eval.Issue{Category: category, Severity: severity, Original: "o", Correction: "c", Explanation: "e"}
}

func uuid(n int) string {
	return "00000000-0000-4000-8000-" + pad(n)
}

func pad(n int) string {
	s := "000000000000"
	d := []byte(s)
	for i := len(d) - 1; n > 0 && i >= 0; i-- {
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d)
}

// scheduleOpts is a permissive schedule for tests that only care which
// exercise comes next: no daily limit and the real clock.
func scheduleOpts() store.ScheduleOptions {
	now := time.Now()
	return store.ScheduleOptions{
		Now:       now,
		DayStart:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		NewPerDay: 0,
	}
}

func TestMigrateAndSeedAreIdempotent(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	first, err := s.Seed(ctx)
	if err != nil || first < 30 {
		t.Fatalf("first seed inserted %d, err=%v", first, err)
	}
	second, err := s.Seed(ctx)
	if err != nil || second != 0 {
		t.Fatalf("second seed inserted %d, err=%v", second, err)
	}
}

func TestNextExercisePrefersUnseenThenOldest(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if _, err := s.NextExercise(ctx, "", 0, scheduleOpts()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("empty bank should give ErrNotFound, got %v", err)
	}
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	for _, typ := range []string{"vocab", "writing", ""} {
		e, err := s.NextExercise(ctx, typ, 0, scheduleOpts())
		if err != nil {
			t.Fatal(err)
		}
		if typ != "" && e.Type != typ {
			t.Errorf("asked for %s, got %s", typ, e.Type)
		}
	}

	// Attempt every writing exercise except one: the unseen one must come next.
	var unseen int64
	seen := map[int64]bool{}
	for i := 0; i < 200; i++ {
		e, err := s.NextExercise(ctx, "writing", 0, scheduleOpts())
		if err != nil {
			t.Fatal(err)
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		if len(seen) == 14 {
			unseen = e.ID
			break
		}
		// mark as attempted
		id := uuid(len(seen))
		if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: e.ID, Answer: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAttempt(ctx, id, ev("correct"), "m", "p"); err != nil {
			t.Fatal(err)
		}
	}
	if unseen == 0 {
		t.Fatal("did not reach the last unseen exercise")
	}
	e, err := s.NextExercise(ctx, "writing", 0, scheduleOpts())
	if err != nil || e.ID != unseen {
		t.Fatalf("expected the only unseen exercise %d, got %+v err=%v", unseen, e, err)
	}

	// excludeID is skipped when there is another choice.
	other, err := s.NextExercise(ctx, "writing", unseen, scheduleOpts())
	if err != nil || other.ID == unseen {
		t.Fatalf("exclude was ignored: %+v err=%v", other, err)
	}
}

func TestExerciseJSONNeverExposesReferenceMeaning(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := s.NextExercise(ctx, "vocab", 0, scheduleOpts())
	if err != nil {
		t.Fatal(err)
	}
	if e.IntendedMeaning == "" {
		t.Fatal("the store must load the reference meaning for evaluation")
	}
	req := e.EvalRequest("my answer")
	if req.IntendedMeaning == "" || req.TargetWord == "" || req.Answer != "my answer" {
		t.Fatalf("eval request incomplete: %+v", req)
	}
	// What the browser receives must not contain the reference answer.
	body, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), e.IntendedMeaning) || strings.Contains(string(body), "intended_meaning") {
		t.Fatalf("exercise JSON leaks the reference meaning: %s", body)
	}
}

func TestAttemptLifecycle(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	e, _ := s.NextExercise(ctx, "writing", 0, scheduleOpts())
	other, _ := s.NextExercise(ctx, "writing", e.ID, scheduleOpts())
	id := uuid(1)
	p := store.BeginParams{ID: id, ExerciseID: e.ID, Answer: "my answer"}

	if out, err := s.BeginAttempt(ctx, p); err != nil || out != store.Started {
		t.Fatalf("first begin: %v %v", out, err)
	}
	if out, _ := s.BeginAttempt(ctx, p); out != store.InProgress {
		t.Errorf("duplicate while pending should be InProgress, got %v", out)
	}
	if out, _ := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: e.ID, Answer: "different"}); out != store.Conflict {
		t.Errorf("same id with another answer should Conflict, got %v", out)
	}
	if out, _ := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: other.ID, Answer: "my answer"}); out != store.Conflict {
		t.Errorf("same id with another exercise should Conflict, got %v", out)
	}

	// Failure keeps the answer and allows a retry with the same id.
	if err := s.FailAttempt(ctx, id, "provider_error"); err != nil {
		t.Fatal(err)
	}
	if out, _ := s.BeginAttempt(ctx, p); out != store.Retrying {
		t.Errorf("after failure the same id should be Retrying, got %v", out)
	}

	// A pending attempt that is abandoned for long enough can be reclaimed.
	if _, err := raw.Exec(`UPDATE attempts SET updated_at = now() - interval '10 minutes' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if out, _ := s.BeginAttempt(ctx, p); out != store.Retrying {
		t.Errorf("stale pending attempt should be reclaimed, got %v", out)
	}

	// Completing stores the evaluation and one row per issue.
	if err := s.CompleteAttempt(ctx, id, ev("mostly_correct", issue("articles", "error"), issue("prepositions", "unnatural")), "model-x", "pv1"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = raw.QueryRow(`SELECT count(*) FROM attempt_issues WHERE attempt_id = $1`, id).Scan(&n)
	if n != 2 {
		t.Errorf("expected 2 issue rows, got %d", n)
	}
	if err := s.CompleteAttempt(ctx, id, ev("correct"), "m", "p"); err == nil {
		t.Error("completing an already evaluated attempt must fail, so progress is never applied twice")
	}
	if out, _ := s.BeginAttempt(ctx, p); out != store.AlreadyEvaluated {
		t.Errorf("expected AlreadyEvaluated, got %v", out)
	}

	d, err := s.GetAttempt(ctx, id)
	if err != nil || d.Status != "evaluated" || d.Answer != "my answer" || len(d.Evaluation) == 0 {
		t.Fatalf("detail: %+v err=%v", d, err)
	}
	if _, err := s.GetAttempt(ctx, uuid(99)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown attempt should be ErrNotFound, got %v", err)
	}

	var rows int
	_ = raw.QueryRow(`SELECT count(*) FROM attempts`).Scan(&rows)
	if rows != 1 {
		t.Errorf("retries must never create duplicate rows, found %d", rows)
	}
}

func TestListAttemptsPaging(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	_, _ = s.Seed(ctx)
	e, _ := s.NextExercise(ctx, "", 0, scheduleOpts())
	for i := 1; i <= 5; i++ {
		id := uuid(i)
		_, _ = s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: e.ID, Answer: "a"})
		_ = s.CompleteAttempt(ctx, id, ev("correct"), "m", "p")
	}
	page, total, err := s.ListAttempts(ctx, 2, 0)
	if err != nil || total != 5 || len(page) != 2 {
		t.Fatalf("page=%d total=%d err=%v", len(page), total, err)
	}
	page2, _, _ := s.ListAttempts(ctx, 2, 4)
	if len(page2) != 1 {
		t.Errorf("last page should have 1 item, got %d", len(page2))
	}
}

func TestStats(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	_, _ = s.Seed(ctx)
	vocab, _ := s.NextExercise(ctx, "vocab", 0, scheduleOpts())
	writing, _ := s.NextExercise(ctx, "writing", 0, scheduleOpts())

	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	add := func(n int, ex *store.Exercise, parent string, e *eval.Evaluation, at time.Time) string {
		id := uuid(n)
		if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, ParentID: parent, Answer: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAttempt(ctx, id, e, "m", "p"); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`UPDATE attempts SET created_at = $2 WHERE id = $1`, id, at); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// Day 0, day -1 and day -2 have practice (streak 3); day -3 has none.
	first := add(1, writing, "", ev("partially_correct", issue("articles", "error"), issue("articles", "error"), issue("clarity", "style")), now)
	add(2, vocab, "", ev("correct"), now.AddDate(0, 0, -1))
	add(3, vocab, "", ev("incorrect", issue("vocabulary_meaning", "error")), now.AddDate(0, 0, -2))
	// A rewrite of attempt 1 that improved the verdict. Rewrites are excluded from quality stats.
	add(4, writing, first, ev("correct"), now.Add(time.Minute))
	// One failed attempt.
	failedID := uuid(5)
	_, _ = s.BeginAttempt(ctx, store.BeginParams{ID: failedID, ExerciseID: vocab.ID, Answer: "a"})
	_ = s.FailAttempt(ctx, failedID, "provider_error")

	st, err := s.Stats(ctx, time.UTC, now.Add(time.Hour), 8)
	if err != nil {
		t.Fatal(err)
	}
	if st.Totals.FirstAttempts != 4 || st.Totals.Rewrites != 1 || st.Totals.Evaluated != 4 || st.Totals.Failed != 1 {
		t.Errorf("totals: %+v", st.Totals)
	}
	if st.FirstAttemptsEvaluated != 3 {
		t.Errorf("first attempts evaluated should exclude the rewrite and the failed one, got %d", st.FirstAttemptsEvaluated)
	}
	if st.StreakDays != 3 {
		t.Errorf("streak: want 3, got %d", st.StreakDays)
	}
	if len(st.Daily) != 14 || st.Daily[13].Count != 2 || st.Daily[12].Count != 1 || st.Daily[11].Count != 1 || st.Daily[10].Count != 0 {
		t.Errorf("daily: %+v", st.Daily[9:])
	}
	if st.Rewrites.Total != 1 || st.Rewrites.Improved != 1 {
		t.Errorf("rewrites: %+v", st.Rewrites)
	}
	// 2 article errors over 3 first answers = 6.7 per 10 answers; style issues are not counted.
	if len(st.TopCategories) != 2 || st.TopCategories[0].Category != "articles" || st.TopCategories[0].Total != 2 || st.TopCategories[0].PerTenAnswers != 6.7 {
		t.Errorf("categories: %+v", st.TopCategories)
	}
	for _, c := range st.TopCategories {
		if c.Category == "clarity" {
			t.Error("style issues must not count as errors")
		}
	}
	if st.Trend.Enough {
		t.Error("trend needs 20 first attempts")
	}
	var typeVerdicts = map[string]int{}
	for _, ts := range st.ByType {
		typeVerdicts[ts.Type] = ts.Total
	}
	if typeVerdicts["vocab"] != 2 || typeVerdicts["writing"] != 1 {
		t.Errorf("by type: %+v", st.ByType)
	}
}

func TestStatsTimeZoneAndStreakRules(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	_, _ = s.Seed(ctx)
	ex, _ := s.NextExercise(ctx, "vocab", 0, scheduleOpts())
	athens, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Skip("no tzdata in this environment")
	}
	add := func(n int, at time.Time) {
		id := uuid(n)
		_, _ = s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"})
		_ = s.CompleteAttempt(ctx, id, ev("correct"), "m", "p")
		_, _ = raw.Exec(`UPDATE attempts SET created_at = $2 WHERE id = $1`, id, at)
	}
	// 22:30 UTC on Oct 26 is 00:30 on Oct 27 in Athens (UTC+2 after the DST change).
	add(1, time.Date(2026, 10, 26, 22, 30, 0, 0, time.UTC))
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

	st, _ := s.Stats(ctx, athens, now, 8)
	if st.Daily[13].Date != "2026-10-27" || st.Daily[13].Count != 1 {
		t.Errorf("Athens should count it on Oct 27: %+v", st.Daily[12:])
	}
	stUTC, _ := s.Stats(ctx, time.UTC, now, 8)
	if stUTC.Daily[13].Count != 0 || stUTC.Daily[12].Count != 1 {
		t.Errorf("UTC should count it on Oct 26: %+v", stUTC.Daily[12:])
	}

	// No practice today yet but yesterday had some: the streak is not broken.
	add(2, time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC))
	st3, _ := s.Stats(ctx, time.UTC, time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), 8)
	if st3.StreakDays != 2 {
		t.Errorf("streak should count from yesterday: want 2, got %d", st3.StreakDays)
	}
}

func TestStatsTrendNeedsTwentyAttempts(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	_, _ = s.Seed(ctx)
	ex, _ := s.NextExercise(ctx, "vocab", 0, scheduleOpts())
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 20; i++ {
		verdict := "incorrect"
		if i > 10 {
			verdict = "correct"
		}
		id := uuid(i)
		_, _ = s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"})
		_ = s.CompleteAttempt(ctx, id, ev(verdict), "m", "p")
		_, _ = raw.Exec(`UPDATE attempts SET created_at = $2 WHERE id = $1`, id, now.Add(time.Duration(i)*time.Minute-time.Hour*3))
	}
	st, err := s.Stats(ctx, time.UTC, now.Add(time.Hour), 8)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Trend.Enough || st.Trend.PreviousAvg != 0 || st.Trend.RecentAvg != 3 {
		t.Errorf("trend: %+v", st.Trend)
	}
}

func TestVocabularyScheduleDueFirstAndDailyLimit(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	opts := func(at time.Time) store.ScheduleOptions {
		return store.ScheduleOptions{
			Now:       at,
			DayStart:  time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
			NewPerDay: 2,
		}
	}
	answer := func(n int, ex *store.Exercise, verdict string, at time.Time, parent string) string {
		id := uuid(n)
		if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, ParentID: parent, Answer: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAttemptAt(ctx, id, ev(verdict), "m", "p", at); err != nil {
			t.Fatal(err)
		}
		return id
	}
	state := func(exerciseID int64) (box int, due time.Time, streak int) {
		if err := raw.QueryRow(`SELECT box, due_at, streak FROM review_state WHERE exercise_id = $1`, exerciseID).
			Scan(&box, &due, &streak); err != nil {
			t.Fatal(err)
		}
		return box, due, streak
	}

	// A first exposure is "new" and a correct answer moves it to box 2, due one day later.
	first, err := s.NextExercise(ctx, "vocab", 0, opts(now))
	if err != nil || first.Mode != "new" {
		t.Fatalf("first exposure: %+v err=%v", first, err)
	}
	firstAttempt := answer(1, first, "correct", now, "")
	box, due, streak := state(first.ID)
	if box != 2 || streak != 1 || !due.Equal(now.Add(24*time.Hour)) {
		t.Errorf("after correct: box=%d streak=%d due=%v", box, streak, due)
	}

	// A rewrite must not touch the schedule.
	answer(2, first, "incorrect", now.Add(time.Minute), firstAttempt)
	if box, due2, _ := state(first.ID); box != 2 || !due2.Equal(due) {
		t.Errorf("a rewrite changed the schedule: box=%d due=%v", box, due2)
	}

	// Two hours later the first word is not due, so a second new word is introduced.
	second, err := s.NextExercise(ctx, "vocab", first.ID, opts(now.Add(2*time.Hour)))
	if err != nil || second.Mode != "new" || second.ID == first.ID {
		t.Fatalf("second new word: %+v err=%v", second, err)
	}
	// A wrong answer restarts it at box 1 with a short relearning delay.
	answer(3, second, "incorrect", now.Add(2*time.Hour), "")
	box, due, streak = state(second.ID)
	if box != 1 || streak != 0 || !due.Equal(now.Add(2*time.Hour+10*time.Minute)) {
		t.Errorf("after incorrect: box=%d streak=%d due=%v", box, streak, due)
	}

	// The daily new-word limit is now reached and the short review is not due yet.
	if _, err := s.NextExercise(ctx, "vocab", 0, opts(now.Add(2*time.Hour+5*time.Minute))); !errors.Is(err, store.ErrDailyLimit) {
		t.Fatalf("expected ErrDailyLimit, got %v", err)
	}
	// Mixed practice still offers writing when vocabulary has nothing available.
	if w, err := s.NextExercise(ctx, "", 0, opts(now.Add(2*time.Hour+5*time.Minute))); err != nil || w.Type != "writing" {
		t.Fatalf("mixed practice should fall back to writing: %+v err=%v", w, err)
	}

	// Once it is due, the review wins even though the new-word limit is reached.
	review, err := s.NextExercise(ctx, "vocab", 0, opts(now.Add(2*time.Hour+11*time.Minute)))
	if err != nil || review.Mode != "review" || review.ID != second.ID {
		t.Fatalf("due review should come first: %+v err=%v", review, err)
	}
	if review.DueAt == nil {
		t.Error("a review should report when it became due")
	}
}

// A word answered well must not come back in the same study session. This
// covers mostly_correct too, because it means the right idea with a minor
// imperfection; keeping it in box 1 made the same word reappear every ten
// minutes and lead every session.
func TestSuccessfulAnswerDoesNotRepeatSoon(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	opts := func(at time.Time) store.ScheduleOptions {
		return store.ScheduleOptions{
			Now:       at,
			DayStart:  time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
			NewPerDay: 8,
		}
	}
	answer := func(n int, ex *store.Exercise, verdict string, at time.Time) {
		id := uuid(n)
		if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAttemptAt(ctx, id, ev(verdict), "m", "p", at); err != nil {
			t.Fatal(err)
		}
	}

	for i, verdict := range []string{"correct", "mostly_correct"} {
		t.Run(verdict, func(t *testing.T) {
			ex, err := s.NextExercise(ctx, "vocab", 0, opts(now))
			if err != nil {
				t.Fatal(err)
			}
			answer(100+i, ex, verdict, now)

			var box int
			var due time.Time
			if err := raw.QueryRow(`SELECT box, due_at FROM review_state WHERE exercise_id = $1`, ex.ID).
				Scan(&box, &due); err != nil {
				t.Fatal(err)
			}
			if box != 2 {
				t.Errorf("%s should advance to box 2, got box %d", verdict, box)
			}
			if !due.Equal(now.Add(24 * time.Hour)) {
				t.Errorf("%s should be due in 24 hours, got %v", verdict, due)
			}

			// Ten minutes later, the same word must not be served again.
			next, err := s.NextExercise(ctx, "vocab", 0, opts(now.Add(10*time.Minute)))
			if err != nil {
				t.Fatal(err)
			}
			if next.ID == ex.ID {
				t.Fatalf("%s word returned after 10 minutes", verdict)
			}
			if next.Mode != "new" {
				t.Errorf("expected a new word instead, got mode %q", next.Mode)
			}
		})
	}
}

// A wrong answer must return soon so the learner can try again.
func TestWrongAnswerRepeatsSoon(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	opts := func(at time.Time) store.ScheduleOptions {
		return store.ScheduleOptions{
			Now:       at,
			DayStart:  time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
			NewPerDay: 8,
		}
	}
	ex, err := s.NextExercise(ctx, "vocab", 0, opts(now))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid(200)
	if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAttemptAt(ctx, id, ev("incorrect"), "m", "p", now); err != nil {
		t.Fatal(err)
	}
	review, err := s.NextExercise(ctx, "vocab", 0, opts(now.Add(11*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	if review.ID != ex.ID || review.Mode != "review" {
		t.Fatalf("a wrong answer should return in ten minutes: got %+v", review)
	}
}

func TestWritingIsNotScheduled(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ex, err := s.NextExercise(ctx, "writing", 0, scheduleOpts())
	if err != nil {
		t.Fatal(err)
	}
	id := uuid(1)
	if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAttemptAt(ctx, id, ev("correct"), "m", "p", now); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := raw.QueryRow(`SELECT count(*) FROM review_state`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("writing must not enter the vocabulary schedule, found %d rows", n)
	}
}

func TestStatsSRS(t *testing.T) {
	s, raw := storetest.NewWithDB(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	opts := store.ScheduleOptions{Now: now, DayStart: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), NewPerDay: 8}
	ex, err := s.NextExercise(ctx, "vocab", 0, opts)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid(1)
	if _, err := s.BeginAttempt(ctx, store.BeginParams{ID: id, ExerciseID: ex.ID, Answer: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAttemptAt(ctx, id, ev("correct"), "m", "p", now); err != nil {
		t.Fatal(err)
	}
	st, err := s.Stats(ctx, time.UTC, now, 8)
	if err != nil {
		t.Fatal(err)
	}
	if st.SRS.Tracked != 1 || st.SRS.DueNow != 0 || st.SRS.NewToday != 1 || st.SRS.NewPerDay != 8 {
		t.Errorf("srs counters: %+v", st.SRS)
	}
	if len(st.SRS.Boxes) != 5 || st.SRS.Boxes[1].Count != 1 || st.SRS.Boxes[1].Box != 2 {
		t.Errorf("srs boxes: %+v", st.SRS.Boxes)
	}
	if st.SRS.Mastered != 0 {
		t.Errorf("nothing should be mastered yet: %+v", st.SRS)
	}
	if st.SRS.NextDueAt == nil || !st.SRS.NextDueAt.Equal(now.Add(24*time.Hour)) {
		t.Errorf("next due: %v", st.SRS.NextDueAt)
	}

	// Make it due and it should be counted as a due review.
	if _, err := raw.Exec(`UPDATE review_state SET due_at = $1`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Stats(ctx, time.UTC, now, 8)
	if st.SRS.DueNow != 1 {
		t.Errorf("due now: %+v", st.SRS)
	}
}

func TestSeedFileAddsExtraContentAndValidates(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	if _, err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "extra.json")
	body := `[{
		"slug": "vocab-resilient",
		"type": "vocab",
		"context": "Cities need resilient systems that can cope with extreme weather.",
		"target_word": "resilient",
		"intended_meaning": "able to recover quickly from difficulties or change",
		"topic": "environment",
		"cefr": "B2",
		"tier": 1,
		"source": "generated"
	}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := s.SeedFile(ctx, path)
	if err != nil || n != 1 {
		t.Fatalf("extra seed inserted %d, err=%v", n, err)
	}
	if n, err := s.SeedFile(ctx, path); err != nil || n != 0 {
		t.Fatalf("second load inserted %d, err=%v", n, err)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"slug":"vocab-bad","type":"vocab","context":"no target here","target_word":"resilient","intended_meaning":"x","topic":"society","cefr":"B2"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SeedFile(ctx, bad); err == nil {
		t.Error("a context without the target word must be rejected")
	}
	if _, err := s.SeedFile(ctx, filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file should return os.ErrNotExist, got %v", err)
	}
}
