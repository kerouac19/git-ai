-- 005_create_orgs.up.sql
-- Adds a flat orgs table and links every user to exactly one org via
-- users.org_id. A sentinel default org ('研发', UUID …a1) lets the column
-- default backfill all existing users and lets future Go code reference
-- the default org by a stable UUID without a name lookup.

CREATE TABLE IF NOT EXISTS orgs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sentinel default org. Trailing 'a1' is deliberately distinct from the
-- bootstrap admin user UUID …001 (DEFAULT_USER_ID, see config.go:196).
INSERT INTO orgs (id, name)
VALUES ('00000000-0000-0000-0000-0000000000a1', '研发')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE users
    ADD COLUMN org_id UUID
        NOT NULL
        DEFAULT '00000000-0000-0000-0000-0000000000a1'
        REFERENCES orgs(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_users_org_id ON users (org_id);
