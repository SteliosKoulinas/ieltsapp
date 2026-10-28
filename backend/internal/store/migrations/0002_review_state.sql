-- Vocabulary scheduling (Leitner boxes), controlled by the backend.
--
-- There is still one learner and no users table. When authentication arrives,
-- this table gets a user_id and the primary key becomes (user_id, exercise_id).
--
-- tier is a curriculum priority: 1 is the most useful vocabulary for the
-- learner's current goal, 4 is the lowest. It only affects which NEW exercise
-- is introduced next, never which review is due.
ALTER TABLE exercises ADD COLUMN tier INT NOT NULL DEFAULT 2 CHECK (tier BETWEEN 1 AND 4);

CREATE TABLE review_state (
    exercise_id  BIGINT PRIMARY KEY REFERENCES exercises (id) ON DELETE CASCADE,
    box          INT NOT NULL DEFAULT 1 CHECK (box BETWEEN 1 AND 5),
    due_at       TIMESTAMPTZ NOT NULL,
    streak       INT NOT NULL DEFAULT 0,
    lapses       INT NOT NULL DEFAULT 0,
    last_verdict TEXT CHECK (last_verdict IN ('correct', 'mostly_correct', 'partially_correct', 'incorrect')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The scheduler asks for the oldest due review first.
CREATE INDEX review_state_due_idx ON review_state (due_at);

-- Exercises answered before this migration existed should be reviewed, not
-- introduced as new again. They become due immediately; created_at keeps the
-- original first answer date so they do not count towards today's new limit.
INSERT INTO review_state (exercise_id, box, due_at, created_at, updated_at)
SELECT e.id, 1, now(), min(a.evaluated_at), now()
FROM exercises e
JOIN attempts a ON a.exercise_id = e.id
WHERE e.type = 'vocab' AND a.parent_attempt_id IS NULL AND a.status = 'evaluated'
GROUP BY e.id
ON CONFLICT (exercise_id) DO NOTHING;
