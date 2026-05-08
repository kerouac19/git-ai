# Dashboard Org Display — Design Spec

- **Status**: Draft
- **Author**: kerouac19
- **Date**: 2026-05-08
- **Scope**: server-go (Go API) + server-go/web (React SPA)
- **Depends on**: `2026-05-08-user-org-schema-design.md` (migration 005, already shipped — adds `users.org_id` + `orgs(id, name)`).

## 1. Goal

Surface organization affiliation in two places where users already look:

1. **Admin activity dashboard** (`/admin/activity`) — the **Top 组织** leaderboard currently always renders empty because the backend stub queries a non-existent `org_memberships` table. Make it actually return data using the real `users.org_id` schema.
2. **User profile page** (`/me`) — the **组织架构** card currently renders the legacy "pseudo-org" derived from the user themselves (org_id == user_id, org_name == display_name). Make it render the user's real org from `orgs` via `users.org_id`.

## 2. Non-Goals

- **No JWT/auth changes.** `auth.TokenSubject`, `userToSubject` (`internal/handler/login.go:163-183`), and the `personal_org_id` / `orgs` JWT claims keep their legacy "pseudo-org-as-user" shape. Touching them affects token shape, requires re-issue, and ripples beyond the dashboard scope.
- **No `model.User.OrgID` field.** No SELECT-list changes to `FindByID` / `FindByUsernameOrEmail` / `scanUser`. The two queries we add are localized to the new dashboard/me code paths.
- **No org-management UI/API** (no list/create/rename orgs, no admin-edit-user-org). Future PRs.
- **No Top 用户 column for org.** Top users leaderboard stays as `用户 / Prompt / AI 行数`.
- **No `slug` column on orgs.** Schema stays `(id, name, created_at, updated_at)`. The existing `Slug: …` line on the `/me` org card is removed.
- **No registration-flow changes.** New users still default to the sentinel org (`研发`) via the column DEFAULT.
- **No caching.** One extra DB round-trip on each `/me` is acceptable; if it ever shows up in profiling, that's a future optimization.

## 3. Architecture

```
                                        ┌────────────────────────┐
GET /api/admin/dashboard/stats          │ AdminDashboardService  │
    └── jwtMW → requireAdmin            │   getTopOrgs(...)      │
        └── AdminDashboardHandler ─────▶│   (rewritten SQL:      │
                                        │    users.org_id JOIN)  │
                                        └────────────────────────┘

                                        ┌────────────────────────┐
GET /api/me                             │ CompatibilityHandler   │
    └── jwtMW                           │   GetMe(...)           │
        └── CompatibilityHandler ──────▶│     ▸ existing logic   │
                                        │     ▸ NEW: UserService │
                                        │       .GetUserOrg()    │
                                        │     ▸ adds top-level   │
                                        │       "org" field      │
                                        └────────────────────────┘
```

**Boundaries:**

- The `getTopOrgs` change lives entirely inside the existing service method — no new methods, no new types, no new routes.
- The `/me` change adds **one** new method to `UserService` (`GetUserOrg`) and **one** new top-level field (`org`) on the response. The legacy `user.personal_org_id` and `user.orgs[]` fields stay populated as before for backward compat with any caller still reading the old shape.

## 4. Backend Changes

### 4.1 `internal/service/admin_dashboard.go` — fix `getTopOrgs`

