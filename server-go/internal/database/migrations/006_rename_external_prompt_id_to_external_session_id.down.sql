DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'metrics_events'
          AND column_name = 'external_session_id'
    ) THEN
        ALTER TABLE metrics_events RENAME COLUMN external_session_id TO external_prompt_id;
    END IF;
END $$;
