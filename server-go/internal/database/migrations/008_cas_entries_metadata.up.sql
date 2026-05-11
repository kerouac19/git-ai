-- Allow CAS uploads to attach arbitrary string metadata (e.g. transcript
-- tags, prompt store labels). Stored as JSONB so future queries can use
-- GIN indexing if needed. Existing rows get NULL — backward compatible.

ALTER TABLE cas_entries
    ADD COLUMN IF NOT EXISTS metadata JSONB;
