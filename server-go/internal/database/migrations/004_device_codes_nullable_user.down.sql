-- Restore NOT NULL constraints on user_id and subject_json
UPDATE oauth_device_codes SET user_id = '' WHERE user_id IS NULL;
UPDATE oauth_device_codes SET subject_json = '{}'::jsonb WHERE subject_json IS NULL;
ALTER TABLE oauth_device_codes ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE oauth_device_codes ALTER COLUMN subject_json SET NOT NULL;
