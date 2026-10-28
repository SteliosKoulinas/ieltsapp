// Package httpapi is the JSON REST API consumed by the SvelteKit frontend.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "time/tzdata" // IANA time zone data embedded, so zones work in a minimal container

	"ielts/backend/internal/eval"
	"ielts/backend/internal/store"
)

const (
	maxBodyBytes   = 8 << 10
	maxAnswerRunes = 1000
	evalTimeout    = 75 * time.Second
)

// Evaluator is the AI evaluation service. Every submitted answer goes through it.
type Evaluator interface {
	Evaluate(ctx context.Context, req eval.Request) (*eval.Evaluation, error)
}

type Server struct {
	store          *store.Store
	eval           Evaluator
	model          string
	limiter        *limiter
	newVocabPerDay int
	log            *slog.Logger
	now            func() time.Time
}

func New(st *store.Store, ev Evaluator, model string, evalsPerMinute, newVocabPerDay int, log *slog.Logger) *Server {
	now := time.Now
	return &Server{store: st, eval: ev, model: model, limiter: newLimiter(evalsPerMinute, now),
		newVocabPerDay: newVocabPerDay, log: log, now: now}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/exercises/next", s.nextExercise)
	mux.HandleFunc("POST /api/attempts", s.createAttempt)
	mux.HandleFunc("GET /api/attempts", s.listAttempts)
	mux.HandleFunc("GET /api/attempts/{id}", s.getAttempt)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "No such endpoint.")
	})
	return s.recoverer(s.logRequests(securityHeaders(mux)))
}

// ---- responses ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	AttemptID string `json:"attempt_id,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: code, Message: msg})
}

// ---- middleware ----

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logRequests logs method, path, status and duration. Request bodies (the
// learner's answers) and headers are never logged.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic in handler", "panic", rec, "path", r.URL.Path)
				writeErr(w, http.StatusInternalServerError, "internal_error", "Something went wrong on the server.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---- handlers ----

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.Error("health check: database unreachable", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) nextExercise(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	if typ != "" && typ != "any" && typ != eval.TypeVocab && typ != eval.TypeWriting {
		writeErr(w, http.StatusBadRequest, "invalid_type", "type must be vocab, writing or any.")
		return
	}
	if typ == "any" {
		typ = ""
	}
	var exclude int64
	if v := r.URL.Query().Get("exclude"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "invalid_exclude", "exclude must be an exercise id.")
			return
		}
		exclude = n
	}
	loc, ok := s.locationParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_timezone", "tz must be an IANA time zone name such as Europe/Athens.")
		return
	}
	now := s.now().In(loc)
	opts := store.ScheduleOptions{
		Now:       now,
		DayStart:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc),
		NewPerDay: s.newVocabPerDay,
	}
	ex, err := s.store.NextExercise(r.Context(), typ, exclude, opts)
	switch {
	case errors.Is(err, store.ErrDailyLimit):
		writeErr(w, http.StatusNotFound, "daily_limit_reached",
			"You have reached the daily limit for new words. Due reviews are unaffected, and writing practice is still available.")
		return
	case errors.Is(err, store.ErrNothingDue):
		msg := "Nothing is due right now. Reviews return on their schedule."
		if due, derr := s.store.NextDueAt(r.Context(), now); derr == nil && due != nil {
			msg = "Nothing is due right now. The next review is at " + due.In(loc).Format("2 Jan 15:04") + "."
		}
		writeErr(w, http.StatusNotFound, "nothing_due", msg)
		return
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "no_exercise", "There are no exercises available for this choice.")
		return
	case err != nil:
		s.serverError(w, "next exercise", err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type attemptRequest struct {
	ID              string  `json:"id"`
	ExerciseID      int64   `json:"exercise_id"`
	Answer          string  `json:"answer"`
	ParentAttemptID *string `json:"parent_attempt_id"`
}

type attemptResponse struct {
	AttemptID  string          `json:"attempt_id"`
	ExerciseID int64           `json:"exercise_id"`
	Status     string          `json:"status"`
	Evaluation json.RawMessage `json:"evaluation"`
	Reused     bool            `json:"reused"`
}

func (s *Server) createAttempt(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in attemptRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "The request body is not valid JSON or is too large.")
		return
	}
	in.ID = strings.ToLower(in.ID)
	in.Answer = strings.TrimSpace(in.Answer)
	if !uuidRe.MatchString(in.ID) {
		writeErr(w, http.StatusBadRequest, "invalid_id", "id must be a UUID generated by the client.")
		return
	}
	if in.ExerciseID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid_exercise", "exercise_id is required.")
		return
	}
	if n := utf8.RuneCountInString(in.Answer); n == 0 || n > maxAnswerRunes || !utf8.ValidString(in.Answer) || strings.ContainsRune(in.Answer, 0) {
		writeErr(w, http.StatusBadRequest, "invalid_answer", "The answer must be 1 to 1000 characters.")
		return
	}
	parentID := ""
	if in.ParentAttemptID != nil && *in.ParentAttemptID != "" {
		parentID = strings.ToLower(*in.ParentAttemptID)
		if !uuidRe.MatchString(parentID) {
			writeErr(w, http.StatusBadRequest, "invalid_parent", "parent_attempt_id must be a UUID.")
			return
		}
	}

	ctx := r.Context()
	ex, err := s.store.GetExercise(ctx, in.ExerciseID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "exercise_not_found", "That exercise does not exist.")
		return
	}
	if err != nil {
		s.serverError(w, "get exercise", err)
		return
	}
	if parentID != "" {
		parentEx, err := s.store.AttemptExerciseID(ctx, parentID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && parentEx != ex.ID) {
			writeErr(w, http.StatusBadRequest, "invalid_parent", "The attempt you are rewriting does not belong to this exercise.")
			return
		}
		if err != nil {
			s.serverError(w, "get parent attempt", err)
			return
		}
	}

	// Save the answer first, so it is never lost if the AI provider fails.
	outcome, err := s.store.BeginAttempt(ctx, store.BeginParams{
		ID: in.ID, ExerciseID: ex.ID, ParentID: parentID, Answer: in.Answer,
	})
	if err != nil {
		s.serverError(w, "begin attempt", err)
		return
	}
	switch outcome {
	case store.AlreadyEvaluated:
		d, err := s.store.GetAttempt(ctx, in.ID)
		if err != nil {
			s.serverError(w, "load evaluated attempt", err)
			return
		}
		writeJSON(w, http.StatusOK, attemptResponse{AttemptID: d.ID, ExerciseID: ex.ID, Status: d.Status, Evaluation: d.Evaluation, Reused: true})
		return
	case store.InProgress:
		writeJSON(w, http.StatusConflict, errorBody{Error: "in_progress", AttemptID: in.ID, Retryable: true,
			Message: "This answer is already being evaluated. Wait a few seconds and try again."})
		return
	case store.Conflict:
		writeErr(w, http.StatusConflict, "id_conflict", "This attempt id was already used for a different answer.")
		return
	}

	// Started or Retrying: this request owns the evaluation.
	if !s.limiter.allow() {
		s.fail(r, in.ID, "local_rate_limit")
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate_limited", AttemptID: in.ID, Retryable: true,
			Message: "Too many evaluations in a short time. Wait a few seconds and try again. Your answer was saved."})
		return
	}

	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	result, err := s.eval.Evaluate(evalCtx, ex.EvalRequest(in.Answer))
	if err != nil {
		s.respondEvaluationFailure(w, r, in.ID, err)
		return
	}

	// Persist even if the browser has already gone away.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelSave()
	if err := s.store.CompleteAttempt(saveCtx, in.ID, result, s.model, eval.PromptVersion()); err != nil {
		s.log.Error("saving evaluation failed", "attempt", in.ID, "err", err)
		s.fail(r, in.ID, "storage_error")
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "storage_error", AttemptID: in.ID, Retryable: true,
			Message: "The feedback could not be saved. Try again."})
		return
	}
	body, _ := json.Marshal(result)
	writeJSON(w, http.StatusOK, attemptResponse{AttemptID: in.ID, ExerciseID: ex.ID, Status: "evaluated", Evaluation: body})
}

