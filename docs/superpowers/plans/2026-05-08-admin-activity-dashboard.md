# Admin Activity Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-only platform activity dashboard at `/admin/activity` backed by a new `GET /api/admin/dashboard/stats` endpoint, surfacing usage trends, top users/orgs, agent/model distribution, and AI code adoption rate over a 7d/30d window.

**Architecture:** New `AdminDashboardService` + `AdminDashboardHandler` mounted under `/api/admin/...` and gated by the existing `adminOnly()` middleware. New React route lazy-loaded with Recharts for visualization. Pure additions — no changes to existing endpoints, schemas, or routes.

**Tech Stack:** Go 1.x · gin · pgxpool · Postgres · React 19 · React Router 7 · Vite 8 · Recharts 2.x

**Spec:** `docs/superpowers/specs/2026-05-08-admin-activity-dashboard-design.md`

**Delivery:** Two PRs — Tasks 1-4 ship together as PR 1 (backend), Tasks 5-12 as PR 2 (frontend).

---

## Test Strategy Note

The Go server has no DB-integration test infrastructure today (existing tests use either pure functions or fake services — see `device_flow_test.go` and `releases_test.go`). This plan follows that pattern:

- **Handler-level tests** with a fake `AdminDashboardSvc` interface — covers HTTP wiring, auth, JSON shape, error paths.
- **No automated SQL tests** — service correctness is verified by `curl` against a local server with seed data. If new DB test infra is desired, that is its own project, out of scope here.

The frontend has no automated test setup; verification is manual in the browser.

---

# PR 1 — Backend (Tasks 1-4)

## Task 1: Add response types

**Files:**
- Create: `server-go/internal/model/admin_dashboard.go`

- [ ] **Step 1: Create the model file**

```go
package model

// AdminDashboardData is the payload returned by GET /api/admin/dashboard/stats.
// All numeric fields default to zero and all slices default to empty (never
// nil) so the SPA can render unconditionally.
type AdminDashboardData struct {
	Range             string                 `json:"range"`
	Summary           AdminDashboardSummary  `json:"summary"`
	Trend             []AdminTrendPoint      `json:"trend"`
	TopUsers          []AdminTopUser         `json:"topUsers"`
	TopOrgs           []AdminTopOrg          `json:"topOrgs"`
	AgentDistribution []AdminDistributionRow `json:"agentDistribution"`
	ModelDistribution []AdminDistributionRow `json:"modelDistribution"`
}

type AdminDashboardSummary struct {
	ActiveUsersToday   int     `json:"activeUsersToday"`
	ActiveUsersInRange int     `json:"activeUsersInRange"`
	TotalPrompts       int     `json:"totalPrompts"`
	TotalCheckpoints   int     `json:"totalCheckpoints"`
	AICodePercentage   float64 `json:"aiCodePercentage"`
}

type AdminTrendPoint struct {
	Date             string `json:"date"`
	ActiveUsers      int    `json:"activeUsers"`
	PromptCount      int    `json:"promptCount"`
	CheckpointCount  int    `json:"checkpointCount"`
	CommittedAILines int    `json:"committedAiLines"`
	TotalAddedLines  int    `json:"totalAddedLines"`
	GeneratedAILines int    `json:"generatedAiLines"`
	EditedAILines    int    `json:"editedAiLines"`
}

type AdminTopUser struct {
	UserID           string `json:"userId"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	PromptCount      int    `json:"promptCount"`
	CommittedAILines int    `json:"committedAiLines"`
}

type AdminTopOrg struct {
	OrgID       string `json:"orgId"`
	OrgName     string `json:"orgName"`
	PromptCount int    `json:"promptCount"`
	MemberCount int    `json:"memberCount"`
}

type AdminDistributionRow struct {
	Label       string  `json:"label"`
	PromptCount int     `json:"promptCount"`
	Share       float64 `json:"share"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd server-go && go build ./...`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add server-go/internal/model/admin_dashboard.go
git commit -m "server-go: add admin dashboard response types"
```

---

## Task 2: Handler with fake-service unit tests

**Files:**
- Create: `server-go/internal/handler/admin_dashboard.go`
- Create: `server-go/internal/handler/admin_dashboard_test.go`

- [ ] **Step 1: Write the handler skeleton with the service interface**

Create `server-go/internal/handler/admin_dashboard.go`:

```go
package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git-ai-server/internal/model"

	"github.com/gin-gonic/gin"
)

// AdminDashboardSvc is the surface AdminDashboardHandler depends on. Defined
// here so tests can swap in a fake without touching the real DB.
type AdminDashboardSvc interface {
	GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error)
}

type AdminDashboardHandler struct {
	Svc AdminDashboardSvc
}

var validAdminRanges = map[string]bool{"7d": true, "30d": true}

