# User Org Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a flat `orgs` table and a `users.org_id` foreign key (NOT NULL, with a `研发` default org as sentinel) via a single SQL migration. Zero Go-code changes.

**Architecture:** One new migration file pair (`005_create_orgs.up.sql` / `.down.sql`) embedded via `go:embed` and run on server startup by `golang-migrate/migrate/v4`. The column gets a hardcoded `DEFAULT '<sentinel-uuid>'` so existing `INSERT INTO users (...)` statements (which omit `org_id`) keep working unchanged. Existing rows are auto-backfilled to the sentinel by the `DEFAULT` clause when the column is added.

**Tech Stack:** PostgreSQL 16+, `golang-migrate/migrate/v4` (already wired in `internal/database/migrate.go`), embedded SQL via `migrations/embed.go`.

**Spec:** `docs/superpowers/specs/2026-05-08-user-org-schema-design.md`

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server-go/internal/database/migrations/005_create_orgs.up.sql` | **Create** | Create `orgs`, insert sentinel `研发` row, add `users.org_id NOT NULL DEFAULT REFERENCES orgs(id)`, add index. |
| `server-go/internal/database/migrations/005_create_orgs.down.sql` | **Create** | Drop column, drop index, drop table — reverse order of up. |

No Go files are created or modified. The migration is picked up automatically by `migrations/embed.go` (`//go:embed *.sql`) and applied by `RunMigrations` in `internal/database/migrate.go`.

---

## Pre-flight (do once before Task 1)

- [ ] **Verify branch and clean state**

Run from repo root `/Users/hg/git/git-ai`:

```bash
git status
```

Expected: on branch `server-feature` (or whatever feature branch this is being implemented on). The only diffs should be unrelated to migrations. If `server-go/internal/database/migrations/005_*.sql` already exists, stop and ask the operator.

- [ ] **Verify the next migration number is 005**

```bash
ls server-go/internal/database/migrations/ | grep -E '^[0-9]{3}_' | sort | tail -3
```

Expected output (last three entries):

```
003_create_users.up.sql
004_device_codes_nullable_user.down.sql
004_device_codes_nullable_user.up.sql
```

If anything ≥ 005 already exists, stop and renumber this plan's migration accordingly before proceeding.

- [ ] **Verify local Postgres is reachable**

The `.env` points at `192.168.2.38:5432`, db `git_ai_dev`, user `devops`, password `123456`. Confirm:

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c '\dt'
```

Expected: a list including `users`, `bundles`, etc. If this fails, fix DB connectivity before continuing — every later verification step needs it.

---

## Task 1: Create the up-migration

**Files:**

- Create: `server-go/internal/database/migrations/005_create_orgs.up.sql`

- [ ] **Step 1: Create the file with exact contents below**

```sql
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
```

- [ ] **Step 2: Lint the SQL syntax with `psql --variable=ON_ERROR_STOP=on -c '\i'` against a throwaway transaction**

This dry-runs the up-migration inside a transaction that we then ROLLBACK — it catches syntax errors and FK type mismatches without persisting changes.

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -v ON_ERROR_STOP=1 <<'EOF'
BEGIN;
\i server-go/internal/database/migrations/005_create_orgs.up.sql
ROLLBACK;
EOF
```

Expected: `BEGIN`, `CREATE TABLE`, `INSERT 0 1`, `ALTER TABLE`, `CREATE INDEX`, `ROLLBACK`. No `ERROR:` lines.

If you get `ERROR: relation "users" does not exist`, the dev DB hasn't had migrations 001–004 applied — run the server once (`go run ./cmd/server` from `server-go/`) to apply them, then retry.

If you get `ERROR: column "org_id" of relation "users" already exists`, a prior run of this task partially applied — drop the column manually (`ALTER TABLE users DROP COLUMN org_id; DROP TABLE orgs;`) before retrying.

---

## Task 2: Create the down-migration

**Files:**

- Create: `server-go/internal/database/migrations/005_create_orgs.down.sql`

- [ ] **Step 1: Create the file with exact contents below**

