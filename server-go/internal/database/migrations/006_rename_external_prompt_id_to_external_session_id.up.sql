-- Align column name with client attr index 23 (EXTERNAL_SESSION_ID).
-- The column was originally named external_prompt_id but the client always
-- writes attr index 23 as external_session_id; this rename fixes the semantic
-- drift. No data conversion needed.
--
-- Idempotent: skip when the rename was already applied (e.g. a database
-- created from the updated 001 migration where the column is already named
-- external_session_id).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'metrics_events'
          AND column_name = 'external_prompt_id'
    ) THEN
        ALTER TABLE metrics_events RENAME COLUMN external_prompt_id TO external_session_id;
    END IF;
END $$;
