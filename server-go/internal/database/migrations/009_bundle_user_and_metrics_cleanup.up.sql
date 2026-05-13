-- Converge upgraded databases with schema.sql:
--   * bundles now track the creating user and an updated_at timestamp.
--   * metrics_events keeps parent/custom attrs in attrs_json only; dedicated
--     columns are no longer written by the service.

ALTER TABLE bundles
    ADD COLUMN IF NOT EXISTS user_id TEXT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

UPDATE bundles
SET user_id = 'unknown'
WHERE user_id IS NULL OR btrim(user_id) = '';

UPDATE bundles
SET updated_at = created_at
WHERE updated_at IS NULL;

ALTER TABLE bundles
    ALTER COLUMN user_id SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bundles_user_id ON bundles (user_id);

ALTER TABLE metrics_events
    DROP COLUMN IF EXISTS parent_session_id,
    DROP COLUMN IF EXISTS external_parent_session_id,
    DROP COLUMN IF EXISTS custom_attributes;
