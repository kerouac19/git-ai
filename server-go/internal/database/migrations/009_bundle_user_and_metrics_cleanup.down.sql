ALTER TABLE metrics_events
    ADD COLUMN IF NOT EXISTS parent_session_id TEXT,
    ADD COLUMN IF NOT EXISTS external_parent_session_id TEXT,
    ADD COLUMN IF NOT EXISTS custom_attributes TEXT;

DROP INDEX IF EXISTS idx_bundles_user_id;

ALTER TABLE bundles
    DROP COLUMN IF EXISTS user_id,
    DROP COLUMN IF EXISTS updated_at;
