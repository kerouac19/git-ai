-- Reverse 007. Brings prompt_id back as a nullable TEXT column (data not
-- preserved — drop was a clean cut; only the column shape is restored).

DROP INDEX IF EXISTS idx_metrics_events_branch;
DROP INDEX IF EXISTS idx_metrics_events_session_id;

ALTER TABLE metrics_events
    DROP COLUMN IF EXISTS custom_attributes,
    DROP COLUMN IF EXISTS external_parent_session_id,
    DROP COLUMN IF EXISTS parent_session_id,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS session_id,
    DROP COLUMN IF EXISTS branch,
    DROP COLUMN IF EXISTS base_commit_sha,
    DROP COLUMN IF EXISTS commit_sha,
    DROP COLUMN IF EXISTS author;

ALTER TABLE metrics_events
    ADD COLUMN IF NOT EXISTS prompt_id TEXT;
