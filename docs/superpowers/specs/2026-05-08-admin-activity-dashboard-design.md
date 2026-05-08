# Admin Activity Dashboard — Design Spec

- **Status**: Draft
- **Author**: kerouac19
- **Date**: 2026-05-08
- **Scope**: server-go (Go API) + server-go/web (React SPA)

## 1. Goal

Add a platform-wide activity dashboard for **system administrators** (`role == "admin"`), available at `/admin/activity` in the SPA. The dashboard answers four questions:

1. Is the platform's usage growing or declining over time?
2. Which users and organizations are most active?
3. Which AI agents and models are being used the most?
4. What is the overall AI code adoption rate (committed AI lines / total added lines)?

## 2. Non-Goals

- No org / user / agent **filter dropdowns** — only a fixed `7d` / `30d` toggle.
- No caching layer in the first cut. Add only if measured query time warrants it.
- No drill-down — clicking a top user/org does not navigate to a detail page.
- No CSV / report export. Existing `POST /api/dashboard/generate-report` is unchanged.
- No global navigation bar. The entry to the new dashboard is a single link card at the top of `/me`, visible only to admins.
- No rework of the existing per-user `/api/dashboard/stats` endpoint. It stays as-is.

## 3. Architecture Overview

```
Browser (SPA)
  └── /admin/activity                     (new route, gated by user.role)
        └── GET /api/admin/dashboard/stats?range=7d|30d
              └── jwtMW → requireAdmin → AdminDashboardHandler
                    └── AdminDashboardService (errgroup, concurrent queries)
                          └── Postgres: metrics_events (+ users, orgs joins)
```

**Key boundary decisions**:

- A **new** `AdminDashboardService` is introduced rather than extending `DashboardService`. The existing service's contract is "stats for a given user_id"; mixing in an admin mode (e.g., `userID == ""` means global) blurs authorization. The two services share no state and may have similar SQL templates, which is acceptable.
- A **new** route group `/api/admin/dashboard/*` makes the privilege requirement obvious from the URL. A single `requireAdmin` middleware guards the whole group.
- The SPA gates the route on the client (`user.role === "admin"`), but real authorization is the server's 403 response. Client gating is purely UX.

## 4. Backend

### 4.1 New Files

- `internal/service/admin_dashboard.go` — `AdminDashboardService` with `GetGlobalStats(ctx, range)`.
- `internal/handler/admin_dashboard.go` — HTTP layer; parses `range`, returns 400 on invalid values, calls service.
- `internal/middleware/require_admin.go` — reads `userSubjectAndRole(c)`; 403 if role != `"admin"`.
- `internal/handler/admin_dashboard_test.go` — table-driven integration tests (mirroring `device_flow_test.go` style).

### 4.2 Wiring

In `cmd/server/...` route setup:

```go
adminDashSvc := &service.AdminDashboardService{Pool: pool, MetricsSvc: metricsSvc}
adminDashH   := &handler.AdminDashboardHandler{Svc: adminDashSvc}

admin := api.Group("/admin", jsonLimit, jwtMW, middleware.RequireAdmin())
admin.GET("/dashboard/stats", adminDashH.GetGlobalStats)
```

### 4.3 API Contract

`GET /api/admin/dashboard/stats?range=7d|30d`

- Auth: JWT cookie + `role == "admin"`.
- Query: `range` ∈ {`7d`, `30d`}. Default `7d`. Anything else → 400.
- Response (200):

