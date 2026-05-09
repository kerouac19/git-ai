# Team Dashboard Open Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open the existing admin activity dashboard to all authenticated users — same data, neutral name, new public route — by moving the backend route out of the `/api/admin` group, renaming the SPA page, ungating the entry card, and adding a redirect for stale bookmarks.

**Architecture:** Single-handler reuse. Backend: move one route line in `cmd/server/main.go` (drop `adminOnly()` from its middleware chain; place under `/api/dashboard/global`); leave `AdminDashboardHandler`/`AdminDashboardService` types and files unchanged (mass rename is deferred churn). Frontend: rename `AdminActivity.tsx` → `Dashboard.tsx`, drop in-component admin gates, add `/admin/activity → /dashboard` SPA redirect, ungate the entry card on `/me`. No DB migrations, no auth/JWT changes, no data redaction.

**Tech Stack:** Go 1.x + Gin + pgx (server-go) · React 18 + TypeScript + Vite + react-router-dom (server-go/web). Tests: Go `testing` + httptest (no middleware in test router).

**Spec:** `docs/superpowers/specs/2026-05-09-team-dashboard-open-access-design.md`

---

## File Map

**Backend — modify only:**
- `server-go/cmd/server/main.go` — move route, delete `/api/admin` group block.
- `server-go/internal/handler/admin_dashboard.go` — strip "admin-only" wording from comments; **no signature changes**.
- `server-go/internal/handler/admin_dashboard_test.go` — update URL string used by `newAdminDashTestRouter` and the 4 `httptest.NewRequest` calls.

**Frontend — modify only:**
- `server-go/web/src/api/admin.ts` — change URL from `/api/admin/dashboard/stats` to `/api/dashboard/global`.

**Frontend — rename + modify:**
- `server-go/web/src/routes/AdminActivity.tsx` → `server-go/web/src/routes/Dashboard.tsx` — rename file; drop `user` prop; delete admin gates + `forbidden` branch + `Navigate` import; rename component(s); change page heading text.

**Frontend — modify only:**
- `server-go/web/src/App.tsx` — swap lazy import target + path; add `/admin/activity` redirect route.
- `server-go/web/src/routes/Me.tsx` — remove `user.role === "admin"` gate around the entry card; update card title text + link target.

**No new files. No deletions other than the file rename above.**

---

## Task 1: Backend — move route, drop adminOnly(), update tests

**Files:**
- Modify: `server-go/cmd/server/main.go:214-228` (route registration block) — move dashboard/global route, delete `/api/admin` group
- Modify: `server-go/internal/handler/admin_dashboard.go:13-23` (file-top comments) — strip "admin-only" language
- Modify: `server-go/internal/handler/admin_dashboard_test.go:35,45,61,77,93,119` (six URL strings)

