-- Listening dictation exercises and their own repetition schedule.
--
-- A listening exercise stores two texts:
--   audio_text   what the browser speaks (sent to the client, because the
--                Web Speech API needs it locally)
--   answer_text  the canonical transcript used for evaluation, never sent to
--                the browser before the answer is submitted
-- accepted_forms holds equivalent renderings, mainly for numbers.

ALTER TABLE exercises DROP CONSTRAINT exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check
    CHECK (type IN ('vocab', 'writing', 'listening'));

ALTER TABLE exercises
    ADD COLUMN audio_text      TEXT,
    ADD COLUMN answer_text     TEXT,
    ADD COLUMN listening_mode  TEXT CHECK (listening_mode IN ('text', 'numeric')),
    ADD COLUMN listening_kind  TEXT,
    ADD COLUMN accepted_forms  JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE exercises ADD CONSTRAINT listening_needs_content CHECK (
    type <> 'listening' OR (
        audio_text IS NOT NULL AND audio_text <> '' AND
        answer_text IS NOT NULL AND answer_text <> '' AND
        listening_mode IS NOT NULL AND
        listening_kind IS NOT NULL AND listening_kind <> ''
    )
);

CREATE INDEX exercises_listening_idx
    ON exercises (status, type, listening_mode, tier);

-- Speed and replay data. Only listening attempts fill these in; the checks
-- keep accidental client values from producing meaningless statistics.
ALTER TABLE attempts
    ADD COLUMN duration_ms INT CHECK (duration_ms IS NULL OR duration_ms BETWEEN 500 AND 900000),
    ADD COLUMN replays     INT NOT NULL DEFAULT 0 CHECK (replays BETWEEN 0 AND 20);

-- Listening has its own queue. It is separate from vocabulary review_state so
-- the two schedules and their statistics never mix.
CREATE TABLE listening_review_state (
    exercise_id  BIGINT PRIMARY KEY REFERENCES exercises (id) ON DELETE CASCADE,
    box          INT NOT NULL DEFAULT 1 CHECK (box BETWEEN 1 AND 5),
    due_at       TIMESTAMPTZ NOT NULL,
    streak       INT NOT NULL DEFAULT 0,
    lapses       INT NOT NULL DEFAULT 0,
    last_verdict TEXT CHECK (last_verdict IN ('correct', 'mostly_correct', 'partially_correct', 'incorrect')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX listening_review_state_due_idx ON listening_review_state (due_at);