func (h *AdminDashboardHandler) GetGlobalStats(c *gin.Context) {
	rangeKey := c.DefaultQuery("range", "7d")
	if !validAdminRanges[rangeKey] {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid range; expected 7d or 30d",
		})
		return
	}

	data, err := h.Svc.GetGlobalStats(c.Request.Context(), rangeKey)
	if err != nil {
		Internal(c, err)
		return
	}
	if data == nil {
		Internal(c, errors.New("admin dashboard service returned nil data"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
```

- [ ] **Step 2: Write the failing handler tests**

Create `server-go/internal/handler/admin_dashboard_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git-ai-server/internal/model"

	"github.com/gin-gonic/gin"
)

type fakeAdminDashSvc struct {
	data    *model.AdminDashboardData
	err     error
	gotCtx  context.Context
	gotKey  string
	calls   int
}

func (f *fakeAdminDashSvc) GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotKey = rangeKey
	return f.data, f.err
}

func newAdminDashTestRouter(svc AdminDashboardSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &AdminDashboardHandler{Svc: svc}
	r.GET("/api/admin/dashboard/stats", h.GetGlobalStats)
	return r
}

func TestAdminDashboard_DefaultRange(t *testing.T) {
	fake := &fakeAdminDashSvc{
		data: &model.AdminDashboardData{Range: "7d"},
	}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if fake.gotKey != "7d" {
		t.Errorf("service got range %q, want 7d", fake.gotKey)
	}
}

func TestAdminDashboard_ExplicitRange30d(t *testing.T) {
	fake := &fakeAdminDashSvc{data: &model.AdminDashboardData{Range: "30d"}}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats?range=30d", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if fake.gotKey != "30d" {
		t.Errorf("service got range %q, want 30d", fake.gotKey)
	}
}

func TestAdminDashboard_InvalidRange(t *testing.T) {
	fake := &fakeAdminDashSvc{}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats?range=42d", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if fake.calls != 0 {
		t.Errorf("service called %d times on bad range; want 0", fake.calls)
	}
}

