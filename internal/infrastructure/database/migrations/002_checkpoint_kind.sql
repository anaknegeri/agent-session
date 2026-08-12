-- Checkpoints carried only a free-text label, which callers used for both the
-- trigger ("auto", "precompact", "handoff") and a description, sometimes an
-- entire decision. Retention needs to keep the meaningful checkpoints while
-- pruning the noisy ones, so the trigger becomes its own column.
ALTER TABLE checkpoints ADD COLUMN kind TEXT NOT NULL DEFAULT 'manual';

-- Backfill from the labels the hooks and services have been writing.
UPDATE checkpoints SET kind = 'auto'
    WHERE label = 'auto' OR label LIKE 'auto-checkpoint%';
UPDATE checkpoints SET kind = 'precompact' WHERE label = 'precompact';
UPDATE checkpoints SET kind = 'handoff' WHERE label = 'handoff';

CREATE INDEX IF NOT EXISTS idx_checkpoints_kind
    ON checkpoints(session_id, kind, created_at);
