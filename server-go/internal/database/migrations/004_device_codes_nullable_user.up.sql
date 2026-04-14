-- Make user_id and subject_json nullable in oauth_device_codes
-- These fields should only be populated when a user actually authorizes the device code,
-- not at creation time with default values.
--
-- NOTE: We intentionally do NOT backfill existing pending rows to NULL here.
-- Old server instances still running during a rolling deploy expect non-null values.
-- Existing rows will naturally expire; new rows created by the updated code will be NULL.
ALTER TABLE oauth_device_codes ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE oauth_device_codes ALTER COLUMN subject_json DROP NOT NULL;
