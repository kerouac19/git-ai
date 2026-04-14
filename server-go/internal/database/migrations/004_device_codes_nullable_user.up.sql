-- Make user_id and subject_json nullable in oauth_device_codes
-- These fields should only be populated when a user actually authorizes the device code,
-- not at creation time with default values.
ALTER TABLE oauth_device_codes ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE oauth_device_codes ALTER COLUMN subject_json DROP NOT NULL;

-- Clear default data from pending records that were never actually authorized
UPDATE oauth_device_codes
SET user_id = NULL, subject_json = NULL
WHERE status = 'pending';