```sql
-- 005_create_orgs.down.sql
-- Reverse of 005_create_orgs.up.sql. The column DROP also drops the FK
-- and the index implicitly; the explicit DROP INDEX is defensive in case
-- the index was somehow recreated against an unrelated column.

ALTER TABLE users DROP COLUMN IF EXISTS org_id;
DROP INDEX IF EXISTS idx_users_org_id;
DROP TABLE IF EXISTS orgs;
```

- [ ] **Step 2: Dry-run the down against a transient up+down cycle**

This applies the up-migration, then the down-migration, and verifies the schema is back to where it started — all inside a single transaction that gets rolled back.

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -v ON_ERROR_STOP=1 <<'EOF'
BEGIN;
\i server-go/internal/database/migrations/005_create_orgs.up.sql
\i server-go/internal/database/migrations/005_create_orgs.down.sql
-- Verify orgs is gone and users.org_id is gone:
SELECT COUNT(*) AS orgs_table_should_be_zero
FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = 'orgs';
SELECT COUNT(*) AS users_org_id_column_should_be_zero
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'org_id';
ROLLBACK;
EOF
```

Expected output (key lines):

```
 orgs_table_should_be_zero
---------------------------
                         0
(1 row)

 users_org_id_column_should_be_zero
------------------------------------
                                  0
(1 row)
ROLLBACK
```

If either count is non-zero, the down-migration is missing something — fix it before continuing.

---

## Task 3: Apply the migration for real and validate end-state

**Files:** none modified. Pure verification.

- [ ] **Step 1: Build the server binary so the embedded FS picks up the new SQL files**

```bash
cd server-go && go build -o bin/git-ai-server ./cmd/server && cd ..
```

Expected: no output, exit code 0, file `server-go/bin/git-ai-server` exists. If build fails, the most likely cause is a typo in the SQL filename (must match `*.sql` glob in `migrations/embed.go`).

- [ ] **Step 2: Run the server briefly so `RunMigrations` applies migration 005**

The migration runner uses `golang-migrate`'s `schema_migrations` table to track applied versions, so applying via the server itself is the right way (raw `psql` would leave that table out of sync).

```bash
cd server-go
set -a; source .env; set +a
./bin/git-ai-server &
SERVER_PID=$!
sleep 4
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
cd ..
```

Expected: server logs show a successful boot (port bind line). The migration runner is silent on success (`golang-migrate` only logs on error). If you see `running migrations: ...`, capture the error message and stop — do not proceed to Task 3 verification.

Alternative if you already run the server under systemd / a process manager on this dev box: just restart it (`systemctl restart git-ai-server` or equivalent). Migrations run on every boot.

- [ ] **Step 2.5: Confirm migration 005 is now in `schema_migrations`**

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c \
    "SELECT version, dirty FROM schema_migrations;"
```

Expected: a single row with `version = 5`, `dirty = f`. If `dirty = t`, the migration partially failed — you must manually clean up (drop any new objects, set `dirty = f`, drop the row, fix the SQL, retry).

- [ ] **Step 3: Verify orgs table exists with the sentinel row**

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c "SELECT id, name FROM orgs;"
```

Expected:

```
                  id                  | name
--------------------------------------+------
 00000000-0000-0000-0000-0000000000a1 | 研发
(1 row)
```

- [ ] **Step 4: Verify all existing users got the sentinel org_id**

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c "SELECT id, username, org_id FROM users LIMIT 10;"
```

Expected: every row has `org_id = 00000000-0000-0000-0000-0000000000a1`. If any user shows `NULL`, the migration didn't apply the DEFAULT correctly — investigate before continuing.

Also run a count check:

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c "SELECT COUNT(*) AS unassigned FROM users WHERE org_id IS NULL;"
```

Expected: `unassigned = 0`.

- [ ] **Step 5: Verify a new INSERT without explicit org_id still works**

This proves existing Go code (`UserService.Create` in `server-go/internal/service/user.go:39-48`) won't break. We simulate exactly what that code does:

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev <<'EOF'
INSERT INTO users (username, password_hash, role, status)
VALUES ('plan_smoketest_user', 'x', 'user', 1);

SELECT username, org_id FROM users WHERE username = 'plan_smoketest_user';

DELETE FROM users WHERE username = 'plan_smoketest_user';
EOF
```

