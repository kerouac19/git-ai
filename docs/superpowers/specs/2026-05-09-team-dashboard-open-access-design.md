# Team Dashboard Open Access — Design Spec

- **Status**: Draft
- **Author**: kerouac19
- **Date**: 2026-05-09
- **Scope**: server-go (Go API) + server-go/web (React SPA)
- **Depends on**: `2026-05-08-admin-activity-dashboard-design.md` (already shipped — built the original admin-only dashboard) and `2026-05-08-dashboard-org-display-design.md` (already shipped — fixed `getTopOrgs` SQL).

## 1. Goal

Open the existing admin activity dashboard to **all authenticated users**. Today only `role == "admin"` can view `/admin/activity`; the request is to let every logged-in user see the same page with the same data.

## 2. Non-Goals

- **No data redaction.** Top users continue to include email; aggregate counters are unchanged. Decision per brainstorming: trust-based team, full transparency.
- **No new admin-only surface.** We are not splitting "admin extended view" vs "team view" — there is one view, shared.
- **No role/permission model changes.** `adminOnly()` middleware stays in place (still used by `/user/register`); we just stop applying it to the dashboard route.
- **No filename rename of `admin_dashboard.go` / `admin_dashboard_test.go` / `AdminDashboardService` / `AdminDashboardHandler` / `AdminDashboardData` / `AdminTopUser` etc.** Renaming the Go types and files is mass churn for zero behavior change. Comments at file top get the "admin-only" wording stripped; identifiers stay. (If the misnomer ever bites, follow-up PR.)
- **No backend route alias.** `/api/admin/dashboard/stats` is removed outright. The frontend is the only known consumer.
- **No backwards-compat for `/api/admin/dashboard/stats` callers outside this repo.** If external scripts/integrations exist, they break — accepted risk per the migration decision.

## 3. Architecture

```
                                       ┌────────────────────────┐
GET /api/dashboard/global              │ AdminDashboardService  │
    └── jwtMW                          │   GetGlobalStats(...)  │  (unchanged)
        └── AdminDashboardHandler ────▶│                        │
                                       └────────────────────────┘

GET /admin/activity   ──── 302 Navigate ───▶  GET /dashboard
                                              └── <Dashboard /> (renamed from <AdminActivity />)
```

**Boundaries:**

- The handler/service stays exactly as-is. The only backend change is the **route registration site** in `cmd/server/main.go`: move one line out of the `/api/admin` group into the existing `/api/dashboard` group, drop `adminOnly()` from its middleware chain.
- The frontend changes are: rename one route + one component + one CSS-irrelevant card title; ungate one entry on `/me`; update one API helper's URL; add one redirect line.

## 4. Backend Changes

### 4.1 `cmd/server/main.go` — move route, drop `adminOnly()`

Current state (lines 216-228):

```go
dashboard := api.Group("/dashboard", jsonLimit)
{
    dashboard.GET("/public", dashboardH.GetPublicStats)
    dashboard.GET("/stats", jwtMW, dashboardH.GetStats)
    dashboard.POST("/generate-report", jwtMW, csrfMW, dashboardH.GenerateReport)
}

// Admin-only platform-wide dashboard. Reuses the existing adminOnly()
// middleware below — non-admin callers get 403, unauthenticated 401.
admin := api.Group("/admin", jsonLimit, jwtMW, adminOnly())
{
    admin.GET("/dashboard/stats", adminDashH.GetGlobalStats)
}
```

After:

```go
dashboard := api.Group("/dashboard", jsonLimit)
{
    dashboard.GET("/public", dashboardH.GetPublicStats)
    dashboard.GET("/stats", jwtMW, dashboardH.GetStats)
    dashboard.POST("/generate-report", jwtMW, csrfMW, dashboardH.GenerateReport)
    dashboard.GET("/global", jwtMW, adminDashH.GetGlobalStats)   // moved + renamed; jwtMW only
}
```

The `/api/admin` group is removed entirely (it had only one route). The `adminOnly()` function itself stays — it's still used by `api.POST("/user/register", …, adminOnly(), …)` at line 179.

### 4.2 `internal/handler/admin_dashboard.go` — comment cleanup only

Strip "admin-only" / "admin"-flavored language from the file-level comment + `GetGlobalStats` docstring. Do NOT rename the type, the method, or the file. The handler is still semantically the "global / cross-user / cross-org" stats endpoint; the misleading name is acknowledged tech debt deferred to a future PR.

### 4.3 `internal/handler/admin_dashboard_test.go` — URL string update

The 4 existing test cases register the handler at `/api/admin/dashboard/stats` for their httptest router. Update the path string in:

- `newAdminDashTestRouter` (line 35): `r.GET("/api/admin/dashboard/stats", h.GetGlobalStats)` → `r.GET("/api/dashboard/global", h.GetGlobalStats)`
- All `httptest.NewRequest(...)` calls in the file: same path swap.

No new test cases needed. The test file does not exercise middleware (no `jwtMW` / `adminOnly()` are wired into the test router today), so dropping `adminOnly()` from the production route doesn't change what the tests cover. The "non-admin can call it" property is verified manually in §6.

## 5. Frontend Changes

### 5.1 `web/src/api/admin.ts` — URL change

```ts
// before
api.get<AdminDashboardResponse>(`/api/admin/dashboard/stats?range=${range}`)
// after
api.get<AdminDashboardResponse>(`/api/dashboard/global?range=${range}`)
```

The file is left at `web/src/api/admin.ts` for now; renaming it forces every importer to change. If the file rename is desired, do it as a follow-up commit (single mechanical rename + import updates).