- [ ] **Step 1: Update the existing 4 backend tests to point at the new URL (test-first — should fail because production hasn't moved yet)**

Open `server-go/internal/handler/admin_dashboard_test.go` and replace **every occurrence** of `/api/admin/dashboard/stats` with `/api/dashboard/global`. There are 6 occurrences:
- Line 35: `r.GET("/api/admin/dashboard/stats", h.GetGlobalStats)` → `r.GET("/api/dashboard/global", h.GetGlobalStats)`
- Line 45: `httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)` → `…, "/api/dashboard/global", nil)`
- Line 61: `httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats?range=30d", nil)` → `…, "/api/dashboard/global?range=30d", nil)`
- Line 77: `httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats?range=42d", nil)` → `…, "/api/dashboard/global?range=42d", nil)`
- Line 93: `httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)` → `…, "/api/dashboard/global", nil)`
- Line 119: `httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)` → `…, "/api/dashboard/global", nil)`

You can use `Edit` with `replace_all: true` on the string `/api/admin/dashboard/stats` in this file — there are no false positives (the string only appears in URL contexts).

- [ ] **Step 2: Run the tests to confirm they still pass (the test router is built ad-hoc per test, so renaming the registered path + the request paths together keeps them in sync)**

Run from `/Users/hg/git/git-ai/server-go`:

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/... -run AdminDashboard -v
```

Expected: All 5 `TestAdminDashboard_*` tests PASS. (Yes — they pass *before* production is changed because the test creates its own router locally; this test file does not import `cmd/server/main.go`'s route table. The tests verify handler behavior, not route registration. The URL string just needs to be consistent within the test file.)

- [ ] **Step 3: Modify the production route registration in `cmd/server/main.go`**

Read `server-go/cmd/server/main.go` lines 214-228. Current block:

```go
		// Dashboard. Public stats stay open; per-user stats require a
		// session and always map to the caller's own sub.
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

Replace with:

```go
		// Dashboard. Public stats stay open; per-user stats require a
		// session and always map to the caller's own sub. /global returns
		// the same cross-user/cross-org payload to any authenticated user.
		dashboard := api.Group("/dashboard", jsonLimit)
		{
			dashboard.GET("/public", dashboardH.GetPublicStats)
			dashboard.GET("/stats", jwtMW, dashboardH.GetStats)
			dashboard.POST("/generate-report", jwtMW, csrfMW, dashboardH.GenerateReport)
			dashboard.GET("/global", jwtMW, adminDashH.GetGlobalStats)
		}
```

The entire `admin := api.Group("/admin", …)` block (and its single inner route) is **deleted**. The `adminOnly()` function itself stays — it's still imported below at line 179 by `/user/register`.

- [ ] **Step 4: Strip "admin-only" wording from `admin_dashboard.go` comments**

Read `server-go/internal/handler/admin_dashboard.go` lines 13-23. Replace this block:

```go
// AdminDashboardSvc is the surface AdminDashboardHandler depends on. Defined
// here so tests can swap in a fake without touching the real DB.
type AdminDashboardSvc interface {
	GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error)
}

type AdminDashboardHandler struct {
	Svc AdminDashboardSvc
}

func (h *AdminDashboardHandler) GetGlobalStats(c *gin.Context) {
```

With (only the inline comment changes — types/method are kept verbatim, but a brief docstring is added on the handler):

```go
// AdminDashboardSvc is the surface the dashboard handler depends on. Defined
// here so tests can swap in a fake without touching the real DB. (Type name
// is preserved for now; renaming is deferred churn.)
type AdminDashboardSvc interface {
	GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error)
}

type AdminDashboardHandler struct {
	Svc AdminDashboardSvc
}

// GetGlobalStats returns the cross-user/cross-org dashboard payload to any
// authenticated user. Routed at GET /api/dashboard/global.
func (h *AdminDashboardHandler) GetGlobalStats(c *gin.Context) {
```

No identifier changes. No signature changes. Comments only.

- [ ] **Step 5: Build + run the full backend test suite**

Run from `/Users/hg/git/git-ai/server-go`:

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```

Expected: build clean (no unused-import errors — the `admin` group deletion does not free any imports because `adminOnly` is still used by `/user/register`). All tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/hg/git/git-ai && git add server-go/cmd/server/main.go server-go/internal/handler/admin_dashboard.go server-go/internal/handler/admin_dashboard_test.go && git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
server-go: move dashboard global stats from /api/admin to /api/dashboard/global

Drops adminOnly() from the cross-user/cross-org dashboard route so any
authenticated user can read the same payload. Same handler, same data
shape. The /api/admin group is removed (it had only one route);
adminOnly() stays — still used by /user/register.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify with `git status` — working tree clean for these three files.

---

## Task 2: Frontend — repoint API helper at the new URL

**Files:**
- Modify: `server-go/web/src/api/admin.ts:6` (URL string)

- [ ] **Step 1: Update the URL in `api/admin.ts`**

Read `server-go/web/src/api/admin.ts`. Replace:

```ts
import { api } from "./client";
import type { AdminDashboardResponse, AdminRangeKey } from "../types/api";

export const adminApi = {
  fetchDashboard: (range: AdminRangeKey) =>
    api.get<AdminDashboardResponse>(`/api/admin/dashboard/stats?range=${range}`),
};
```

With:

```ts
import { api } from "./client";
import type { AdminDashboardResponse, AdminRangeKey } from "../types/api";

export const adminApi = {
  fetchDashboard: (range: AdminRangeKey) =>
    api.get<AdminDashboardResponse>(`/api/dashboard/global?range=${range}`),
};
```

The `adminApi` export name is preserved (renaming would force every importer to update; deferred to follow-up). Only the URL string changes.

- [ ] **Step 2: Build the frontend to confirm no type errors**

Run from `/Users/hg/git/git-ai/server-go/web`:

```bash
cd /Users/hg/git/git-ai/server-go/web && npm run build
```

Expected: build clean (one URL change cannot introduce TS errors, but build also catches import drift).

- [ ] **Step 3: Commit**

```bash
cd /Users/hg/git/git-ai && git add server-go/web/src/api/admin.ts && git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
server-go/web: point dashboard API helper at /api/dashboard/global

Mirrors the backend route move. Export name (adminApi) kept to avoid
churning every importer; rename deferred.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Frontend — rename AdminActivity → Dashboard, ungate, redirect, retitle entry card

This task touches three files together because the rename + route swap + entry-card update are tightly coupled — the import, the route path, and the link `to` must move atomically or the SPA breaks mid-task.

**Files:**
- Rename + modify: `server-go/web/src/routes/AdminActivity.tsx` → `server-go/web/src/routes/Dashboard.tsx`
- Modify: `server-go/web/src/App.tsx:8,17-24` (lazy import + route block)
- Modify: `server-go/web/src/routes/Me.tsx:84-91` (entry card)

- [ ] **Step 1: Read the full current `AdminActivity.tsx`**

Already shown in the spec, but read it again with `Read` to make sure no surprises crept in.

- [ ] **Step 2: Rename the file using `git mv`**

```bash
cd /Users/hg/git/git-ai && git mv server-go/web/src/routes/AdminActivity.tsx server-go/web/src/routes/Dashboard.tsx
```

After this, `git status` shows `renamed: server-go/web/src/routes/AdminActivity.tsx -> server-go/web/src/routes/Dashboard.tsx`.

- [ ] **Step 3: Rewrite `Dashboard.tsx` with all the inline changes**

Use `Write` to overwrite `server-go/web/src/routes/Dashboard.tsx` with the full new contents below. Every change vs the original is intentional — see the inline comments.

```tsx
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { adminApi } from "../api/admin";
import type { AdminDashboardData, AdminRangeKey } from "../types/api";

import RangeToggle from "../components/admin/RangeToggle";
import SummaryCards from "../components/admin/SummaryCards";
import TrendChart from "../components/admin/TrendChart";
import AdoptionStackedBar from "../components/admin/AdoptionStackedBar";
import Leaderboard from "../components/admin/Leaderboard";
import DistributionDonut from "../components/admin/DistributionDonut";

type FetchState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; data: AdminDashboardData };

function DashboardContent() {
  const [range, setRange] = useState<AdminRangeKey>("7d");
  const [tick, setTick] = useState(0);
  const [state, setState] = useState<FetchState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    adminApi.fetchDashboard(range)
      .then(res => { if (!cancelled) setState({ status: "ready", data: res.data }); })
      .catch(err => {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : "未知错误";
        setState({ status: "error", message });
      });
    return () => { cancelled = true; };
  }, [range, tick]);

  const rangeLabel = range === "7d" ? "7 天" : "30 天";

  return (
    <main className="page-main admin-page">
      <div className="panel">
        <div className="admin-page__header">
          <div>
            <h1 style={{ margin: 0 }}>团队看板</h1>
            <p className="muted" style={{ margin: "4px 0 0 0" }}>
              全平台聚合数据 · <Link to="/me">返回个人页</Link>
            </p>
          </div>
          <RangeToggle
            value={range}
            onChange={setRange}
            disabled={state.status === "loading"}
          />
        </div>

        {state.status === "loading" && (
          <p className="muted" style={{ marginTop: 24 }}>加载中…</p>
        )}

        {state.status === "error" && (
          <div className="card" style={{ marginTop: 24 }}>
            <p style={{ color: "var(--danger)" }}>加载失败: {state.message}</p>
            <button type="button" onClick={() => setTick(t => t + 1)}>重试</button>
          </div>
        )}

        {state.status === "ready" && (
          <>
            <SummaryCards summary={state.data.summary} rangeLabel={rangeLabel} />

            <div className="admin-page__chart-stack">
              <TrendChart data={state.data.trend} />
              <AdoptionStackedBar data={state.data.trend} />
            </div>

            <div className="grid">
              <Leaderboard
                title="Top 用户"
                rows={state.data.topUsers}
                columns={[
                  { header: "用户", render: (r) => r.name || r.email || r.userId },
                  { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                  { header: "AI 行数", render: (r) => r.committedAiLines.toLocaleString(), align: "right" },
                ]}
              />
              <Leaderboard
                title="Top 组织"
                rows={state.data.topOrgs}
                columns={[
                  { header: "组织", render: (r) => r.orgName || r.orgId },
                  { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                  { header: "成员", render: (r) => r.memberCount.toLocaleString(), align: "right" },
                ]}
              />
            </div>

            <div className="grid">
              <DistributionDonut title="Agent 分布" rows={state.data.agentDistribution} />
              <DistributionDonut title="模型分布" rows={state.data.modelDistribution} />
            </div>
          </>
        )}
      </div>
    </main>
  );
}

export default function Dashboard() {
  return (
    <ProtectedRoute>
      {() => <DashboardContent />}
    </ProtectedRoute>
  );
}
```

Diff vs original (for review):
- Removed `Navigate` from `react-router-dom` import (no longer used).
- Removed `User` from type imports (component no longer takes `user` prop).
- Removed `ApiError` import (only used by the deleted 403 branch).
- `FetchState.error` no longer has `forbidden?: boolean` — dead.
- `AdminActivityContent({ user })` → `DashboardContent()` — no props.
- Deleted the `if (user.role !== "admin") return;` early-out at the top of the `useEffect`.
- Deleted the 403-catch branch (no longer reachable; endpoint is open to all authenticated users).
- `useEffect` deps `[range, user.role, tick]` → `[range, tick]`.
- Deleted both `<Navigate to="/me" replace />` early returns (admin-gate + forbidden-redirect).
- Deleted the `&& !state.forbidden` guard on the error-card render (the field no longer exists).
- `<h1>平台活跃度看板</h1>` → `<h1>团队看板</h1>`.
- `function AdminActivity()` → `function Dashboard()`; the inner `{({ user }) => <AdminActivityContent user={user} />}` → `{() => <DashboardContent />}`.
- `admin-page` / `admin-page__*` CSS classes are **kept** (renaming = pointless churn; deferred per spec §8).

- [ ] **Step 4: Update `App.tsx` — swap lazy import + add redirect**

Read `server-go/web/src/App.tsx`. Replace its full contents with:

```tsx
import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import Login from "./routes/Login";
import Me from "./routes/Me";
import DeviceFlow from "./routes/DeviceFlow";
import DeviceResult from "./routes/DeviceResult";

const Dashboard = lazy(() => import("./routes/Dashboard"));

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/me" element={<Me />} />
      <Route path="/oauth/device" element={<DeviceFlow />} />
      <Route path="/oauth/device/result" element={<DeviceResult />} />
      <Route
        path="/dashboard"
        element={
          <Suspense fallback={<div style={{ padding: 24 }}>Loading…</div>}>
            <Dashboard />
          </Suspense>
        }
      />
      <Route path="/admin/activity" element={<Navigate to="/dashboard" replace />} />
      <Route path="*" element={<Navigate to="/me" replace />} />
    </Routes>
  );
}
```

Diff vs original:
- `const AdminActivity = lazy(() => import("./routes/AdminActivity"));` → `const Dashboard = lazy(() => import("./routes/Dashboard"));`
- Route `path="/admin/activity"` (lazy + Suspense + `<AdminActivity />`) → `path="/dashboard"` with `<Dashboard />`.
- New line: `<Route path="/admin/activity" element={<Navigate to="/dashboard" replace />} />` — keeps stale bookmarks working.

- [ ] **Step 5: Update `Me.tsx` entry card — ungate + retitle + relink**

Read `server-go/web/src/routes/Me.tsx` lines 84-91. Replace:

```tsx
      {user.role === "admin" && (
        <Link to="/admin/activity" className="card admin-entry-card" style={{ marginTop: 16, display: "block" }}>
          <h2 style={{ margin: 0 }}>管理员看板</h2>
          <p className="muted" style={{ margin: "4px 0 0 0" }}>
            查看平台全局活跃度统计 →
          </p>
        </Link>
      )}
```

With:

```tsx
      <Link to="/dashboard" className="card admin-entry-card" style={{ marginTop: 16, display: "block" }}>
        <h2 style={{ margin: 0 }}>团队看板</h2>
        <p className="muted" style={{ margin: "4px 0 0 0" }}>
          查看全平台活跃度统计 →
        </p>
      </Link>
```

Changes:
- `{user.role === "admin" && (…)}` wrapper deleted — entry shows for every authenticated user.
- `to="/admin/activity"` → `to="/dashboard"`.
- Title `管理员看板` → `团队看板`.
- Subtitle wording trimmed (`查看平台全局活跃度统计` → `查看全平台活跃度统计`).
- `admin-entry-card` class kept (deferred rename per spec §8).

- [ ] **Step 6: Build the frontend — must be clean**

Run from `/Users/hg/git/git-ai/server-go/web`:

```bash
cd /Users/hg/git/git-ai/server-go/web && npm run build
```

Expected: build clean. The most likely failure modes if a step was botched:
- "Cannot find module './routes/Dashboard'" → `git mv` didn't run or wrong filename. Re-check.
- "'Navigate' is declared but never read" / "'User' is declared but never read" / "'ApiError' is declared but never read" → forgot to drop an import in `Dashboard.tsx`. Trim.
- "Property 'forbidden' does not exist on type" → forgot to remove a reference to the dropped `forbidden` field. Search for `forbidden` in the file and delete the residue.
- "user is declared but never read" in `Me.tsx` — unrelated, `user` is still used elsewhere on that page.

- [ ] **Step 7: Commit**

```bash
cd /Users/hg/git/git-ai && git add server-go/web/src/routes/Dashboard.tsx server-go/web/src/routes/AdminActivity.tsx server-go/web/src/App.tsx server-go/web/src/routes/Me.tsx && git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
server-go/web: rename AdminActivity to Dashboard, ungate for all users

Renames /admin/activity to /dashboard with a one-line redirect for
stale bookmarks. Drops in-component admin gates and the now-unreachable
403-forbidden branch. Entry card on /me shows for every authenticated
user; title becomes 团队看板.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

`git status` should be clean for these four paths after the commit.

---

## Task 4: Final regression — backend + frontend + manual E2E

No code changes in this task. Verify the whole flow end-to-end and only commit if a fix is needed.

**Files:** none (verification only)

- [ ] **Step 1: Backend — full build + tests**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```

Expected: all green.

- [ ] **Step 2: Frontend — full build**

```bash
cd /Users/hg/git/git-ai/server-go/web && npm run build
```

Expected: build clean (Vite output shows generated assets).

- [ ] **Step 3: Manual E2E against the local seeded DB**

Boot the server (the user runs the dev server locally; if you need to start it as the agent, use `task dev` or the user's documented dev script). Then:

1. Log in as a **non-admin** seeded user.
2. On `/me`: confirm the **团队看板** entry card is visible (was hidden before for this user).
3. Click it → URL becomes `/dashboard`. Confirm:
   - Page heading reads `团队看板`.
   - SummaryCards renders.
   - TrendChart + AdoptionStackedBar render.
   - Top 用户 + Top 组织 leaderboards render.
   - Agent + 模型 distribution donuts render.
4. With the same session cookie, hit the API directly:

   ```bash
   curl -s -b "<your-cookie-file>" 'http://127.0.0.1:3000/api/dashboard/global?range=7d' | jq '.success, .data.summary'
   ```

   Expected: `true` + a summary object with non-zero counters (assuming seeded data exists).
5. Visit `/admin/activity` in the browser → URL bar updates to `/dashboard` (the SPA redirect fires).
6. Hit the old API to confirm it's gone:

   ```bash
   curl -s -o /dev/null -w '%{http_code}\n' -b "<your-cookie-file>" 'http://127.0.0.1:3000/api/admin/dashboard/stats'
   ```

   Expected: `404`.
7. Log in as **admin** and repeat steps 2-3 — page should render identically (admin path is now just "another authenticated user").

- [ ] **Step 4: Verify no follow-up commit needed**

```bash
cd /Users/hg/git/git-ai && git status
```

Expected: working tree clean (or only contains the pre-existing `vite.config.ts` local edit + `.claude/` untracked, which are unrelated to this work).

If a regression was found, fix it inline and commit; then return to Step 1 of this task.

- [ ] **Step 5: Commit the plan itself (if not already committed)**

```bash
cd /Users/hg/git/git-ai && git add docs/superpowers/plans/2026-05-09-team-dashboard-open-access.md && git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
docs: implementation plan for opening team dashboard to all users

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Skip this step if the plan file was committed earlier.)

---

## Self-Review (already run by author)

**Spec coverage:**
- §4.1 (route move + drop adminOnly + remove `/admin` group) → Task 1 Step 3 ✓
- §4.2 (comment cleanup, no rename) → Task 1 Step 4 ✓
- §4.3 (test URL update, no new cases) → Task 1 Steps 1-2 ✓
- §5.1 (api/admin.ts URL) → Task 2 ✓
- §5.2 (file rename + ungate + dead-code removal + heading) → Task 3 Steps 2-3 ✓
- §5.3 (App.tsx route swap + redirect) → Task 3 Step 4 ✓
- §5.4 (Me.tsx ungate entry card) → Task 3 Step 5 ✓
- §6.1 (backend build + tests) → Task 4 Steps 1 ✓
- §6.2 (frontend build) → Task 4 Step 2 ✓
- §6.3 (E2E checklist, 7 steps) → Task 4 Step 3 ✓

**Placeholder scan:** none — every code block is complete.

**Type / identifier consistency:**
- `DashboardContent` / `Dashboard` used consistently in Task 3 (Steps 3, 4, 7).
- `/api/dashboard/global` consistent across Task 1 (production + tests + comments) and Task 2 (frontend).
- `/dashboard` route consistent across Task 3 Steps 4 and 5.
- `adminApi` export name explicitly preserved — Task 2 Step 1 calls this out.
- `admin-page` / `admin-entry-card` CSS classes explicitly preserved — Task 3 Steps 3 and 5 call this out.
