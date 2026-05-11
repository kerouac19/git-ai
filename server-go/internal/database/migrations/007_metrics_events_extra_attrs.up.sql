-- Extend metrics_events with the attrs the client has been writing since 1.4.6+
-- but the server has never extracted into dedicated columns. Also drop the
-- prompt_id column (client tombstoned attr index 22; server queries already
-- read attrs_json->>'22', no column reader exists).
--
-- Idempotent so a fresh DB (where 001 already builds the final shape) and an
-- upgraded DB (where 001 was the old shape and 007 fills the delta) converge.

ALTER TABLE metrics_events
    ADD COLUMN IF NOT EXISTS author                       TEXT,
    ADD COLUMN IF NOT EXISTS commit_sha                   TEXT,
    ADD COLUMN IF NOT EXISTS base_commit_sha              TEXT,
    ADD COLUMN IF NOT EXISTS branch                       TEXT,
    ADD COLUMN IF NOT EXISTS session_id                   TEXT,
    ADD COLUMN IF NOT EXISTS trace_id                     TEXT,
    ADD COLUMN IF NOT EXISTS parent_session_id            TEXT,
    ADD COLUMN IF NOT EXISTS external_parent_session_id   TEXT,
    ADD COLUMN IF NOT EXISTS custom_attributes            TEXT;

ALTER TABLE metrics_events
    DROP COLUMN IF EXISTS prompt_id;

CREATE INDEX IF NOT EXISTS idx_metrics_events_session_id ON metrics_events (session_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_branch     ON metrics_events (branch);