Current SQL joins `org_memberships` (which doesn't exist) and silently returns `[]` on `42P01` (undefined_table). Now that the real schema is shipped, the silent fallback is misleading — it would also silence real bugs. Replace it.

**New SQL (full method body):**

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

**What changed line by line:**

- Two `JOIN` lines: `org_memberships` → `users` on `e.user_id`; `m.org_id` → `u.org_id`.
- Removed the `pgErr.Code == "42P01"` silent-empty fallback. Real DB errors now propagate as `admin top orgs: <err>` and the dashboard endpoint returns 500 — which is the right behavior post-schema-ship.
- Removed the misleading "Best-effort" / "schema isn't deployed yet" comments at the top.
- Kept `context.Canceled` early-exit (still legitimate when the errgroup unwinds).
- Removed unused `pgconn`, `log` imports (verify with `goimports`/`go build`).

### 4.2 `internal/service/user.go` — new `GetUserOrg` method

Append to the existing file:

```go
// GetUserOrg returns the user's org id and name via JOIN. Returns (id, name, nil)
// on success, or ("", "", nil) if the user does not exist (caller decides how to
// handle missing user — typically already gated by JWT auth so this is rare).
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

The JOIN is safe: `users.org_id` is `NOT NULL` and FK-constrained to `orgs.id`, so any extant user row is guaranteed to JOIN. The `pgx.ErrNoRows` branch only triggers if `userID` doesn't exist.

### 4.3 `internal/handler/compatibility.go` — `GetMe` adds `org` field

In `GetMe` (lines 52-95), after the existing `dashboard` and `records` fetches and before the `c.JSON` call, add:

```go
orgID, orgName, err := h.UserSvc.GetUserOrg(c.Request.Context(), userID)
if err != nil {
    Internal(c, err)
    return
}
```

And in the JSON payload, add the new top-level field:

```go
c.JSON(http.StatusOK, gin.H{
    "success": true,
    "user": gin.H{ /* unchanged */ },
    "dashboard":              dashboard,
    "recentAuthorship":       records,
    "totalAuthorshipRecords": total,
    "org": gin.H{           // NEW
        "id":   orgID,
        "name": orgName,
    },
})
```

If the user happens to have no row (e.g., deleted concurrently — extremely unlikely behind JWT), `orgID == "" && orgName == ""` and the frontend renders "暂无组织". Same fallback the existing card already has.

`CompatibilityHandler` needs a `UserSvc *service.UserService` field (it currently has `DashboardSvc` and `AuthorshipSvc`). Wire it in `cmd/server/main.go` where the handler is constructed.

### 4.4 Wiring in `cmd/server/main.go`

Find the `CompatibilityHandler` construction and add the `UserSvc` field. The `userSvc` instance already exists in scope (used for login/registration handlers).

## 5. Frontend Changes

### 5.1 `web/src/types/api.ts` — extend `MeApiResponse`

```ts
export interface MeApiResponse {
  success: boolean;
  user: User;
  dashboard: DashboardStats;
  recentAuthorship: unknown[];
  totalAuthorshipRecords: number;
  org?: { id: string; name: string };   // NEW
}
```

`org` is optional to keep the type tolerant of older server responses during deploy windows. The frontend treats missing/empty `org.name` as "暂无组织".

### 5.2 `web/src/routes/Me.tsx` — render real org

Current (lines 20, 54-77):

```ts
const org = user.orgs?.[0];
// ... renders `org.org_name` and `org.org_slug`
```

Replace with:

```ts
const org = me.org;   // me is the MeApiResponse
// ... renders `org.name` only; the "Slug: ..." line is removed.
```

The card structure (icon, title `组织架构`, the conditional `org ? card-with-name : card-with-暂无组织`) stays identical. Only:

- Source of truth changes from `user.orgs[0]` to top-level `me.org`.
- `<p className="me-page__org-slug">Slug: {org.org_slug}</p>` is **deleted**.
- `<p className="me-page__org-name">{org.org_name}</p>` becomes `<p className="me-page__org-name">{org.name}</p>`.

The `me-page__org-slug` CSS class likely becomes unused; cleaning it up is in scope (small) but skippable if it's also referenced elsewhere — verify with grep before deletion.

## 6. Testing

### 6.1 Backend

**`getTopOrgs`** — manual verification against the local seeded DB:

```sql
-- Should return one row: id …a1, name 研发, prompt_count = X, member_count = Y
SELECT
    o.id::text, coalesce(o.name, '') AS org_name,
    coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) AS prompt_count,
    count(distinct e.user_id) AS member_count
FROM public.metrics_events e
JOIN public.users u ON u.id::text = e.user_id
JOIN public.orgs o ON o.id = u.org_id
WHERE e.event_timestamp >= now() - interval '7 days'
GROUP BY o.id, o.name
ORDER BY prompt_count DESC, member_count DESC, o.id ASC
LIMIT 10;
```

End-to-end: GET `/api/admin/dashboard/stats?range=7d` as an admin user — `topOrgs` array should now contain one row for the `研发` org with realistic counts.

**`GetUserOrg`** — table-driven test (mirroring style of `device_flow_test.go`) is *optional* — the method is a 6-line query, verifiable end-to-end via the `/me` integration check below.

### 6.2 `/me` end-to-end

- Boot the server locally against the seeded `git_ai_dev`.
- Log in as `admin` (or any seeded user).
- `curl /api/me` and confirm response includes `"org": {"id": "00000000-...-a1", "name": "研发"}`.
- Open `/me` in the SPA — the `组织架构` card shows `研发` and **no Slug line**.

### 6.3 Regression

- `go build ./...` clean.
- `go test ./...` all pass — handler tests use mock `DashboardSvc`/`AuthorshipSvc`; if `GetMe` now requires a `UserSvc`, the test setup needs the new field too. Update fakes accordingly.
- Frontend `vite build` clean (catches any TS errors from the type change).

## 7. Risk & Rollback

- **Risk:** The `getTopOrgs` SQL change removes a fallback that silently masked schema drift. If the `orgs` table were dropped accidentally in some environment, `/api/admin/dashboard/stats` would now 500 instead of returning a partial response. Mitigation: this is the correct behavior — schema drift should be loud, not silent.
- **Rollback:** Revert the single commit. No DB change required (schema 005 stays).

## 8. Future Work (still deferred)

Carrying forward from `2026-05-08-user-org-schema-design.md` §7:

1. `model.User.OrgID` field; `FindByID` / `Create` read+write it.
2. `OrgService` + `GET /api/orgs` + admin endpoints (rename, create, set user.org_id).
3. Replace JWT `personal_org_id` / `orgs[]` legacy shim with real org info.
4. Org-list / user-detail admin UI.
5. Registration: optional org selection.
