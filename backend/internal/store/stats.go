package store

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"ielts/backend/internal/eval"
)

// Statistics are computed from the attempt history on every request. At the
// scale of one learner this is cheap, and it avoids keeping a second copy of
// the numbers that could drift out of sync.
//
// Quality measures (verdicts, error rates) use FIRST attempts only. A rewrite
// is made after seeing feedback, so counting it would make results look better
// than the learner's unaided ability.

type Stats struct {
	Totals                 Totals          `json:"totals"`
	FirstAttemptsEvaluated int             `json:"first_attempts_evaluated"`
	ByType                 []TypeStats     `json:"by_type"`
	Daily                  []DayCount      `json:"daily"`
	StreakDays             int             `json:"streak_days"`
	TopCategories          []CategoryStat  `json:"top_categories"`
	CategoryWindow         int             `json:"category_window"`
	Recent                 []RecentAttempt `json:"recent"`
	Trend                  Trend           `json:"trend"`
	Rewrites               RewriteStats    `json:"rewrites"`
	SRS                    SRSStats        `json:"srs"`
}

// SRSStats describes the vocabulary repetition schedule. "Mastered" means box
// 5, the last box, not a claim of perfect knowledge.
type SRSStats struct {
	Tracked   int        `json:"tracked"`
	DueNow    int        `json:"due_now"`
	NewToday  int        `json:"new_today"`
	NewPerDay int        `json:"new_per_day"`
	Boxes     []BoxCount `json:"boxes"`
	Mastered  int        `json:"mastered"`
	NextDueAt *time.Time `json:"next_due_at,omitempty"`
}

type BoxCount struct {
	Box   int `json:"box"`
	Count int `json:"count"`
}

type Totals struct {
	FirstAttempts int `json:"first_attempts"`
	Rewrites      int `json:"rewrites"`
	Evaluated     int `json:"evaluated"`
	Failed        int `json:"failed"`
}

type TypeStats struct {
	Type     string         `json:"type"`
	Total    int            `json:"total"`
	Verdicts map[string]int `json:"verdicts"`
}

type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type CategoryStat struct {
	Category      string  `json:"category"`
	Total         int     `json:"total"`
	PerTenAnswers float64 `json:"per_ten_answers"`
	RecentTotal   int     `json:"recent_total"`
	RecentPerTen  float64 `json:"recent_per_ten"`
}

type RecentAttempt struct {
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	Verdict string    `json:"verdict"`
	Score   int       `json:"score"`
}

// Trend compares the average verdict score of the last 10 first attempts with
// the 10 before them. It is only reported once there are at least 20.
type Trend struct {
	Enough      bool    `json:"enough"`
	RecentAvg   float64 `json:"recent_avg"`
	PreviousAvg float64 `json:"previous_avg"`
}

type RewriteStats struct {
	Total    int `json:"total"`
	Improved int `json:"improved"`
	Same     int `json:"same"`
	Worse    int `json:"worse"`
}

const (
	dailyDays        = 14
	streakLookback   = 70
	recentListSize   = 30
	categoryWindowSz = 10
)

func (s *Store) Stats(ctx context.Context, loc *time.Location, now time.Time, newPerDay int) (*Stats, error) {
	st := &Stats{ByType: []TypeStats{}, TopCategories: []CategoryStat{}, Recent: []RecentAttempt{}}

	if err := s.db.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE parent_attempt_id IS NULL),
       count(*) FILTER (WHERE parent_attempt_id IS NOT NULL),
       count(*) FILTER (WHERE status = 'evaluated'),
       count(*) FILTER (WHERE status = 'failed')
FROM attempts`).Scan(&st.Totals.FirstAttempts, &st.Totals.Rewrites, &st.Totals.Evaluated, &st.Totals.Failed); err != nil {
		return nil, err
	}

	// Verdicts per exercise type, first attempts only.
	rows, err := s.db.QueryContext(ctx, `
SELECT e.type, a.verdict, count(*)
FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE a.status = 'evaluated' AND a.parent_attempt_id IS NULL
GROUP BY e.type, a.verdict`)
	if err != nil {
		return nil, err
	}
	byType := map[string]*TypeStats{}
	for rows.Next() {
		var typ, verdict string
		var n int
		if err := rows.Scan(&typ, &verdict, &n); err != nil {
			rows.Close()
			return nil, err
		}
		ts := byType[typ]
		if ts == nil {
			ts = &TypeStats{Type: typ, Verdicts: map[string]int{}}
			byType[typ] = ts
		}
		ts.Verdicts[verdict] = n
		ts.Total += n
		st.FirstAttemptsEvaluated += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range []string{"vocab", "writing"} {
		if ts, ok := byType[t]; ok {
			st.ByType = append(st.ByType, *ts)
		}
	}

	if err := s.fillDaily(ctx, st, loc, now); err != nil {
		return nil, err
	}
	if err := s.fillCategories(ctx, st); err != nil {
		return nil, err
	}
	if err := s.fillRecent(ctx, st); err != nil {
		return nil, err
	}
	if err := s.fillRewrites(ctx, st); err != nil {
		return nil, err
	}
	if err := s.fillSRS(ctx, st, loc, now, newPerDay); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) fillSRS(ctx context.Context, st *Stats, loc *time.Location, now time.Time, newPerDay int) error {
	srs := &st.SRS
	srs.NewPerDay = newPerDay
	srs.Boxes = make([]BoxCount, 5)
	for i := range srs.Boxes {
		srs.Boxes[i].Box = i + 1
	}

	n := now.In(loc)
	dayStart := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	var nextDue sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE due_at <= $1),
       count(*) FILTER (WHERE created_at >= $2),
       count(*) FILTER (WHERE box >= 5),
       min(due_at) FILTER (WHERE due_at > $1)
FROM review_state`, now, dayStart).Scan(&srs.Tracked, &srs.DueNow, &srs.NewToday, &srs.Mastered, &nextDue); err != nil {
		return err
	}
	if nextDue.Valid {
		srs.NextDueAt = &nextDue.Time
	}

	rows, err := s.db.QueryContext(ctx, `SELECT box, count(*) FROM review_state GROUP BY box`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var box, count int
		if err := rows.Scan(&box, &count); err != nil {
			return err
		}
		if box >= 1 && box <= len(srs.Boxes) {
			srs.Boxes[box-1].Count = count
		}
	}
	return rows.Err()
}