func (s *Server) respondEvaluationFailure(w http.ResponseWriter, r *http.Request, id string, err error) {
	var rl *eval.RateLimitError
	switch {
	case errors.Is(err, eval.ErrRateLimited):
		s.log.Warn("provider rate limited", "attempt", id, "err", err)
		s.fail(r, id, "provider_rate_limited")
		msg := "The AI provider's rate limit was reached. Wait about a minute and try again. Your answer was saved."
		if errors.As(err, &rl) && rl.Daily() {
			msg = "The AI provider's daily quota is used up. Try again later. Your answer was saved."
		}
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate_limited", AttemptID: id, Retryable: true, Message: msg})
	case errors.Is(err, eval.ErrInvalidOutput):
		s.log.Warn("evaluation failed validation", "attempt", id, "err", err)
		s.fail(r, id, "invalid_output")
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "evaluation_unavailable", AttemptID: id, Retryable: true,
			Message: "The AI returned feedback that did not pass validation. Try again. Your answer was saved."})
	default:
		s.log.Warn("evaluation failed", "attempt", id, "err", err)
		s.fail(r, id, "provider_error")
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "evaluation_unavailable", AttemptID: id, Retryable: true,
			Message: "Evaluation is temporarily unavailable. Your answer was saved, please try again."})
	}
}

// fail marks the attempt as failed so the same id can be retried.
func (s *Server) fail(r *http.Request, id, code string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.FailAttempt(ctx, id, code); err != nil {
		s.log.Error("could not mark attempt as failed", "attempt", id, "err", err)
	}
}

func (s *Server) getAttempt(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.PathValue("id"))
	if !uuidRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, "invalid_id", "id must be a UUID.")
		return
	}
	d, err := s.store.GetAttempt(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "attempt_not_found", "That attempt does not exist.")
		return
	}
	if err != nil {
		s.serverError(w, "get attempt", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) listAttempts(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(r, "limit", 20, 1, 100)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100.")
		return
	}
	offset, ok := intParam(r, "offset", 0, 0, 1_000_000)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative number.")
		return
	}
	items, total, err := s.store.ListAttempts(r.Context(), limit, offset)
	if err != nil {
		s.serverError(w, "list attempts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	loc, ok := s.locationParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid_timezone", "tz must be an IANA time zone name such as Europe/Athens.")
		return
	}
	st, err := s.store.Stats(r.Context(), loc, s.now(), s.newVocabPerDay)
	if err != nil {
		s.serverError(w, "stats", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// locationParam reads the optional tz query parameter. All date-based limits
// follow the learner's local day, not the server's.
func (s *Server) locationParam(r *http.Request) (*time.Location, bool) {
	loc := time.UTC
	if tz := r.URL.Query().Get("tz"); tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil || len(tz) > 64 {
			return nil, false
		}
		loc = l
	}
	return loc, true
}

func intParam(r *http.Request, name string, def, lo, hi int) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

func (s *Server) serverError(w http.ResponseWriter, what string, err error) {
	s.log.Error(what+" failed", "err", err)
	writeErr(w, http.StatusInternalServerError, "internal_error", "Something went wrong on the server.")
}
