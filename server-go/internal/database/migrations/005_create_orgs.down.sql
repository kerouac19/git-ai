-- 005_create_orgs.down.sql
-- Reverse of 005_create_orgs.up.sql. The column DROP also drops the FK
-- and the index implicitly; the explicit DROP INDEX is defensive in case
-- the index was somehow recreated against an unrelated column.

ALTER TABLE users DROP COLUMN IF EXISTS org_id;
DROP INDEX IF EXISTS idx_users_org_id;
DROP TABLE IF EXISTS orgs;
