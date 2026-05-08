# User Org Schema — Design Spec

- **Status**: Draft
- **Author**: kerouac19
- **Date**: 2026-05-08
- **Scope**: server-go DB schema only (one new migration). No Go, no API, no UI.

## 1. Goal

Give every user an organization affiliation (`org_id`) so future features (admin-org reporting, org-scoped permissions, registration flow) have a stable schema to build on. Today the codebase already references an `orgs` / `org_memberships` schema speculatively in `internal/service/admin_dashboard.go:176-219`, but the tables don't exist.

This change is **schema only** — it lays the foundation. Service/handler/UI changes come in later PRs.

## 2. Non-Goals

- No `OrgService`, no `/api/orgs` handlers, no admin UI.
- No change to `model.User` struct, `UserService.Create`, or `scanUser`. The migration is designed so existing Go code keeps working untouched.
- No change to the `getTopOrgs` admin-dashboard query. It currently joins `org_memberships` (which still doesn't exist after this PR) and silently returns `[]` on `42P01` — that behavior is unchanged.
- No multi-org membership, no org hierarchy. Each user belongs to **exactly one** org. Orgs themselves are flat (no `parent_org_id`).
- No registration-flow changes. Sign-up does not ask for an org; new users land in the default org.

## 3. Design Decisions

| Decision | Choice | Reason |
|---|---|---|
| Cardinality | Single org per user (`users.org_id`) | Simplest. User confirmed orgs will be few. |
| Hierarchy | Flat `orgs` table, no `parent_org_id` | YAGNI; can add later via additive column. |
| Nullability | `org_id NOT NULL` | Stronger invariant from day one. |
| Backfill mechanism | Column `DEFAULT '<sentinel-uuid>'` instead of two-step nullable→backfill→NOT NULL | Lets existing `INSERT INTO users (...)` (which doesn't mention `org_id`) keep working. No Go changes required. |
| Default org name | `研发` | Per user direction. |
| Default org UUID | Fixed sentinel `00000000-0000-0000-0000-0000000000a1` | Stable, code-referenceable, hardcoded as column DEFAULT. Trailing `a1` is deliberately distinct from the bootstrap admin user UUID `…001` (used by `DEFAULT_USER_ID` in `internal/config/config.go:196`) to avoid cross-table confusion. |
| `ON DELETE` | `RESTRICT` | Defensive. Deleting an org with users attached must be an explicit operation. |
| Org name uniqueness | `UNIQUE` on `orgs.name` | Few orgs, names act as identifiers in any future UI. |

## 4. Migration

New files:

- `server-go/internal/database/migrations/005_create_orgs.up.sql`
- `server-go/internal/database/migrations/005_create_orgs.down.sql`

### 4.1 `005_create_orgs.up.sql`

```sql
CREATE TABLE IF NOT EXISTS orgs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sentinel default org. Fixed UUID so it can be referenced as a column DEFAULT
-- and from Go code in later PRs without a name lookup.
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

Notes:

- Adding a `NOT NULL` column with a constant `DEFAULT` is fast on Postgres ≥ 11 (metadata-only, no full table rewrite).
- Existing rows pick up the sentinel via the DEFAULT — no separate `UPDATE` step needed.
- The `INSERT ... ON CONFLICT DO NOTHING` makes the migration idempotent if rerun in a non-transactional context.

### 4.2 `005_create_orgs.down.sql`

```sql
ALTER TABLE users DROP COLUMN IF EXISTS org_id;
DROP INDEX IF EXISTS idx_users_org_id;  -- harmless if column drop already removed it
DROP TABLE IF EXISTS orgs;
```

## 5. Compatibility Audit

Files in `server-go/internal` that touch `users`:

| File / Site | Impact | Action |
|---|---|---|
| `service/user.go` `Create` (INSERT without `org_id`) | DEFAULT supplies sentinel | None |
| `service/user.go` `scanUser` / `FindByID` / `FindByUsernameOrEmail` (SELECT explicit column list) | Doesn't read `org_id` | None |
| `model/user.go` `User` struct | No `OrgID` field | None — added when API/UI layer needs it |
| `service/admin_dashboard.go:176` `getTopOrgs` | Still joins `org_memberships` (non-existent) | None — query already fails safe with `42P01 → []` |
| Other queries on `users` (login, dashboard) | Don't touch `org_id` | None |

Conclusion: zero Go-code changes required. Verifiable by `grep -n org server-go/internal/...go` after the migration runs.

## 6. Testing

- `task build` must pass (no Go changes; sanity check only).
- Run the migration against a local Postgres and verify:
  - `SELECT * FROM orgs;` shows one row, name `研发`, id ending `…a1`.
  - `SELECT id, org_id FROM users LIMIT 5;` shows all existing users now have `org_id = …a1`.
  - Inserting a new user without specifying `org_id` succeeds and gets the sentinel.
- No new Go tests in this PR. Existing `task test` should pass unchanged.

## 7. Future Work (not in this PR)

1. Add `OrgID` to `model.User`; update `scanUser` and `Create` to read/write it. Update existing INSERTs to take an explicit `org_id` parameter.
2. `OrgService` + `GET /api/orgs`, `POST /api/admin/orgs`, `PATCH /api/admin/users/:id` (set org_id).
3. Fix `getTopOrgs` to join `users.org_id` directly instead of the never-created `org_memberships` table.
4. Admin UI: list/create orgs; user detail page exposes org dropdown.
5. Registration flow: optional org selection (defaults to sentinel).