### 5.2 `web/src/routes/AdminActivity.tsx` → `web/src/routes/Dashboard.tsx`

**Rename the file.** Inside, all of the following must change — the component currently has its own admin gates beyond the `/me` entry-card gate:

- **Component rename**: `function AdminActivityContent({ user })` → `function DashboardContent()` (drop the `user` prop — see "remove user prop" below). `export default function AdminActivity()` → `export default function Dashboard()`. The render-prop call site in the default export drops `user` from the destructure: `{() => <DashboardContent />}`.
- **Remove `user` prop entirely**: after deleting the role gates, `user` is no longer referenced inside the component. Drop it from the inner component's signature, the `useEffect` dependency array (`[range, user.role, tick]` → `[range, tick]`), and the outer default-export render-prop.
- **Delete line 26 admin gate** in the `useEffect`: `if (user.role !== "admin") return;` — this was preventing the fetch from firing for non-admins. Removing it lets the fetch fire for everyone.
- **Delete lines 43-45 redirect**: `if (user.role !== "admin") { return <Navigate to="/me" replace />; }` — was bouncing non-admins back to `/me`. Removed wholesale.
- **Delete lines 47-49 forbidden-redirect** + the `forbidden: true` branch in the catch (line 33-36): with the API now open to all logged-in users, a 403 response is no longer expected for this endpoint. The `forbidden?: boolean` field on the `FetchState` `error` variant becomes dead and should be removed from the type union too.
- **Drop now-unused `Navigate` import** from `react-router-dom` (only `Link` remains used).
- **Page heading text**: `<h1>平台活跃度看板</h1>` → `<h1>团队看板</h1>`. The subtitle `全平台聚合数据 · 返回个人页` already reads neutrally and stays.

### 5.3 `web/src/App.tsx` — route swap + redirect

```tsx
const Dashboard = lazy(() => import("./routes/Dashboard"));   // renamed import

// inside <Routes>:
<Route
  path="/dashboard"
  element={
    <Suspense fallback={<div style={{ padding: 24 }}>Loading…</div>}>
      <Dashboard />
    </Suspense>
  }
/>
<Route path="/admin/activity" element={<Navigate to="/dashboard" replace />} />
```

The `/admin/activity` redirect is the entire backwards-compat surface for stale bookmarks. Two lines.

### 5.4 `web/src/routes/Me.tsx` — ungate entry card

Lines 84-91 today:

```tsx
{user.role === "admin" && (
  <Link to="/admin/activity" className="card admin-entry-card" style={...}>
    <h2 style={{ margin: 0 }}>管理员看板</h2>
    <p className="muted" style={...}>
      查看平台全局活跃度统计 →
    </p>
  </Link>
)}
```

After:

```tsx
<Link to="/dashboard" className="card admin-entry-card" style={...}>
  <h2 style={{ margin: 0 }}>团队看板</h2>
  <p className="muted" style={...}>
    查看全平台活跃度统计 →
  </p>
</Link>
```

The `admin-entry-card` CSS class name itself is left unchanged (it's a stable selector; renaming = pointless churn). Only the rendering condition + visible text + link target change.

## 6. Testing

### 6.1 Backend

- `go build ./...` clean.
- `go test ./internal/handler/...` — the 4 existing dashboard tests pass after the URL string update.
- `go test ./...` clean.

### 6.2 Frontend

- `npm run build` clean (catches lazy-import path mistakes + TS errors from the file rename).
- Lint clean (`npm run lint` if configured).

### 6.3 End-to-end (manual, against local seeded DB)

1. Boot server. Log in as **non-admin** user.
2. Visit `/me` — confirm the "团队看板" entry card is visible (was hidden before).
3. Click it — lands on `/dashboard`, page renders summary KPIs + trend + Top Users + Top Orgs + agent/model distribution.
4. Hit `/api/dashboard/global?range=7d` directly via cURL (with the user's session cookie) — returns 200 and the same JSON shape as the old admin endpoint.
5. Visit `/admin/activity` — auto-redirects to `/dashboard` (browser URL bar updates).
6. Hit `/api/admin/dashboard/stats` directly — returns 404.
7. Repeat steps 1-3 as **admin** user — same dashboard renders identically (admin path is now just "non-admin who happens to have role=admin").

## 7. Risk & Rollback

- **Risk 1 — privacy:** Top users leaderboard exposes user emails to all teammates. Decision: accepted, trust-based team. If concerns surface post-deploy, follow-up PR can swap email for `username` in the response or strip the field server-side.
- **Risk 2 — external API consumers:** Any out-of-repo script hitting `/api/admin/dashboard/stats` breaks (404). Decision: accepted; no known external consumers. If discovered, the route can be re-aliased trivially.
- **Risk 3 — Stale bookmarks:** Mitigated by the `/admin/activity → /dashboard` SPA redirect.
- **Rollback:** Single commit revert. No DB migrations, no config changes, no auth shape changes. Frontend and backend ship together (mono-repo), so revert atomically rolls both back.

## 8. Future Work (deferred)

- Rename `AdminDashboardService`, `AdminDashboardHandler`, `AdminDashboardData`, `AdminTop*` types, the source files, and the test file to neutral names (e.g., `GlobalDashboardService`). Pure mechanical rename PR.
- Rename `web/src/api/admin.ts` → `web/src/api/dashboard.ts` and the `admin-entry-card` CSS class to a neutral name (e.g., `dashboard-entry-card`).
- Optionally re-introduce an admin-only "extended" view with PII or moderation tools. Would re-use `adminOnly()` middleware on a separate endpoint.
- Consider per-user / per-org "my team" filtered view (range + org scope) once orgs become a richer concept.