```jsonc
{
  "success": true,
  "data": {
    "range": "7d",
    "summary": {
      "activeUsersToday": 42,
      "activeUsersInRange": 180,
      "totalPrompts": 5234,            // distinct prompt_id where event_id=2
      "totalCheckpoints": 890,         // row count where event_id=4
      "aiCodePercentage": 38.4
    },
    "trend": [
      {
        "date": "2026-05-01",
        "activeUsers": 25,
        "promptCount": 320,
        "checkpointCount": 80,
        "committedAiLines": 1500,
        "totalAddedLines": 4000,
        "generatedAiLines": 1900,
        "editedAiLines": 200
      }
    ],
    "topUsers": [
      { "userId": "...", "name": "...", "email": "...", "promptCount": 120, "committedAiLines": 800 }
    ],
    "topOrgs": [
      { "orgId": "...", "orgName": "...", "promptCount": 450, "memberCount": 12 }
    ],
    "agentDistribution": [
      { "label": "claude_code", "promptCount": 1200, "share": 0.42 }
    ],
    "modelDistribution": [
      { "label": "claude-opus-4-7", "promptCount": 800, "share": 0.30 }
    ]
  },
  "timestamp": "2026-05-08T..."
}
```

- 401 if not authenticated, 403 if non-admin, 400 on bad `range`, 500 on DB error.
- Empty platform → all numeric fields are `0`, all arrays are `[]` (never `null`). Required so the frontend can render without conditional fallbacks per field.

### 4.4 SQL Strategy

All queries scope to `event_timestamp >= now() - interval '<range>'` with `<range>` resolved server-side from the parsed `range` parameter (never interpolated from raw input). Queries run concurrently via `errgroup.WithContext`.

- **summary.aiCodePercentage**: same expression as `DashboardService.getOverview` minus the `where user_id = $1` clause.
- **summary.activeUsersToday / activeUsersInRange**: `count(distinct user_id) filter (where event_timestamp >= ...)`.
- **trend**: `group by date_trunc('day', event_timestamp)`. Server backfills missing days with zero rows so the array length is always 7 or 30 — frontend chart code stays simple.
- **topUsers**: `group by user_id` then `JOIN public.users` on id, `order by prompt_count desc limit 10`.
- **topOrgs**: `JOIN public.org_memberships` to map `user_id → org_id`, then aggregate per org and `JOIN public.orgs` for name. Member count is `count(distinct user_id)` within the range.
- **agentDistribution / modelDistribution**: `group by attrs_json->>'20'` (agent) / `attrs_json->>'21'` (model) where `event_id = 2`. `share` computed in Go after the query so the SQL is a single GROUP BY.

### 4.5 Performance Notes (For Reviewer)

These queries scan the entire `metrics_events` table for the time window. With current data volume this is acceptable. **Triggers for adding caching later**:

- p95 latency of the endpoint > 1s, OR
- `metrics_events` row count exceeds ~10M

When triggered, options in order of preference:
1. Add a 60s in-memory LRU keyed on `(range)`. Admin pages don't need real-time freshness.
2. Materialized view refreshed every 5 minutes via cron.

This is explicitly out of scope for the first cut.

### 4.6 Testing

Integration tests follow `device_flow_test.go` / `releases_test.go` patterns. Test cases:

- Non-admin user (`role = "user"`) → 403.
- Unauthenticated → 401.
- `range=invalid` → 400, no DB call (verified by query logger).
- Admin, empty `metrics_events` → 200, zero values, empty arrays.
- Admin with seed data covering 3 users / 2 orgs / 2 agents / 2 models across 9 days → assert:
  - `summary.totalPrompts` matches expected count.
  - `trend` array length equals the requested range.
  - `topUsers` is sorted by `promptCount` desc, ties broken by stable secondary key.
  - `agentDistribution` shares sum to ~1.0.
- Concurrent admin requests do not deadlock or interleave incorrectly (errgroup smoke test).

## 5. Frontend

### 5.1 New Files

- `src/routes/AdminActivity.tsx` — page shell, fetches and renders.
- `src/api/admin.ts` — `fetchAdminDashboard(range)` typed wrapper.
- `src/types/api.ts` — append `AdminDashboardData`, `TrendPoint`, etc.
- `src/components/admin/SummaryCards.tsx` — 5 KPI tiles.
- `src/components/admin/TrendChart.tsx` — Recharts dual-axis line: DAU + prompt count.
- `src/components/admin/AdoptionStackedBar.tsx` — Recharts stacked bar of generated/committed/edited per day.
- `src/components/admin/Leaderboard.tsx` — generic table component reused for top users and top orgs (props: rows, columns).
- `src/components/admin/DistributionDonut.tsx` — Recharts donut, reused for agent and model.
- `src/components/admin/RangeToggle.tsx` — `7d` / `30d` segmented control.
- `src/components/AdminLink.tsx` — admin-only entry card mounted on `/me`.