// fillDaily buckets evaluated answers by calendar day in the learner's time
// zone (done in Go so daylight saving and zone names are always handled by one
// implementation) and derives the practice streak.
func (s *Store) fillDaily(ctx context.Context, st *Stats, loc *time.Location, now time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT created_at FROM attempts
WHERE status = 'evaluated' AND created_at >= $1`, now.AddDate(0, 0, -streakLookback-1))
	if err != nil {
		return err
	}
	defer rows.Close()
	perDay := map[string]int{}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return err
		}
		perDay[t.In(loc).Format("2006-01-02")]++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	day := func(back int) string { return today.AddDate(0, 0, -back).Format("2006-01-02") }

	st.Daily = make([]DayCount, 0, dailyDays)
	for i := dailyDays - 1; i >= 0; i-- {
		d := day(i)
		st.Daily = append(st.Daily, DayCount{Date: d, Count: perDay[d]})
	}

	// A streak is not broken until a full day passes without practice, so if
	// today has no answers yet the count starts from yesterday.
	start := 0
	if perDay[day(0)] == 0 {
		start = 1
	}
	for i := start; i < streakLookback && perDay[day(i)] > 0; i++ {
		st.StreakDays++
	}
	return nil
}

func (s *Store) fillCategories(ctx context.Context, st *Stats) error {
	st.CategoryWindow = min(categoryWindowSz, st.FirstAttemptsEvaluated)
	rows, err := s.db.QueryContext(ctx, `
WITH firsts AS (
    SELECT id, row_number() OVER (ORDER BY created_at DESC, id) AS rn
    FROM attempts WHERE status = 'evaluated' AND parent_attempt_id IS NULL
)
SELECT i.category, count(*), count(*) FILTER (WHERE f.rn <= $1)
FROM attempt_issues i JOIN firsts f ON f.id = i.attempt_id
WHERE i.severity = 'error'
GROUP BY i.category
ORDER BY count(*) DESC, i.category`, categoryWindowSz)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c CategoryStat
		if err := rows.Scan(&c.Category, &c.Total, &c.RecentTotal); err != nil {
			return err
		}
		if st.FirstAttemptsEvaluated > 0 {
			c.PerTenAnswers = round1(float64(c.Total) / float64(st.FirstAttemptsEvaluated) * 10)
		}
		if st.CategoryWindow > 0 {
			c.RecentPerTen = round1(float64(c.RecentTotal) / float64(st.CategoryWindow) * 10)
		}
		st.TopCategories = append(st.TopCategories, c)
	}
	return rows.Err()
}

func (s *Store) fillRecent(ctx context.Context, st *Stats) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.created_at, e.type, a.verdict
FROM attempts a JOIN exercises e ON e.id = a.exercise_id
WHERE a.status = 'evaluated' AND a.parent_attempt_id IS NULL
ORDER BY a.created_at DESC, a.id
LIMIT $1`, recentListSize)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r RecentAttempt
		if err := rows.Scan(&r.At, &r.Type, &r.Verdict); err != nil {
			return err
		}
		r.Score = eval.VerdictScore(r.Verdict)
		st.Recent = append(st.Recent, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// Oldest first, so charts read left to right.
	sort.SliceStable(st.Recent, func(i, j int) bool { return st.Recent[i].At.Before(st.Recent[j].At) })

	if len(st.Recent) >= 20 {
		last := st.Recent[len(st.Recent)-10:]
		prev := st.Recent[len(st.Recent)-20 : len(st.Recent)-10]
		st.Trend = Trend{Enough: true, RecentAvg: round1(avgScore(last)), PreviousAvg: round1(avgScore(prev))}
	}
	return nil
}

func (s *Store) fillRewrites(ctx context.Context, st *Stats) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.verdict, c.verdict
FROM attempts c JOIN attempts p ON p.id = c.parent_attempt_id
WHERE c.status = 'evaluated' AND p.status = 'evaluated'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var before, after string
		if err := rows.Scan(&before, &after); err != nil {
			return err
		}
		st.Rewrites.Total++
		switch b, a := eval.VerdictScore(before), eval.VerdictScore(after); {
		case a > b:
			st.Rewrites.Improved++
		case a < b:
			st.Rewrites.Worse++
		default:
			st.Rewrites.Same++
		}
	}
	return rows.Err()
}

func avgScore(rs []RecentAttempt) float64 {
	total := 0
	for _, r := range rs {
		total += r.Score
	}
	return float64(total) / float64(len(rs))
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