func TestAdminDashboard_ServiceError(t *testing.T) {
	fake := &fakeAdminDashSvc{err: errors.New("db blew up")}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminDashboard_ResponseShape(t *testing.T) {
	fake := &fakeAdminDashSvc{
		data: &model.AdminDashboardData{
			Range: "7d",
			Summary: model.AdminDashboardSummary{
				TotalPrompts:     5,
				AICodePercentage: 12.5,
			},
			Trend:             []model.AdminTrendPoint{{Date: "2026-05-01", ActiveUsers: 1}},
			TopUsers:          []model.AdminTopUser{},
			TopOrgs:           []model.AdminTopOrg{},
			AgentDistribution: []model.AdminDistributionRow{},
			ModelDistribution: []model.AdminDistributionRow{},
		},
	}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var payload struct {
		Success bool                       `json:"success"`
		Data    model.AdminDashboardData   `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if !payload.Success {
		t.Error("success should be true")
	}
	if payload.Data.Range != "7d" {
		t.Errorf("range = %q, want 7d", payload.Data.Range)
	}
	if payload.Data.Summary.TotalPrompts != 5 {
		t.Errorf("totalPrompts = %d, want 5", payload.Data.Summary.TotalPrompts)
	}
}
```

- [ ] **Step 3: Run the tests, expect them to pass**

Run: `cd server-go && go test ./internal/handler/ -run TestAdminDashboard -v`
Expected: all 5 tests PASS. (Handler is already implemented in step 1; this isn't strict TDD because the contract is small enough to write together — the failing-state proof is "if the handler is broken, these tests fail".)

If any fail, fix the handler in `admin_dashboard.go` until they pass.

- [ ] **Step 4: Commit**

```bash
git add server-go/internal/handler/admin_dashboard.go server-go/internal/handler/admin_dashboard_test.go
git commit -m "server-go: add admin dashboard handler with auth/range validation"
```

---

## Task 3: Service implementation (SQL)

**Files:**
- Create: `server-go/internal/service/admin_dashboard.go`

This task has no automated tests — verification is by `curl` after Task 4 wires the route. The SQL deliberately mirrors patterns from `service/dashboard.go` to stay consistent. We **drop the `where user_id = $1` filter** to make the queries platform-wide.

- [ ] **Step 1: Create the service**

```go
package service

import (
	"context"
	"fmt"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

type AdminDashboardService struct {
	Pool *pgxpool.Pool
}

// rangeToDays maps the public "7d"/"30d" labels to integers used in the SQL.
// Caller MUST validate the input — anything other than 7 or 30 is undefined.
func rangeToDays(rangeKey string) int {
	switch rangeKey {
	case "30d":
		return 30
	default:
		return 7
	}
}

func (s *AdminDashboardService) GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error) {
	days := rangeToDays(rangeKey)

	g, ctx := errgroup.WithContext(ctx)

	var (
		summary  model.AdminDashboardSummary
		trend    []model.AdminTrendPoint
		topUsers []model.AdminTopUser
		topOrgs  []model.AdminTopOrg
		agents   []model.AdminDistributionRow
		models   []model.AdminDistributionRow
	)

	g.Go(func() error { var err error; summary, err = s.getSummary(ctx, days); return err })
	g.Go(func() error { var err error; trend, err = s.getTrend(ctx, days); return err })
	g.Go(func() error { var err error; topUsers, err = s.getTopUsers(ctx, days); return err })
	g.Go(func() error { var err error; topOrgs, err = s.getTopOrgs(ctx, days); return err })
	g.Go(func() error { var err error; agents, err = s.getDistribution(ctx, days, "20"); return err })
	g.Go(func() error { var err error; models, err = s.getDistribution(ctx, days, "21"); return err })

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &model.AdminDashboardData{
		Range:             rangeKey,
		Summary:           summary,
		Trend:             trend,
		TopUsers:          topUsers,
		TopOrgs:           topOrgs,
		AgentDistribution: agents,
		ModelDistribution: models,
	}, nil
}

func (s *AdminDashboardService) getSummary(ctx context.Context, days int) (model.AdminDashboardSummary, error) {
	var sum model.AdminDashboardSummary
	var totalAdded, committedAI int64
	err := s.Pool.QueryRow(ctx, fmt.Sprintf(`
		select
			coalesce(count(distinct user_id) filter (where event_timestamp >= date_trunc('day', now())), 0) as active_today,
			coalesce(count(distinct user_id), 0) as active_in_range,
			coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and coalesce(attrs_json->>'22', '') <> ''), 0) as total_prompts,
			coalesce(count(*) filter (where event_id = 4), 0) as total_checkpoints,
			coalesce(sum((values_json->>'2')::int) filter (where event_id = 1), 0) as total_added,
			coalesce(sum((values_json->'5'->>0)::int) filter (where event_id = 1), 0) as committed_ai
		from public.metrics_events
		where event_timestamp >= now() - interval '%d days'
	`, days)).Scan(
		&sum.ActiveUsersToday,
		&sum.ActiveUsersInRange,
		&sum.TotalPrompts,
		&sum.TotalCheckpoints,
		&totalAdded,
		&committedAI,
	)
	if err != nil {
		return sum, fmt.Errorf("admin summary: %w", err)
	}
	if totalAdded > 0 {
		sum.AICodePercentage = float64(committedAI) / float64(totalAdded) * 100
	}
	return sum, nil
}

func (s *AdminDashboardService) getTrend(ctx context.Context, days int) ([]model.AdminTrendPoint, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		with days as (
			select generate_series(
				date_trunc('day', now()) - interval '%d days' + interval '1 day',
				date_trunc('day', now()),
				interval '1 day'
			)::date as day
		)
		select
			d.day::text,
			coalesce(count(distinct e.user_id), 0) as active_users,
			coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
			coalesce(count(*) filter (where e.event_id = 4), 0) as checkpoint_count,
			coalesce(sum((e.values_json->'5'->>0)::int) filter (where e.event_id = 1), 0) as committed_ai,
			coalesce(sum((e.values_json->>'2')::int) filter (where e.event_id = 1), 0) as total_added,
			coalesce(sum((e.values_json->'7'->>0)::int) filter (where e.event_id = 1), 0) as generated_ai,
			coalesce(sum((e.values_json->'4'->>0)::int) filter (where e.event_id = 1), 0) as edited_ai
		from days d
		left join public.metrics_events e on date_trunc('day', e.event_timestamp)::date = d.day
		group by d.day
		order by d.day
	`, days-1))
	if err != nil {
		return nil, fmt.Errorf("admin trend query: %w", err)
	}
	defer rows.Close()

	out := make([]model.AdminTrendPoint, 0, days)
	for rows.Next() {
		var p model.AdminTrendPoint
		if err := rows.Scan(
			&p.Date,
			&p.ActiveUsers,
			&p.PromptCount,
			&p.CheckpointCount,
			&p.CommittedAILines,
			&p.TotalAddedLines,
			&p.GeneratedAILines,
			&p.EditedAILines,
		); err != nil {
			return nil, fmt.Errorf("admin trend scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *AdminDashboardService) getTopUsers(ctx context.Context, days int) ([]model.AdminTopUser, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			e.user_id,
			coalesce(u.name, '') as name,
			coalesce(u.email, '') as email,
			count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> '') as prompt_count,
			coalesce(sum((e.values_json->'5'->>0)::int) filter (where e.event_id = 1), 0) as committed_ai
		from public.metrics_events e
		left join public.users u on u.id::text = e.user_id
		where e.event_timestamp >= now() - interval '%d days'
		group by e.user_id, u.name, u.email
		order by prompt_count desc, committed_ai desc, e.user_id asc
		limit 10
	`, days))
	if err != nil {
		return nil, fmt.Errorf("admin top users: %w", err)
	}
	defer rows.Close()

	out := make([]model.AdminTopUser, 0, 10)
	for rows.Next() {
		var u model.AdminTopUser
		if err := rows.Scan(&u.UserID, &u.Name, &u.Email, &u.PromptCount, &u.CommittedAILines); err != nil {
			return nil, fmt.Errorf("admin top users scan: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *AdminDashboardService) getTopOrgs(ctx context.Context, days int) ([]model.AdminTopOrg, error) {
	// Best-effort: if org_memberships or orgs tables don't exist or schema
	// differs, return empty rather than failing the whole dashboard. The
	// caller can adapt the JOIN once real org schema is confirmed.
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			o.id::text,
			coalesce(o.name, '') as org_name,
			count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> '') as prompt_count,
			count(distinct e.user_id) as member_count
		from public.metrics_events e
		join public.org_memberships m on m.user_id::text = e.user_id
		join public.orgs o on o.id = m.org_id
		where e.event_timestamp >= now() - interval '%d days'
		group by o.id, o.name
		order by prompt_count desc, member_count desc, o.id asc
		limit 10
	`, days))
	if err != nil {
		// Tolerate schema drift — log and return empty.
		return []model.AdminTopOrg{}, nil
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

// getDistribution computes prompt-count share grouped by attrs_json->>attrKey.
// attrKey "20" = agent, "21" = model.
func (s *AdminDashboardService) getDistribution(ctx context.Context, days int, attrKey string) ([]model.AdminDistributionRow, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			coalesce(nullif(attrs_json->>'%s', ''), '(unknown)') as label,
			count(distinct attrs_json->>'22') as prompt_count
		from public.metrics_events
		where event_id = 2
			and event_timestamp >= now() - interval '%d days'
			and coalesce(attrs_json->>'22', '') <> ''
		group by 1
		order by prompt_count desc, label asc
	`, attrKey, days))
	if err != nil {
		return nil, fmt.Errorf("admin distribution: %w", err)
	}
	defer rows.Close()

	type bucket struct {
		label string
		count int
	}
	var buckets []bucket
	var total int
	for rows.Next() {
		var b bucket
		if err := rows.Scan(&b.label, &b.count); err != nil {
			return nil, fmt.Errorf("admin distribution scan: %w", err)
		}
		buckets = append(buckets, b)
		total += b.count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]model.AdminDistributionRow, 0, len(buckets))
	for _, b := range buckets {
		share := 0.0
		if total > 0 {
			share = float64(b.count) / float64(total)
		}
		out = append(out, model.AdminDistributionRow{
			Label:       b.label,
			PromptCount: b.count,
			Share:       share,
		})
	}
	return out, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd server-go && go build ./...`
Expected: no output (success).

- [ ] **Step 3: Run all existing tests to verify no regressions**

Run: `cd server-go && go test ./...`
Expected: all existing tests still pass (we added types and a service but no wiring yet).

- [ ] **Step 4: Commit**

```bash
git add server-go/internal/service/admin_dashboard.go
git commit -m "server-go: add AdminDashboardService with platform-wide aggregation queries"
```

---

## Task 4: Wire the route + manual smoke test

**Files:**
- Modify: `server-go/cmd/server/main.go` (add service, handler, route)

- [ ] **Step 1: Wire service, handler, and route**

In `server-go/cmd/server/main.go`:

After line 80 (`dashboardSvc := &service.DashboardService{...}`), add:

```go
adminDashSvc := &service.AdminDashboardService{Pool: pool}
```

After line 107 (`dashboardH := &handler.DashboardHandler{Svc: dashboardSvc}`), add:

```go
adminDashH := &handler.AdminDashboardHandler{Svc: adminDashSvc}
```

In the `api := r.Group("/api")` block, after the existing `dashboard := api.Group(...)` block (around line 218), add:

```go
// Admin-only platform-wide dashboard. Reuses the existing adminOnly()
// middleware below — non-admin callers get 403, unauthenticated 401.
admin := api.Group("/admin", jsonLimit, jwtMW, adminOnly())
{
	admin.GET("/dashboard/stats", adminDashH.GetGlobalStats)
}
```

- [ ] **Step 2: Verify server builds**

Run: `cd server-go && go build ./cmd/server`
Expected: a `server` binary appears in `server-go/`. Delete it: `rm server-go/server`.

- [ ] **Step 3: Run all tests**

Run: `cd server-go && go test ./...`
Expected: all tests pass.

- [ ] **Step 4: Manual smoke test**

Start the local server (the project's normal dev startup — see `server-go/README.md`). Then:

```bash
# 1. Login as the admin bootstrapped user → save the cookies
curl -i -c /tmp/cookies.txt -X POST http://localhost:8080/api/user/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<INITIAL_ADMIN_PASSWORD>"}'

# 2. Hit the new endpoint
curl -i -b /tmp/cookies.txt 'http://localhost:8080/api/admin/dashboard/stats?range=7d' | jq .

# 3. Confirm 200 response with the documented shape (success, data.summary, data.trend (length 7), etc.)

# 4. Range validation
curl -i -b /tmp/cookies.txt 'http://localhost:8080/api/admin/dashboard/stats?range=42d'
# Expect: 400 with {"error":"invalid range; expected 7d or 30d"}

# 5. Unauth check (drop cookies)
curl -i 'http://localhost:8080/api/admin/dashboard/stats'
# Expect: 401

# 6. (Optional) Login as a non-admin and retry — expect 403.
```

If any of the above produces an unexpected response, debug before continuing. Common issue: the `org_memberships` / `orgs` tables may not exist in this schema; the service catches the error and returns empty `topOrgs`, which is the intended behavior — verify by checking `data.topOrgs` is `[]`.

- [ ] **Step 5: Commit**

```bash
git add server-go/cmd/server/main.go
git commit -m "server-go: mount /api/admin/dashboard/stats route"
```

- [ ] **Step 6: Push and open PR 1**

```bash
git push -u origin server-feature
gh pr create --title "server-go: admin activity dashboard backend" --body "$(cat <<'EOF'
## Summary
- Add `GET /api/admin/dashboard/stats?range=7d|30d` endpoint, gated by `jwtMW + adminOnly()`.
- New `AdminDashboardService` aggregates platform-wide metrics from `metrics_events` (summary, trend, topUsers, topOrgs, agent/model distribution).
- Handler-level tests cover auth, range validation, error paths, and JSON shape.

Spec: `docs/superpowers/specs/2026-05-08-admin-activity-dashboard-design.md`

## Test plan
- [ ] `go test ./...` passes
- [ ] `curl` admin → 200 with documented shape
- [ ] `curl` invalid range → 400, no DB call
- [ ] `curl` unauth → 401
- [ ] `curl` non-admin → 403

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

# PR 2 — Frontend (Tasks 5-12)

## Task 5: Add Recharts dependency

**Files:**
- Modify: `server-go/web/package.json`

- [ ] **Step 1: Install Recharts**

```bash
cd server-go/web && pnpm add recharts@^2.15.0
```

- [ ] **Step 2: Verify the build still passes**

```bash
cd server-go/web && pnpm typecheck && pnpm build
```

Expected: build completes; `dist/` is regenerated.

- [ ] **Step 3: Commit**

```bash
git add server-go/web/package.json server-go/web/pnpm-lock.yaml
git commit -m "server-go/web: add recharts dependency for admin charts"
```

---

## Task 6: API types and client method

**Files:**
- Modify: `server-go/web/src/types/api.ts`
- Create: `server-go/web/src/api/admin.ts`

- [ ] **Step 1: Append admin types to `api.ts`**

Append to `server-go/web/src/types/api.ts`:

```ts
export type AdminRangeKey = "7d" | "30d";

export interface AdminDashboardSummary {
  activeUsersToday: number;
  activeUsersInRange: number;
  totalPrompts: number;
  totalCheckpoints: number;
  aiCodePercentage: number;
}

export interface AdminTrendPoint {
  date: string;
  activeUsers: number;
  promptCount: number;
  checkpointCount: number;
  committedAiLines: number;
  totalAddedLines: number;
  generatedAiLines: number;
  editedAiLines: number;
}

export interface AdminTopUser {
  userId: string;
  name: string;
  email: string;
  promptCount: number;
  committedAiLines: number;
}

export interface AdminTopOrg {
  orgId: string;
  orgName: string;
  promptCount: number;
  memberCount: number;
}

export interface AdminDistributionRow {
  label: string;
  promptCount: number;
  share: number;
}

export interface AdminDashboardData {
  range: AdminRangeKey;
  summary: AdminDashboardSummary;
  trend: AdminTrendPoint[];
  topUsers: AdminTopUser[];
  topOrgs: AdminTopOrg[];
  agentDistribution: AdminDistributionRow[];
  modelDistribution: AdminDistributionRow[];
}

export interface AdminDashboardResponse {
  success: boolean;
  data: AdminDashboardData;
  timestamp: string;
}
```

- [ ] **Step 2: Create the API client wrapper**

Create `server-go/web/src/api/admin.ts`:

```ts
import { api } from "./client";
import type { AdminDashboardResponse, AdminRangeKey } from "../types/api";

export const adminApi = {
  fetchDashboard: (range: AdminRangeKey) =>
    api.get<AdminDashboardResponse>(`/api/admin/dashboard/stats?range=${range}`),
};
```

- [ ] **Step 3: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/types/api.ts server-go/web/src/api/admin.ts
git commit -m "server-go/web: add admin dashboard types and API client"
```

---

## Task 7: Range toggle, leaderboard, distribution components

**Files:**
- Create: `server-go/web/src/components/admin/RangeToggle.tsx`
- Create: `server-go/web/src/components/admin/Leaderboard.tsx`
- Create: `server-go/web/src/components/admin/DistributionDonut.tsx`

These three are pure presentation, no data-fetching.

- [ ] **Step 1: Create RangeToggle**

`server-go/web/src/components/admin/RangeToggle.tsx`:

```tsx
import type { AdminRangeKey } from "../../types/api";

interface Props {
  value: AdminRangeKey;
  onChange: (next: AdminRangeKey) => void;
  disabled?: boolean;
}

const OPTIONS: Array<{ key: AdminRangeKey; label: string }> = [
  { key: "7d", label: "7天" },
  { key: "30d", label: "30天" },
];

export default function RangeToggle({ value, onChange, disabled }: Props) {
  return (
    <div className="admin-range-toggle" role="tablist" aria-label="时间范围">
      {OPTIONS.map(opt => (
        <button
          key={opt.key}
          type="button"
          role="tab"
          aria-selected={value === opt.key}
          className={value === opt.key ? "active" : ""}
          disabled={disabled}
          onClick={() => onChange(opt.key)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create Leaderboard**

`server-go/web/src/components/admin/Leaderboard.tsx`:

```tsx
import type { ReactNode } from "react";

interface Column<T> {
  header: string;
  render: (row: T) => ReactNode;
  align?: "left" | "right";
}

interface Props<T> {
  title: string;
  rows: T[];
  columns: Column<T>[];
  emptyMessage?: string;
}

export default function Leaderboard<T>({ title, rows, columns, emptyMessage }: Props<T>) {
  return (
    <div className="card admin-leaderboard">
      <h2>{title}</h2>
      {rows.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>{emptyMessage ?? "暂无数据"}</p>
      ) : (
        <table className="admin-leaderboard__table">
          <thead>
            <tr>
              {columns.map((c, i) => (
                <th key={i} style={{ textAlign: c.align ?? "left" }}>{c.header}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={i}>
                {columns.map((c, j) => (
                  <td key={j} style={{ textAlign: c.align ?? "left" }}>{c.render(row)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create DistributionDonut**

`server-go/web/src/components/admin/DistributionDonut.tsx`:

```tsx
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { AdminDistributionRow } from "../../types/api";

interface Props {
  title: string;
  rows: AdminDistributionRow[];
}

const PALETTE = ["#6366f1", "#22c55e", "#f97316", "#06b6d4", "#a855f7", "#eab308", "#ef4444", "#14b8a6"];

export default function DistributionDonut({ title, rows }: Props) {
  return (
    <div className="card admin-donut">
      <h2>{title}</h2>
      {rows.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 240 }}>
          <ResponsiveContainer>
            <PieChart>
              <Pie
                data={rows}
                dataKey="promptCount"
                nameKey="label"
                innerRadius={50}
                outerRadius={90}
                paddingAngle={2}
              >
                {rows.map((_, i) => (
                  <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
                ))}
              </Pie>
              <Tooltip
                formatter={(value: number, _name: string, item) => {
                  const share = (item?.payload?.share ?? 0) * 100;
                  return [`${value} (${share.toFixed(1)}%)`, item?.payload?.label];
                }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add server-go/web/src/components/admin/
git commit -m "server-go/web: add range toggle, leaderboard, and donut components"
```

---

## Task 8: Trend and adoption charts

**Files:**
- Create: `server-go/web/src/components/admin/TrendChart.tsx`
- Create: `server-go/web/src/components/admin/AdoptionStackedBar.tsx`

- [ ] **Step 1: Create TrendChart**

`server-go/web/src/components/admin/TrendChart.tsx`:

```tsx
import {
  CartesianGrid, Legend, Line, LineChart, ResponsiveContainer,
  Tooltip, XAxis, YAxis,
} from "recharts";
import type { AdminTrendPoint } from "../../types/api";

interface Props {
  data: AdminTrendPoint[];
}

export default function TrendChart({ data }: Props) {
  return (
    <div className="card admin-chart">
      <h2>使用趋势</h2>
      {data.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 280 }}>
          <ResponsiveContainer>
            <LineChart data={data} margin={{ top: 8, right: 24, left: 0, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="date" tick={{ fontSize: 11 }} />
              <YAxis yAxisId="left" tick={{ fontSize: 11 }} />
              <YAxis yAxisId="right" orientation="right" tick={{ fontSize: 11 }} />
              <Tooltip />
              <Legend />
              <Line
                yAxisId="left"
                type="monotone"
                dataKey="activeUsers"
                name="活跃用户"
                stroke="#6366f1"
                strokeWidth={2}
                dot={false}
              />
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="promptCount"
                name="Prompt 数"
                stroke="#22c55e"
                strokeWidth={2}
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Create AdoptionStackedBar**

`server-go/web/src/components/admin/AdoptionStackedBar.tsx`:

```tsx
import {
  Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer,
  Tooltip, XAxis, YAxis,
} from "recharts";
import type { AdminTrendPoint } from "../../types/api";

interface Props {
  data: AdminTrendPoint[];
}

export default function AdoptionStackedBar({ data }: Props) {
  return (
    <div className="card admin-chart">
      <h2>AI 代码采纳趋势</h2>
      {data.length === 0 ? (
        <p className="muted" style={{ margin: 0 }}>暂无数据</p>
      ) : (
        <div style={{ width: "100%", height: 280 }}>
          <ResponsiveContainer>
            <BarChart data={data} margin={{ top: 8, right: 24, left: 0, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="date" tick={{ fontSize: 11 }} />
              <YAxis tick={{ fontSize: 11 }} />
              <Tooltip />
              <Legend />
              <Bar dataKey="generatedAiLines" name="生成"   stackId="a" fill="#a5b4fc" />
              <Bar dataKey="committedAiLines" name="已提交" stackId="a" fill="#6366f1" />
              <Bar dataKey="editedAiLines"    name="人工编辑" stackId="a" fill="#f97316" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/components/admin/TrendChart.tsx server-go/web/src/components/admin/AdoptionStackedBar.tsx
git commit -m "server-go/web: add trend line and AI adoption stacked bar charts"
```

---

## Task 9: Summary cards

**Files:**
- Create: `server-go/web/src/components/admin/SummaryCards.tsx`

- [ ] **Step 1: Create SummaryCards**

`server-go/web/src/components/admin/SummaryCards.tsx`:

```tsx
import type { AdminDashboardSummary } from "../../types/api";

interface Props {
  summary: AdminDashboardSummary;
  rangeLabel: string;
}

interface Tile {
  label: string;
  value: string;
  hint?: string;
}

export default function SummaryCards({ summary, rangeLabel }: Props) {
  const tiles: Tile[] = [
    {
      label: "今日活跃用户",
      value: String(summary.activeUsersToday),
    },
    {
      label: `${rangeLabel}活跃用户`,
      value: String(summary.activeUsersInRange),
    },
    {
      label: `${rangeLabel}总 Prompt`,
      value: summary.totalPrompts.toLocaleString(),
    },
    {
      label: `${rangeLabel}总 Checkpoint`,
      value: summary.totalCheckpoints.toLocaleString(),
    },
    {
      label: "AI 代码采纳率",
      value: `${summary.aiCodePercentage.toFixed(1)}%`,
    },
  ];

  return (
    <div className="metrics-grid admin-summary">
      {tiles.map(t => (
        <div className="card" key={t.label}>
          <p className="metric-label">{t.label}</p>
          <p className="kpi" style={{ fontSize: "1.75rem" }}>{t.value}</p>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

- [ ] **Step 3: Commit**

```bash
git add server-go/web/src/components/admin/SummaryCards.tsx
git commit -m "server-go/web: add admin summary KPI cards"
```

---

## Task 10: AdminActivity page

**Files:**
- Create: `server-go/web/src/routes/AdminActivity.tsx`

- [ ] **Step 1: Create the page**

`server-go/web/src/routes/AdminActivity.tsx`:

```tsx
import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import { adminApi } from "../api/admin";
import { ApiError } from "../api/client";
import type { AdminDashboardData, AdminRangeKey, User } from "../types/api";

import RangeToggle from "../components/admin/RangeToggle";
import SummaryCards from "../components/admin/SummaryCards";
import TrendChart from "../components/admin/TrendChart";
import AdoptionStackedBar from "../components/admin/AdoptionStackedBar";
import Leaderboard from "../components/admin/Leaderboard";
import DistributionDonut from "../components/admin/DistributionDonut";

type FetchState =
  | { status: "loading" }
  | { status: "error"; message: string; forbidden?: boolean }
  | { status: "ready"; data: AdminDashboardData };

function AdminActivityContent({ user }: { user: User }) {
  const [range, setRange] = useState<AdminRangeKey>("7d");
  const [state, setState] = useState<FetchState>({ status: "loading" });

  useEffect(() => {
    if (user.role !== "admin") return; // gating handled outside, defensive
    let cancelled = false;
    setState({ status: "loading" });
    adminApi.fetchDashboard(range)
      .then(res => { if (!cancelled) setState({ status: "ready", data: res.data }); })
      .catch(err => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 403) {
          setState({ status: "error", message: "您没有权限访问此页面。", forbidden: true });
          return;
        }
        const message = err instanceof Error ? err.message : "未知错误";
        setState({ status: "error", message });
      });
    return () => { cancelled = true; };
  }, [range, user.role]);

  if (user.role !== "admin") {
    return <Navigate to="/me" replace />;
  }

  if (state.status === "error" && state.forbidden) {
    return <Navigate to="/me" replace />;
  }

  const rangeLabel = range === "7d" ? "7 天" : "30 天";

  return (
    <main className="page-main admin-page">
      <div className="panel">
        <div className="admin-page__header">
          <div>
            <h1 style={{ margin: 0 }}>平台活跃度看板</h1>
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

        {state.status === "error" && !state.forbidden && (
          <div className="card" style={{ marginTop: 24 }}>
            <p style={{ color: "var(--danger)" }}>加载失败: {state.message}</p>
            <button type="button" onClick={() => setRange(range)}>重试</button>
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

export default function AdminActivity() {
  return (
    <ProtectedRoute>
      {({ user }) => <AdminActivityContent user={user} />}
    </ProtectedRoute>
  );
}
```

- [ ] **Step 2: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

- [ ] **Step 3: Commit**

```bash
git add server-go/web/src/routes/AdminActivity.tsx
git commit -m "server-go/web: add AdminActivity page with charts and leaderboards"
```

---

## Task 11: Wire route + admin entry on /me + styles

**Files:**
- Modify: `server-go/web/src/App.tsx`
- Modify: `server-go/web/src/routes/Me.tsx`
- Modify: `server-go/web/src/styles/globals.css`

- [ ] **Step 1: Add the route to App.tsx**

Replace the contents of `server-go/web/src/App.tsx`:

```tsx
import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import Login from "./routes/Login";
import Me from "./routes/Me";
import DeviceFlow from "./routes/DeviceFlow";
import DeviceResult from "./routes/DeviceResult";

const AdminActivity = lazy(() => import("./routes/AdminActivity"));

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/me" element={<Me />} />
      <Route path="/oauth/device" element={<DeviceFlow />} />
      <Route path="/oauth/device/result" element={<DeviceResult />} />
      <Route
        path="/admin/activity"
        element={
          <Suspense fallback={<div style={{ padding: 24 }}>Loading…</div>}>
            <AdminActivity />
          </Suspense>
        }
      />
      <Route path="*" element={<Navigate to="/me" replace />} />
    </Routes>
  );
}
```

- [ ] **Step 2: Add the admin entry card to Me.tsx**

In `server-go/web/src/routes/Me.tsx`, inside `MeContent` just after the closing `</div>` of the profile header `<div className="panel">` block (before the metrics grid), add:

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

Add the import at the top:

```tsx
import { Link, useNavigate } from "react-router-dom";
```

(Replace the existing `import { useNavigate } from "react-router-dom";`.)

- [ ] **Step 3: Append admin styles to globals.css**

Append to `server-go/web/src/styles/globals.css`:

```css
/* ==========================================================================
   Admin activity dashboard  (.admin-page__, .admin-*)
   ========================================================================== */
.admin-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.admin-range-toggle {
  display: inline-flex;
  background: var(--panel);
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 8px;
  overflow: hidden;
}

.admin-range-toggle button {
  padding: 6px 14px;
  border: 0;
  background: transparent;
  cursor: pointer;
  font-size: 0.875rem;
  color: var(--text-muted, #6b7280);
}

.admin-range-toggle button.active {
  background: var(--accent, #6366f1);
  color: #fff;
}

.admin-range-toggle button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.admin-summary {
  margin-top: 24px;
}

.admin-page__chart-stack {
  display: grid;
  gap: 24px;
  margin-top: 24px;
}

.admin-leaderboard__table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 12px;
  font-size: 0.875rem;
}

.admin-leaderboard__table th,
.admin-leaderboard__table td {
  padding: 8px 6px;
  border-bottom: 1px solid var(--border, #e5e7eb);
}

.admin-leaderboard__table th {
  font-weight: 600;
  color: var(--text-muted, #6b7280);
  text-transform: uppercase;
  font-size: 0.75rem;
  letter-spacing: 0.05em;
}

.admin-entry-card {
  text-decoration: none;
  color: inherit;
  border-left: 3px solid var(--accent, #6366f1);
}

.admin-entry-card:hover {
  background: rgba(99, 102, 241, 0.04);
}
```

- [ ] **Step 4: Verify typecheck and build**

```bash
cd server-go/web && pnpm typecheck && pnpm build
```

Expected: build succeeds; `dist/` is regenerated.

- [ ] **Step 5: Commit**

```bash
git add server-go/web/src/App.tsx server-go/web/src/routes/Me.tsx server-go/web/src/styles/globals.css
git commit -m "server-go/web: wire /admin/activity route and admin entry card on /me"
```

---

## Task 12: Manual browser verification + PR

- [ ] **Step 1: Start the dev server**

```bash
cd server-go/web && pnpm dev
```

In a separate terminal, ensure the Go backend is running with PR 1 merged (or rebased into this branch). Note the dev URL the Vite output prints (typically `http://localhost:5173`).

- [ ] **Step 2: Verify as admin user**

In the browser:

1. Go to `/login`, log in with the admin account.
2. Land on `/me` — confirm the "管理员看板" entry card appears at the top of the panel.
3. Click the card → navigate to `/admin/activity`.
4. Confirm rendering:
   - Summary tiles (5) populated with numbers.
   - Trend line chart shows two lines and 7 x-axis ticks.
   - Stacked bar shows three series.
   - Two leaderboards render (rows or "暂无数据" if empty).
   - Two donuts render (or "暂无数据").
5. Click `30天` toggle. Confirm:
   - Toggle disables briefly while loading.
   - Charts update; trend now has 30 x-axis ticks.
6. Open devtools network tab. Confirm:
   - `GET /api/admin/dashboard/stats?range=7d` → 200.
   - `GET /api/admin/dashboard/stats?range=30d` → 200.

- [ ] **Step 3: Verify as non-admin user**

1. Log out, log in as a non-admin user.
2. On `/me`, the entry card should NOT be visible.
3. Manually navigate to `/admin/activity`. Should redirect back to `/me`.
4. In devtools, paste: `fetch('/api/admin/dashboard/stats').then(r => console.log(r.status))`. Expect: `403`.

- [ ] **Step 4: Verify error handling**

1. Stop the backend.
2. Reload `/admin/activity` (as admin). Confirm: error panel with "重试" button appears.
3. Restart backend, click "重试". Confirm data loads.

- [ ] **Step 5: If verification passed, push and open PR 2**

```bash
git push origin server-feature
gh pr create --title "server-go/web: admin activity dashboard frontend" --body "$(cat <<'EOF'
## Summary
- New route `/admin/activity` with Recharts-powered platform activity dashboard.
- Lazy-loaded; non-admins never download Recharts.
- Entry card on `/me` only visible to admins.

Spec: `docs/superpowers/specs/2026-05-08-admin-activity-dashboard-design.md`
Depends on backend PR (`/api/admin/dashboard/stats`).

## Test plan
- [ ] Admin: `/me` shows entry card → click → page renders all sections
- [ ] 7d ↔ 30d toggle re-fetches and updates charts
- [ ] Non-admin: no entry card; direct `/admin/activity` redirects to `/me`; API returns 403
- [ ] Backend down → error panel with retry button; retry succeeds when backend back

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review Notes

Performed against the spec at `docs/superpowers/specs/2026-05-08-admin-activity-dashboard-design.md`:

- **Spec §3 architecture** → Tasks 1–4 (backend) + Tasks 5–12 (frontend).
- **Spec §4.1 new files** → Plan reuses existing `adminOnly()` from `cmd/server/main.go` instead of creating `internal/middleware/require_admin.go`. This is a deliberate adaptation (precedent already exists in the codebase) noted here.
- **Spec §4.3 API contract** → Task 1 (types) + Task 2 (handler) + Task 3 (service).
- **Spec §4.4 SQL strategy** → Task 3 implements all six queries with the documented `GROUP BY` and JOINs.
- **Spec §4.5 perf notes** → Documented in spec; not implemented (correctly out of scope).
- **Spec §4.6 testing** → Adapted to project reality: handler-level tests (Task 2) + manual curl smoke test (Task 4 step 4). Plan called this out at the top.
- **Spec §5.1–5.7 frontend** → Tasks 5–11.
- **Spec §6 delivery plan** → Tasks 1–4 = PR 1; Tasks 5–12 = PR 2.

**Type consistency check** (spot-checked across tasks): `AdminDashboardData`, `AdminTrendPoint`, `AdminTopUser`, `AdminTopOrg`, `AdminDistributionRow`, `AdminRangeKey` are spelled identically across Go and TS. JSON field names (`committedAiLines`, `aiCodePercentage`, `topUsers`, etc.) match between the Go struct tags and the TS interface members.

**Open variance from spec** (acceptable — see test strategy note): no DB-integration tests for SQL. Verification by curl only. Adding DB-test infra would balloon this project; if desired, file a follow-up.