Expected: the SELECT shows `plan_smoketest_user` with `org_id = 00000000-0000-0000-0000-0000000000a1`. The DELETE cleans up.

If the INSERT errors with `null value in column "org_id" violates not-null constraint`, the column is missing its DEFAULT — open the up-migration and confirm the `DEFAULT '...a1'` clause is present.

- [ ] **Step 6: Verify FK rejects bogus org_id**

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev <<'EOF'
INSERT INTO users (username, password_hash, role, status, org_id)
VALUES ('plan_fk_test', 'x', 'user', 1, '00000000-0000-0000-0000-deadbeefdead');
EOF
```

Expected: `ERROR: insert or update on table "users" violates foreign key constraint`. No row inserted. If the INSERT succeeds, the FK was not created — open the up-migration and confirm `REFERENCES orgs(id) ON DELETE RESTRICT`.

- [ ] **Step 7: Verify ON DELETE RESTRICT protects the sentinel org**

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev <<'EOF'
DELETE FROM orgs WHERE id = '00000000-0000-0000-0000-0000000000a1';
EOF
```

Expected: `ERROR: update or delete on table "orgs" violates foreign key constraint ... on table "users"`. No row deleted.

---

## Task 4: Confirm no Go-side regression

**Files:** none. Pure verification.

- [ ] **Step 1: Build everything**

```bash
cd server-go && go build ./... && cd ..
```

Expected: exit code 0, no output.

- [ ] **Step 2: Run existing Go tests**

```bash
cd server-go && go test ./... && cd ..
```

Expected: all green. Tests in this codebase use mocks/fakes at the handler level (see `internal/handler/admin_dashboard_test.go`), so they don't hit the real DB and should be unaffected by this migration. If any test now fails, inspect the failure — there shouldn't be any coupling to `org_id` since we didn't change Go code, but confirm.

- [ ] **Step 3: Confirm `getTopOrgs` admin-dashboard query still no-ops gracefully**

The pre-existing query in `internal/service/admin_dashboard.go:176-219` joins `org_memberships`, which we deliberately did **not** create in this PR. It should still return `[]` via the 42P01 fallback, *not* error.

```bash
PGPASSWORD=123456 psql -h 192.168.2.38 -U devops -d git_ai_dev -c "
SELECT to_regclass('public.org_memberships') AS should_be_null;"
```

Expected: `should_be_null` is empty/`<NULL>`. (If `org_memberships` somehow exists, the dashboard query semantics change — out of scope for this PR but flag to reviewers.)

---

## Task 5: Commit

- [ ] **Step 1: Stage only the two new SQL files**

Important: `git status` will show unrelated dirty files (`server-go/web/vite.config.ts`, untracked `.claude/`). Do **not** include them.

```bash
git add server-go/internal/database/migrations/005_create_orgs.up.sql \
        server-go/internal/database/migrations/005_create_orgs.down.sql
git status
```

Expected: only the two `.sql` files in the staged section.

- [ ] **Step 2: Commit**

```bash
git commit -m "$(cat <<'EOF'
server-go: add orgs table and users.org_id FK

Single migration. Creates orgs(id, name), inserts a sentinel default org
('研发', UUID …a1), and adds users.org_id NOT NULL DEFAULT REFERENCES
orgs(id) ON DELETE RESTRICT. Existing rows are backfilled by the column
DEFAULT, and existing INSERTs that don't mention org_id keep working.

No Go changes — service, handler, and admin dashboard layers are deferred
to follow-up PRs (see spec, section 7).

Spec: docs/superpowers/specs/2026-05-08-user-org-schema-design.md
EOF
)"
```

- [ ] **Step 3: Show the commit to confirm**

```bash
git log --oneline -1 && git show --stat HEAD
```

Expected: one commit with exactly two files added (`005_create_orgs.up.sql`, `005_create_orgs.down.sql`), no other files touched.

---

## Done

After Task 5, the schema work is complete. The follow-up PRs listed in the spec's "Future Work" section can be planned independently.
