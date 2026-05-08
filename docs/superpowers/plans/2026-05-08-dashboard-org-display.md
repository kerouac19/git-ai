# Dashboard Org Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface real org information in two existing UI surfaces — the admin activity dashboard's "Top 组织" leaderboard, and the `/me` page's "组织架构" card — by leveraging the `users.org_id` schema shipped in migration 005.

**Architecture:** Two surgical, decoupled pieces. (1) Rewrite the `getTopOrgs` SQL to JOIN `users.org_id` instead of the never-created `org_memberships` table, removing the now-stale 42P01 silent fallback. (2) Add a `UserService.GetUserOrg` JOIN method, wire `UserSvc` into `CompatibilityHandler`, and have `GetMe` include a top-level `org` field; frontend renders `me.org.name` in the existing card and drops the `Slug:` line. JWT and `model.User` stay untouched.

**Tech Stack:** Go (gin, pgx/v5), React + TypeScript (Vite), PostgreSQL 18, no new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-08-dashboard-org-display-design.md`

**Depends on:** Migration 005 already applied (commit `c01a9c0f` adds `users.org_id` and the sentinel `研发` org). Local DB at `127.0.0.1:5432 / git_ai_dev / devops:123456` is currently the dev target (the `.env` has been pointed at it; the remote at 192.168.2.38 still lacks the GRANT).

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `server-go/internal/service/admin_dashboard.go` | **Modify** lines 176-219 (replace `getTopOrgs`) | Real Top-N orgs query via `users.org_id` JOIN; surfaces real DB errors |
| `server-go/internal/service/user.go` | **Modify** (append new method) | New `GetUserOrg(ctx, userID) → (id, name, error)` |
| `server-go/internal/handler/compatibility.go` | **Modify** struct (line 17-25) + `GetMe` (lines 52-95) | Add `UserSvc *service.UserService` field; call `GetUserOrg`; add top-level `org` to JSON response |
| `server-go/cmd/server/main.go` | **Modify** ~line 96-104 | Pass `UserSvc: userSvc` when constructing `CompatibilityHandler` |
| `server-go/web/src/types/api.ts` | **Modify** `MeApiResponse` (lines 15-21) | Add `org?: { id: string; name: string }` field |
| `server-go/web/src/routes/Me.tsx` | **Modify** lines 20, 54-77 | Read `me.org` from response; remove `Slug:` line; render `org.name` |
| `server-go/web/src/styles/globals.css` | **Modify** lines 341-345 | Remove now-dead `.me-page__org-slug` CSS rule |

No new files. No new tests (the codebase's handler tests mock services; introducing a `UserSvc` interface to mock for one call is more refactoring than the spec authorizes — manual + curl verification is the convention here).

---

## Pre-flight (do once before Task 1)

- [ ] **Verify branch and DB state**

```bash
git status
git log --oneline -3
PGPASSWORD=123456 psql -h 127.0.0.1 -U devops -d git_ai_dev -c "SELECT version, dirty FROM schema_migrations; SELECT id, name FROM orgs;"
```

Expected:
- Branch: `server-feature` (or whatever feature branch is current).
- Most recent commits include `3fa5f489 docs: spec for surfacing real org…`.
- `schema_migrations` shows `version=5, dirty=f`.
- `orgs` shows one row: `00000000-…-a1 / 研发`.

If `schema_migrations.version` is not 5, stop — Task 1 won't see the right DB state. Re-apply migration 005 by booting the server briefly.

- [ ] **Verify the relevant lines exist where the plan claims**

```bash
sed -n '176,180p' server-go/internal/service/admin_dashboard.go
sed -n '52,55p' server-go/internal/handler/compatibility.go
sed -n '15,21p' server-go/web/src/types/api.ts
sed -n '20,21p' server-go/web/src/routes/Me.tsx
```

Expected: line 176 of `admin_dashboard.go` is the start of `func (s *AdminDashboardService) getTopOrgs`, line 52 of `compatibility.go` is `func (h *CompatibilityHandler) GetMe(c *gin.Context) {`, etc. If line numbers have drifted, locate by content not line number when editing — but the task instructions paste the entire replacement block, so drift is not blocking.

---

## Task 1: Fix `getTopOrgs` to JOIN `users.org_id`

**Files:**
- Modify: `server-go/internal/service/admin_dashboard.go` (replace function `getTopOrgs`, lines 176-219)

- [ ] **Step 1: Replace the entire `getTopOrgs` function with the new implementation**

Open `server-go/internal/service/admin_dashboard.go`. Find the function `func (s *AdminDashboardService) getTopOrgs(ctx context.Context, days int) ([]model.AdminTopOrg, error) {`. Replace its entire body (function + closing `}`) with:

```go
func (s *AdminDashboardService) getTopOrgs(ctx context.Context, days int) ([]model.AdminTopOrg, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			o.id::text,
			coalesce(o.name, '') as org_name,
			coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
			count(distinct e.user_id) as member_count
		from public.metrics_events e
		join public.users u on u.id::text = e.user_id
		join public.orgs o on o.id = u.org_id
		where e.event_timestamp >= now() - interval '%d days'
		group by o.id, o.name
		order by prompt_count desc, member_count desc, o.id asc
		limit 10
	`, days))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("admin top orgs: %w", err)
	}
	defer rows.Close()

	out := make([]model.AdminTopOrg, 0, 10)
	for rows.Next() {
		var o model.AdminTopOrg
		if err := rows.Scan(&o.OrgID, &o.OrgName, &o.PromptCount, &o.MemberCount); err != nil {
			return nil, fmt.Errorf("admin top orgs scan: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
```

What's different versus the old version (for reviewer awareness — do NOT keep both):

- Two `JOIN` lines flipped: `org_memberships` → `users` on `e.user_id`; second join's `m.org_id` → `u.org_id`.
- Removed the `pgconn.PgError` / `42P01` silent-empty fallback. Real DB errors now propagate.
- Removed the misleading "Best-effort" / "schema-drift" comments.
- Kept the `errors.Is(err, context.Canceled)` early exit — still legitimate.

- [ ] **Step 2: Remove now-unused imports**

The `pgconn` and `log` imports were used **only** by the old fallback. They may now be unused. Run:

```bash
cd server-go && goimports -w internal/service/admin_dashboard.go && cd ..
```

If `goimports` isn't installed (`command not found`), `go build ./...` will report unused imports and you can remove them by hand.

- [ ] **Step 3: Verify compile**

```bash
cd server-go && go build ./... && cd ..
```

Expected: exit 0, no output. If you see `imported and not used: "github.com/jackc/pgx/v5/pgconn"` or similar, remove that import line and re-run.

- [ ] **Step 4: Verify the SQL itself runs against the local DB**

This is the SQL the new code emits with `days=7`:

```bash
PGPASSWORD=123456 psql -h 127.0.0.1 -U devops -d git_ai_dev -c "
select
    o.id::text,
    coalesce(o.name, '') as org_name,
    coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
    count(distinct e.user_id) as member_count
from public.metrics_events e
join public.users u on u.id::text = e.user_id
join public.orgs o on o.id = u.org_id
where e.event_timestamp >= now() - interval '7 days'
group by o.id, o.name
order by prompt_count desc, member_count desc, o.id asc
limit 10;"
```

Expected: one row, org_name `研发`, with non-zero `prompt_count` and `member_count` (the seeded DB has 44k metrics events). If you get `relation "users" does not exist` or `column "org_id" does not exist`, migration 005 isn't applied — abort and check pre-flight.

- [ ] **Step 5: End-to-end check via the running server**

Start the server, log in as admin, hit the admin dashboard endpoint:

```bash
cd server-go
set -a; source .env; set +a
./bin/git-ai-server &
SERVER_PID=$!
sleep 3
# Log in as admin (from .env: INITIAL_ADMIN_USERNAME=admin, INITIAL_ADMIN_PASSWORD=admin1234)
TOKEN=$(curl -s -X POST http://localhost:3000/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"login":"admin","password":"admin1234"}' \
    | python3 -c "import sys,json;print(json.load(sys.stdin).get('token',''))")
echo "TOKEN length: ${#TOKEN}"
# Hit the admin endpoint
curl -s "http://localhost:3000/api/admin/dashboard/stats?range=7d" \
    -H "Authorization: Bearer $TOKEN" | python3 -m json.tool | grep -A 5 '"topOrgs"'
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
cd ..
```

Expected: the `topOrgs` array contains one element with `"orgName": "研发"`, plus a non-zero `promptCount` and `memberCount`. If `topOrgs` is empty, the server is hitting an error — capture stderr from the server and inspect.

If your login route differs from `/api/auth/login` (check `cmd/server/main.go` if needed) or if your binary is serving on a different port, adapt accordingly. The point of the check is "topOrgs has a real row, no error".

- [ ] **Step 6: Commit**

```bash
git add server-go/internal/service/admin_dashboard.go
git status
```

Expected: only that one file in the staged set. Then:

```bash
git commit -m "$(cat <<'EOF'
server-go: fix getTopOrgs to JOIN users.org_id

Now that migration 005 has shipped real orgs and users.org_id, replace
the org_memberships placeholder JOIN with the real schema. Drops the
42P01 silent fallback — real DB errors should be loud post-migration,
not masked.

The "Top 组织" leaderboard on /admin/activity now renders actual data
instead of always being empty.

Spec: docs/superpowers/specs/2026-05-08-dashboard-org-display-design.md
EOF
)"
```

---

## Task 2: Add `UserService.GetUserOrg`

**Files:**
- Modify: `server-go/internal/service/user.go` (append new method at end of file)

- [ ] **Step 1: Append the new method**

Open `server-go/internal/service/user.go`. After the existing `scanUser` function (the last function in the file, around line 68-81), append this method:

```go
// GetUserOrg returns the user's org id and name via JOIN. Returns ("", "", nil)
// if the user does not exist (caller decides how to handle missing user — typically
// already gated by JWT auth so this is rare).
func (s *UserService) GetUserOrg(ctx context.Context, userID string) (string, string, error) {
	var orgID, orgName string
	err := s.Pool.QueryRow(ctx, `
		SELECT o.id::text, o.name
		FROM users u
		JOIN orgs o ON o.id = u.org_id
		WHERE u.id = $1`, userID).Scan(&orgID, &orgName)
	if err == pgx.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("getting user org: %w", err)
	}
	return orgID, orgName, nil
}
```

The `pgx` import is already present in this file (line 9). No new imports needed.

- [ ] **Step 2: Verify compile**

```bash
cd server-go && go build ./... && cd ..
```

Expected: exit 0.

- [ ] **Step 3: Verify the JOIN works against the local DB by simulating the call manually**

```bash
ADMIN_ID=$(PGPASSWORD=123456 psql -h 127.0.0.1 -U devops -d git_ai_dev -tA -c "SELECT id FROM users WHERE username='admin' LIMIT 1;")
echo "admin id = $ADMIN_ID"
PGPASSWORD=123456 psql -h 127.0.0.1 -U devops -d git_ai_dev -c "
SELECT o.id::text, o.name
FROM users u
JOIN orgs o ON o.id = u.org_id
WHERE u.id = '$ADMIN_ID';"
```

Expected: one row, `id = 00000000-…-a1`, `name = 研发`. If empty, something is very wrong with the seeded data — check that `admin` user exists and has `org_id` set.

- [ ] **Step 4: Stage changes (DO NOT COMMIT YET)**

```bash
git add server-go/internal/service/user.go
git status
```

The commit happens at the end of Task 3, bundling the service method with its handler consumer.

---

## Task 3: Wire `UserSvc` into `CompatibilityHandler` and surface `org` from `GetMe`

**Files:**
- Modify: `server-go/internal/handler/compatibility.go` (struct lines 17-25; `GetMe` lines 52-95)
- Modify: `server-go/cmd/server/main.go` (handler construction near line 96-104)

- [ ] **Step 1: Add `UserSvc` field to `CompatibilityHandler` struct**

Open `server-go/internal/handler/compatibility.go`. The struct currently looks like:

```go
type CompatibilityHandler struct {
	DashboardSvc  *service.DashboardService
	AuthorshipSvc *service.AuthorshipService
	CasSvc        *service.CasService
	DeviceFlowSvc *auth.DeviceFlowService
	MetricsSvc    *service.MetricsService
	TrustProxy    bool
	Commit        string
}
```

Add `UserSvc *service.UserService` after `MetricsSvc`:

```go
type CompatibilityHandler struct {
	DashboardSvc  *service.DashboardService
	AuthorshipSvc *service.AuthorshipService
	CasSvc        *service.CasService
	DeviceFlowSvc *auth.DeviceFlowService
	MetricsSvc    *service.MetricsService
	UserSvc       *service.UserService
	TrustProxy    bool
	Commit        string
}
```

- [ ] **Step 2: Modify `GetMe` to fetch and return the real org**

Still in `compatibility.go`, locate `GetMe` (around line 52-95). The current end of the function is:

```go
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"id":              userID,
			"email":           userMap["email"],
			"name":            userMap["name"],
			"role":            userMap["role"],
			"personal_org_id": userMap["personal_org_id"],
			"orgs":            userMap["orgs"],
		},
		"dashboard":              dashboard,
		"recentAuthorship":       records,
		"totalAuthorshipRecords": total,
	})
}
```

Insert the `GetUserOrg` call **after** the existing `records, total, err := h.AuthorshipSvc.FindAll(...)` block and **before** the `c.JSON(...)` call, and add the `"org"` field to the JSON payload:

```go
	orgID, orgName, err := h.UserSvc.GetUserOrg(c.Request.Context(), userID)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"id":              userID,
			"email":           userMap["email"],
			"name":            userMap["name"],
			"role":            userMap["role"],
			"personal_org_id": userMap["personal_org_id"],
			"orgs":            userMap["orgs"],
		},
		"dashboard":              dashboard,
		"recentAuthorship":       records,
		"totalAuthorshipRecords": total,
		"org": gin.H{
			"id":   orgID,
			"name": orgName,
		},
	})
}
```

- [ ] **Step 3: Wire `UserSvc` in `cmd/server/main.go`**

Open `server-go/cmd/server/main.go`. The current handler construction (near line 96) looks like:

```go
	compatH := &handler.CompatibilityHandler{
		DashboardSvc:  dashboardSvc,
		AuthorshipSvc: authorshipSvc,
		CasSvc:        casSvc,
		DeviceFlowSvc: deviceFlowSvc,
		MetricsSvc:    metricsSvc,
		TrustProxy:    trustProxy,
		Commit:        commitHash,
	}
```

Add the `UserSvc: userSvc,` line (the `userSvc` instance is already declared earlier in this function, line 75):

```go
	compatH := &handler.CompatibilityHandler{
		DashboardSvc:  dashboardSvc,
		AuthorshipSvc: authorshipSvc,
		CasSvc:        casSvc,
		DeviceFlowSvc: deviceFlowSvc,
		MetricsSvc:    metricsSvc,
		UserSvc:       userSvc,
		TrustProxy:    trustProxy,
		Commit:        commitHash,
	}
```

- [ ] **Step 4: Verify compile**

```bash
cd server-go && go build ./... && cd ..
```

Expected: exit 0. If you get `unknown field UserSvc in struct literal of type handler.CompatibilityHandler`, you missed Step 1 in `compatibility.go`. If you get `compatH.UserSvc undefined`, Step 1 has the field but it's misspelled.

- [ ] **Step 5: Run existing Go tests to confirm no regression**

```bash
cd server-go && go test ./... && cd ..
```

Expected: all pass. There are no `GetMe` tests today (verified via `grep -n TestGetMe internal/handler/*_test.go` returning nothing), so the new struct field doesn't break any test setup. If a test does fail, it's likely on a different file — read the failure carefully.

- [ ] **Step 6: End-to-end check `/api/me` returns the new field**

```bash
cd server-go
set -a; source .env; set +a
./bin/git-ai-server &
SERVER_PID=$!
sleep 3

# Rebuild the binary first to pick up Go changes from Tasks 2 and 3
# (the running binary above might be the old Task-1 build)
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
go build -o bin/git-ai-server ./cmd/server
./bin/git-ai-server &
SERVER_PID=$!
sleep 3

TOKEN=$(curl -s -X POST http://localhost:3000/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"login":"admin","password":"admin1234"}' \
    | python3 -c "import sys,json;print(json.load(sys.stdin).get('token',''))")
curl -s http://localhost:3000/api/me \
    -H "Authorization: Bearer $TOKEN" \
    | python3 -m json.tool | grep -A 3 '"org"'

kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
cd ..
```

Expected: the response includes a top-level `org` object — for example:

```
    "org": {
        "id": "00000000-0000-0000-0000-0000000000a1",
        "name": "研发"
    }
```

If `org` is missing entirely, the binary wasn't rebuilt — re-run `go build -o bin/git-ai-server ./cmd/server`. If `org.id` is empty string, `GetUserOrg` returned `ErrNoRows` — check that the JWT's `id` claim matches a real user row.

- [ ] **Step 7: Commit Tasks 2 + 3 together**

```bash
git add server-go/internal/service/user.go \
        server-go/internal/handler/compatibility.go \
        server-go/cmd/server/main.go
git status
```

Expected: those three files staged, nothing else (the `bin/` binary is gitignored). Then:

```bash
git commit -m "$(cat <<'EOF'
server-go: surface real org on /api/me

Add UserService.GetUserOrg (one JOIN), wire UserSvc into
CompatibilityHandler, and have GetMe include a top-level "org" field
sourced from users.org_id → orgs(name). The legacy user.personal_org_id
and user.orgs[] JWT-derived fields are unchanged for backward compat.

Spec: docs/superpowers/specs/2026-05-08-dashboard-org-display-design.md
EOF
)"
```

---

## Task 4: Frontend — thread `org` through, render real org on `/me`, drop the Slug line

**Files:**
- Modify: `server-go/web/src/types/api.ts` (lines 15-21, `MeApiResponse`)
- Modify: `server-go/web/src/hooks/useMe.ts` (state union + fetch handler)
- Modify: `server-go/web/src/components/ProtectedRoute.tsx` (render-prop signature + pass-through)
- Modify: `server-go/web/src/routes/Me.tsx` (`MeContent` props + line 20 + lines 54-77)
- Modify: `server-go/web/src/styles/globals.css` (remove `.me-page__org-slug`)

The data flow today: `useMe` fetches `/api/me`, drops everything except `{user, dashboard}` from state; `ProtectedRoute` exposes those two via render-prop; `Me` and `AdminActivity` destructure them. To get `org` to `Me`, we thread it through all three hops as **optional**, so `AdminActivity` (which only reads `user`) doesn't need to change.

- [ ] **Step 1: Extend `MeApiResponse` with the optional `org` field**

Open `server-go/web/src/types/api.ts`. The existing `MeApiResponse` (lines 15-21) is:

```ts
export interface MeApiResponse {
  success: boolean;
  user: User;
  dashboard: DashboardStats;
  recentAuthorship: unknown[];
  totalAuthorshipRecords: number;
}
```

Add a final `org?` line:

```ts
export interface MeApiResponse {
  success: boolean;
  user: User;
  dashboard: DashboardStats;
  recentAuthorship: unknown[];
  totalAuthorshipRecords: number;
  org?: { id: string; name: string };
}
```

`org` is optional so a response from an older server (without the new field) still type-checks during deploy windows.

- [ ] **Step 2: Plumb `org` through `useMe.ts`**

Open `server-go/web/src/hooks/useMe.ts`. Two changes:

(a) Add `org?` to the `authenticated` variant of the `State` union (line 7). Replace:

```ts
  | { status: "authenticated"; user: User; dashboard: DashboardStats }
```

with:

```ts
  | { status: "authenticated"; user: User; dashboard: DashboardStats; org?: { id: string; name: string } }
```

(b) Pass `org` through when the fetch resolves (line 19). Replace:

```ts
        setState({ status: "authenticated", user: res.user, dashboard: res.dashboard });
```

with:

```ts
        setState({ status: "authenticated", user: res.user, dashboard: res.dashboard, org: res.org });
```

- [ ] **Step 3: Plumb `org` through `ProtectedRoute.tsx`**

Open `server-go/web/src/components/ProtectedRoute.tsx`. Two changes:

(a) Extend the render-prop signature in the `Props` interface. Replace:

```ts
interface Props {
  children: (data: { user: User; dashboard: DashboardStats }) => ReactNode;
}
```

with:

```ts
interface Props {
  children: (data: { user: User; dashboard: DashboardStats; org?: { id: string; name: string } }) => ReactNode;
}
```

(b) Pass `org` through at the bottom. Replace:

```ts
  return <>{children({ user: state.user, dashboard: state.dashboard })}</>;
```

with:

```ts
  return <>{children({ user: state.user, dashboard: state.dashboard, org: state.org })}</>;
```

`AdminActivity.tsx` uses `{({ user }) => <AdminActivityContent user={user} />}` — destructuring only `user`, so the new optional field doesn't break that consumer.

- [ ] **Step 4: Update `Me.tsx` to consume `org` and drop the Slug line**

Open `server-go/web/src/routes/Me.tsx`. Three changes:

(a) `MeContent` props (line 6). Replace:

```tsx
function MeContent({ user, dashboard }: { user: User; dashboard: DashboardStats }) {
```

with:

```tsx
function MeContent({ user, dashboard, org }: { user: User; dashboard: DashboardStats; org?: { id: string; name: string } }) {
```

(b) Drop the legacy lookup at line 20. Replace:

```tsx
  const org     = user.orgs?.[0];
```

with: nothing — delete this line entirely. `org` is now a prop.

(c) Render `org.name` and remove the slug paragraph. The current card (lines 54-77) reads `org.org_name` / `org.org_slug`. Replace the `{org ? (` JSX block with:

```tsx
          {org ? (
            <div className="card me-page__org-card">
              <h2>组织架构</h2>
              <div className="me-page__org-info">
                <div className="me-page__org-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                    strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
                    style={{ color: "var(--accent)" }}>
                    <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                    <polyline points="9 22 9 12 15 12 15 22" />
                  </svg>
                </div>
                <div>
                  <p className="me-page__org-name">{org.name}</p>
                </div>
              </div>
            </div>
          ) : (
            <div className="card me-page__org-card">
              <h2>组织架构</h2>
              <p className="muted" style={{ margin: 0 }}>暂无组织</p>
            </div>
          )}
```

What's different versus the old version:

- `org.org_name` → `org.name`.
- `<p className="me-page__org-slug">Slug: {org.org_slug}</p>` deleted.
- The `org ?` truthiness check works the same way (undefined still triggers the "暂无组织" branch).

(d) Update the `Me` default-export render-prop (lines 210-216) to forward `org`:

```tsx
export default function Me() {
  return (
    <ProtectedRoute>
      {({ user, dashboard, org }) => <MeContent user={user} dashboard={dashboard} org={org} />}
    </ProtectedRoute>
  );
}
```

- [ ] **Step 5: Remove the dead `.me-page__org-slug` CSS rule**

Open `server-go/web/src/styles/globals.css`. Find lines 341-345:

```css
.me-page__org-slug {
  color: var(--muted);
  font-size: 0.75rem;
  margin: 0;
}
```

Delete those 5 lines (and the trailing blank line that separates the rule from the next one, if appropriate). Verify the rule isn't used elsewhere first:

```bash
grep -rn "me-page__org-slug" server-go/web/src/
```

Expected after Step 4: no matches in `.tsx` files (we deleted the only usage), only the CSS rule itself. After Step 5: no matches at all.

- [ ] **Step 6: TypeScript / build check**

```bash
cd server-go/web && npm run build && cd ../..
```

Expected: clean build, no type errors. If you get a TS error about `org` being undefined, check that the import paths and the destructuring pattern in Steps 2-4 match your actual hook/component shape.

- [ ] **Step 7: Visual verification in the browser**

```bash
cd server-go/web && npm run dev &
DEV_PID=$!
cd ../..
# wait for vite to be up
sleep 4
echo "Vite dev server is on http://localhost:5173 (or whatever Vite picked)."
echo "Manually:"
echo "  1. Open http://localhost:5173/me"
echo "  2. Log in (admin / admin1234) if not already"
echo "  3. The 组织架构 card should show 研发 and NO Slug line"
echo ""
echo "When done verifying: kill $DEV_PID"
```

Stop the dev server when done (`kill <pid>`).

Expected: `/me` renders the card with `研发` as the org name, no `Slug: …` line beneath it. If the card shows "暂无组织", check the network tab for the `/api/me` response — it should include the `org` field.

- [ ] **Step 8: Commit**

```bash
git add server-go/web/src/types/api.ts \
        server-go/web/src/hooks/useMe.ts \
        server-go/web/src/components/ProtectedRoute.tsx \
        server-go/web/src/routes/Me.tsx \
        server-go/web/src/styles/globals.css
git status
```

Expected: those five files staged. Then:

```bash
git commit -m "$(cat <<'EOF'
server-go/web: render real org on /me page

Read the new top-level `org` field from /api/me (id + name) instead of
the legacy pseudo-org-as-user shim in user.orgs[0]. Drop the Slug line
since real orgs only have id + name; remove the now-unused
.me-page__org-slug CSS rule.

Spec: docs/superpowers/specs/2026-05-08-dashboard-org-display-design.md
EOF
)"
```

---

## Task 5: Final regression check + plan commit

- [ ] **Step 1: Full backend build + tests one more time**

```bash
cd server-go && go build ./... && go test ./... && cd ..
```

Expected: green.

- [ ] **Step 2: Frontend build one more time**

```bash
cd server-go/web && npm run build && cd ../..
```

Expected: green.

- [ ] **Step 3: Commit this plan document**

```bash
git add docs/superpowers/plans/2026-05-08-dashboard-org-display.md
git commit -m "$(cat <<'EOF'
docs: implementation plan for dashboard org display

Subagent-driven execution plan for the changes described in
docs/superpowers/specs/2026-05-08-dashboard-org-display-design.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Or, if the operator prefers the plan as a `--amend` to one of the feature commits — confirm before doing so.)

- [ ] **Step 4: Show the final commit graph**

```bash
git log --oneline -8
```

Expected (top of list):

```
<sha> docs: implementation plan for dashboard org display
<sha> server-go/web: render real org on /me page
<sha> server-go: surface real org on /api/me
<sha> server-go: fix getTopOrgs to JOIN users.org_id
3fa5f489 docs: spec for surfacing real org in admin & me dashboards
c01a9c0f server-go: add orgs table and users.org_id FK
…
```

---

## Done

After Task 5, the dashboard surfaces real org info in both places. The follow-up PRs listed in the spec §8 (model.User.OrgID, OrgService, JWT replacement, admin UI, registration org selection) remain explicitly out of scope and can be planned independently.