### 5.2 Routing

`App.tsx`:

```tsx
<Route path="/admin/activity" element={<AdminActivity />} />
```

`AdminActivity` wraps content in `ProtectedRoute` and additionally checks `user.role === "admin"`. Non-admin → `<Navigate to="/me" replace />`. The server is the source of truth for authorization; this is UX only.

### 5.3 Layout

```
┌──────────────────────────────────────────────────────┐
│ 平台活跃度看板        [7天] [30天]   最后更新: ...   │
├──────────────────────────────────────────────────────┤
│ [今日活跃] [周活跃] [总Prompt] [总Ckpt] [AI 占比]    │
├──────────────────────────────────────────────────────┤
│  使用趋势 (双轴线图: DAU 左轴 / Prompt 数 右轴)      │
├──────────────────────────────────────────────────────┤
│  AI 代码采纳趋势 (堆叠柱: generated/committed/edited)│
├──────────────────────────┬───────────────────────────┤
│ Top 用户排行             │ Top 组织排行              │
├──────────────────────────┼───────────────────────────┤
│ Agent 分布 (donut)       │ 模型分布 (donut)          │
└──────────────────────────┴───────────────────────────┘
```

CSS lives in `src/styles/globals.css` under `.admin-page__*` classes, following the existing `.me-page__*` convention.

### 5.4 State and Loading

```tsx
type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; data: AdminDashboardData };
```

Single `useEffect` keyed on `range`. Re-fetches on toggle; toggle disabled while loading.

- Loading: skeleton blocks reusing `.panel` styling, no spinner.
- Error: panel with message and a "重试" button that re-runs the fetch.
- 403 from server: redirect to `/me` and surface a one-time top banner: "您没有权限访问此页面。" Banner state lives in `Me.tsx` via location state.
- Empty platform (all zeros): UI renders normally, charts show "暂无数据" placeholder text inside their container.

### 5.5 Recharts Dependency

Add `recharts` (latest 2.x compatible with React 19) to `web/package.json`. Verify bundle size after build; expect ~100 KB gzipped added to the chunk that contains `AdminActivity`. Use Vite's automatic code splitting — `AdminActivity` is lazy-imported via `React.lazy` so non-admin users never download Recharts.

### 5.6 Admin Entry on /me

`Me.tsx` renders `<AdminLink />` near the top of the profile panel **only when** `user.role === "admin"`. The card links to `/admin/activity` and uses an icon similar to the existing org-card icon for visual consistency.

### 5.7 Manual Verification (no automated frontend tests)

After dev build:

1. Log in as admin → `/me` shows "管理员看板" entry card → click → `/admin/activity` renders with data.
2. Toggle `7d` → `30d` → trend chart updates, x-axis ticks change.
3. Log in as non-admin → `/me` does not show the entry card. Manually navigate to `/admin/activity` → redirected to `/me` with banner.
4. Hit `/api/admin/dashboard/stats` from non-admin's browser console → 403.
5. Empty database scenario (drop seed data) → page renders with zeros, no JS errors.

## 6. Delivery Plan

Two PRs to keep diffs reviewable:

1. **PR 1 — Backend admin dashboard.** Adds service / handler / middleware / route / integration tests. Verifiable end-to-end via `curl` against the local server. No frontend changes.
2. **PR 2 — Frontend admin page.** Adds Recharts, new route, components, and the entry card on `/me`.

Both PRs are pure additions; revert is safe. No DB migrations.

## 7. Open Questions

None at spec time. If the topOrgs join turns out to require a new index on `org_memberships(user_id)`, that index is added inside PR 1 with a short note in the commit message.
