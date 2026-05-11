# Prompt 计数回归修复 + Dashboard 小幅完善 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 dashboard "Prompt 数" 系列指标随新版客户端上报衰减到 0 的回归（客户端已 tombstone attr index 22 / prompt_id），改用 `session_id` 做 distinct 计数；同步前端 UI 文案为"会话"；顺手补齐已有但前端未渲染的 `checkpointByEditKind` 卡片、修复移动端 grid 溢出。

**Architecture:**
- 后端：`service/dashboard.go` + `service/admin_dashboard.go` 共 12 处 SQL 把 `attrs_json->>'22'` 改为 `coalesce(session_id, attrs_json->>'24')`。新数据走 `session_id` 列（Task 1 commit `2b65d1c4` 已建列+索引），历史数据回退到 `attrs_json->>'24'`（同义 attrs 索引），实现新旧数据无缝平滑。
- 前端：`types/api.ts` `AdminDashboardData` 补 `checkpointByEditKind` 字段；UI 文案 `Prompt` 系列统一改为 `会话`；`Dashboard.tsx` 加第三个 `DistributionDonut`；Dashboard grid `minmax(400px, 1fr)` 改为 `minmax(min(400px, 100%), 1fr)` 修复移动端溢出。
- 类型字段 `promptCount` 保留不动（前端组件 `DistributionDonut.tsx` / `TrendChart.tsx` 写死 `dataKey="promptCount"`，且 server 模型注释已说明该字段在不同 distribution 上下文中表达不同 count 语义）。

**Tech Stack:** Go / pgx (后端); React 19 + RR7 + Vite 8 + TypeScript 6 + recharts 2.15 (前端)。

**范围外（本次不做）：** D1 drill-down + filter 面板（下一个 plan）；D2 Leaderboard 分页；C 类新维度（按 author/branch/edit_kind 趋势聚合）；P2/P3 其他项。

**部署顺序提示：** 单 PR、单次部署。先停 server，部署新 build（无新迁移），起 server。旧数据 attrs_json["22"] 在 7d/30d 窗口内仍可读但 dashboard 不再使用；历史 attrs_json["24"] 通过 coalesce 仍能贡献计数；新数据走 session_id 独立列。

---

## File Structure

**修改文件：**
- `server-go/internal/service/dashboard.go` — 5 处 SQL（getOverview, getTopAgent, getTopModel, getTodaySummary）
- `server-go/internal/service/admin_dashboard.go` — 5 处 SQL（getSummary, getTrend, getTopUsers, getTopOrgs, getDistribution）
- `server-go/internal/service/dashboard_test.go` — 新建（首个 dashboard 集成测试）
- `server-go/internal/service/admin_dashboard_test.go` — 新建（首个 service-layer admin_dashboard 集成测试）
- `server-go/web/src/types/api.ts` — `AdminDashboardData` 加 `checkpointByEditKind` 字段
- `server-go/web/src/routes/Dashboard.tsx` — 加第三个 DistributionDonut；Leaderboard 列头 "Prompt"→"会话"；grid `minmax` 修复
- `server-go/web/src/routes/Me.tsx` — 文案 "Prompt"→"会话"
- `server-go/web/src/components/admin/SummaryCards.tsx` — 文案 "总 Prompt"→"总会话数"
- `server-go/web/src/components/admin/TrendChart.tsx` — line name "Prompt 数"→"会话数"

**不需要修改：**
- `model/admin_dashboard.go`、`handler/admin_dashboard.go`、`internal/database/migrations/*` — 无 schema 变更。
- `components/admin/DistributionDonut.tsx` / `Leaderboard.tsx` — 通用组件，`promptCount` dataKey 保持。

---

## Task 1: 后端 SQL — `attrs_json->>'22'` 改为 `coalesce(session_id, attrs_json->>'24')`

**Files:**
- Modify: `server-go/internal/service/dashboard.go` (lines 166, 192, 197, 221, 226, 250)
- Modify: `server-go/internal/service/admin_dashboard.go` (lines 75, 110, 152, 182, 273, 277)
- Create: `server-go/internal/service/admin_dashboard_test.go`

- [ ] **Step 1: 写失败测试覆盖 admin dashboard 的 session-based 计数**

创建 `server-go/internal/service/admin_dashboard_test.go`，内容：

```go
package service

import (
	"testing"

	"git-ai-server/internal/model"
)

func TestGetSummaryCountsBySessionId(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t)

	// Seed: 3 AgentUsage events (event_id=2) under 2 distinct session_ids
	// (s1 with two events, s2 with one). New-style records use session_id
	// column; we leave attrs_json["22"] empty to simulate post-tombstone data.
	metricsSvc := &MetricsService{Pool: pool}
	batch := &model.MetricsBatch{
		V: 1,
		Events: []model.MetricsEvent{
			{IsObject: true, T: 1715000000, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
			{IsObject: true, T: 1715000001, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
			{IsObject: true, T: 1715000002, E: 2, V: map[string]any{}, A: map[string]any{"0": "1.4.7", "24": "s2", "20": "cursor", "21": "gpt-5.4"}},
		},
	}
	errs, err := metricsSvc.UploadBatch(t.Context(), "00000000-0000-0000-0000-000000000001", nil, batch)
	if err != nil {
		t.Fatalf("seed UploadBatch error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("seed UploadBatch errors = %+v", errs)
	}

	adminSvc := &AdminDashboardService{Pool: pool}
	data, err := adminSvc.GetGlobalStats(t.Context(), "7d")
	if err != nil {
		t.Fatalf("GetGlobalStats error = %v", err)
	}

	if data.Summary.TotalPrompts != 2 {
		t.Fatalf("Summary.TotalPrompts = %d, want 2 (distinct session_ids); attrs[\"22\"] is empty so old SQL would return 0", data.Summary.TotalPrompts)
	}

	// Agent distribution: 2 sessions under claude-code (s1), 1 under cursor (s2)
	agents := map[string]int{}
	for _, row := range data.AgentDistribution {
		agents[row.Label] = row.PromptCount
	}
	if agents["claude-code"] != 1 || agents["cursor"] != 1 {
		t.Fatalf("AgentDistribution = %+v; want claude-code=1, cursor=1 (one distinct session each)", data.AgentDistribution)
	}
}
```

> 说明：种子数据故意只写 `attrs["24"]` (session_id) 不写 `attrs["22"]` (prompt_id) — 模拟客户端 tombstone 后的状态。旧 SQL `count(distinct attrs_json->>'22')` 会返回 0；新 SQL 必须返回 2。这就是 P0 的核心契约。

- [ ] **Step 2: 跑测试验证它失败**

```bash
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_metrics_test;" -c "CREATE DATABASE git_ai_metrics_test;"
cd /Users/hg/git/git-ai/server-go && METRICS_TEST_DATABASE_URL='postgres://127.0.0.1:5432/git_ai_metrics_test?sslmode=disable' \
  go test ./internal/service/ -run TestGetSummaryCountsBySessionId -v
```

期望：FAIL，错误信息形如 `Summary.TotalPrompts = 0, want 2 (distinct session_ids); ...`。

- [ ] **Step 3: 修改 admin_dashboard.go — 把所有 attrs_json->>'22' 替换为 coalesce(session_id, attrs_json->>'24')**

编辑 `server-go/internal/service/admin_dashboard.go`，做 5 处替换：

a) **getSummary（约 line 75）** — 把原行：
```go
				coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and coalesce(attrs_json->>'22', '') <> ''), 0) as total_prompts,
```
改为：
```go
				coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''), 0) as total_prompts,
```

b) **getTrend（约 line 110）** — 原行：
```go
				coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
```
改为：
```go
				coalesce(count(distinct coalesce(e.session_id, e.attrs_json->>'24')) filter (where e.event_id = 2 and coalesce(coalesce(e.session_id, e.attrs_json->>'24'), '') <> ''), 0) as prompt_count,
```

c) **getTopUsers（约 line 152）** — 原行：
```go
				coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
```
改为（与 b 同样替换）：
```go
				coalesce(count(distinct coalesce(e.session_id, e.attrs_json->>'24')) filter (where e.event_id = 2 and coalesce(coalesce(e.session_id, e.attrs_json->>'24'), '') <> ''), 0) as prompt_count,
```

d) **getTopOrgs（约 line 182）** — 原行：
```go
				coalesce(count(distinct e.attrs_json->>'22') filter (where e.event_id = 2 and coalesce(e.attrs_json->>'22', '') <> ''), 0) as prompt_count,
```
改为（与 b/c 相同）：
```go
				coalesce(count(distinct coalesce(e.session_id, e.attrs_json->>'24')) filter (where e.event_id = 2 and coalesce(coalesce(e.session_id, e.attrs_json->>'24'), '') <> ''), 0) as prompt_count,
```

e) **getDistribution（约 line 273）** — 原 SQL 块：
```go
		select
			coalesce(nullif(attrs_json->>'%s', ''), '(unknown)') as label,
			count(distinct attrs_json->>'22') as prompt_count
		from public.metrics_events
		where event_id = 2
			and event_timestamp >= now() - interval '%d days'
			and coalesce(attrs_json->>'22', '') <> ''
		group by 1
		order by prompt_count desc, label asc
```
改为：
```go
		select
			coalesce(nullif(attrs_json->>'%s', ''), '(unknown)') as label,
			count(distinct coalesce(session_id, attrs_json->>'24')) as prompt_count
		from public.metrics_events
		where event_id = 2
			and event_timestamp >= now() - interval '%d days'
			and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''
		group by 1
		order by prompt_count desc, label asc
```

- [ ] **Step 4: 修改 dashboard.go — 同样的 5 处替换**

编辑 `server-go/internal/service/dashboard.go`，做以下替换：

a) **getOverview（约 line 166）** — 原行：
```go
				coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and coalesce(attrs_json->>'22', '') <> ''), 0) as active_prompts,
```
改为：
```go
				coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''), 0) as active_prompts,
```

b) **getTopAgent（约 line 192）** — 原 SQL 块：
```go
		select
			nullif(attrs_json->>'20', '') as label,
			count(distinct attrs_json->>'22') as prompt_count
		from public.metrics_events
		where user_id = $1
			and event_id = 2
			and event_timestamp >= now() - interval '7 days'
			and coalesce(attrs_json->>'22', '') <> ''
		group by 1
		order by 2 desc, 1 asc
		limit 1
```
改为：
```go
		select
			nullif(attrs_json->>'20', '') as label,
			count(distinct coalesce(session_id, attrs_json->>'24')) as prompt_count
		from public.metrics_events
		where user_id = $1
			and event_id = 2
			and event_timestamp >= now() - interval '7 days'
			and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''
		group by 1
		order by 2 desc, 1 asc
		limit 1
```

c) **getTopModel（约 line 221）** — 与 b 结构相同，把 `attrs_json->>'20'` 换为 `attrs_json->>'21'`，其余替换照 b：

```go
		select
			nullif(attrs_json->>'21', '') as label,
			count(distinct coalesce(session_id, attrs_json->>'24')) as prompt_count
		from public.metrics_events
		where user_id = $1
			and event_id = 2
			and event_timestamp >= now() - interval '7 days'
			and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''
		group by 1
		order by 2 desc, 1 asc
		limit 1
```

d) **getTodaySummary（约 line 250）** — 原行：
```go
				coalesce(count(distinct attrs_json->>'22') filter (where event_id = 2 and event_timestamp >= date_trunc('day', now()) and coalesce(attrs_json->>'22', '') <> ''), 0) as prompt_count,
```
改为：
```go
				coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and event_timestamp >= date_trunc('day', now()) and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''), 0) as prompt_count,
```

- [ ] **Step 5: 跑测试验证通过**

```bash
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_metrics_test;" -c "CREATE DATABASE git_ai_metrics_test;"
cd /Users/hg/git/git-ai/server-go && METRICS_TEST_DATABASE_URL='postgres://127.0.0.1:5432/git_ai_metrics_test?sslmode=disable' \
  go test ./internal/service/ -run TestGetSummaryCountsBySessionId -v
```
期望：PASS。

- [ ] **Step 6: build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
期望：build 通过，全 PASS。

- [ ] **Step 7: 提交 Task 1**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/internal/service/dashboard.go \
  server-go/internal/service/admin_dashboard.go \
  server-go/internal/service/admin_dashboard_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: count distinct sessions instead of tombstoned prompt_id

Client 1.4.6+ tombstoned attr index 22 (prompt_id, never re-written).
The dashboard's 12 SQL queries all used count(distinct attrs_json->>'22')
filtered by event_id=2 to compute "prompt_count" — under post-tombstone
traffic the count decays to 0 across 7d/30d windows.

Switch to count(distinct coalesce(session_id, attrs_json->>'24')):

  - New rows (Task 1 of attrs catch-up onward) populate the session_id
    column directly — uses the new btree index.
  - Old rows keep their attrs_json["24"] value (SESSION_ID has always
    been attr 24; only the prompt_id at 22 was tombstoned), so the
    coalesce fallback preserves historical counts.

Semantics now: "distinct AI sessions" rather than "distinct prompts",
which is the closest stable replacement available to the client side.
Frontend label changes follow in the next commit.
EOF
)"
```

---

## Task 2: 前端类型补字段 + UI 文案

**Files:**
- Modify: `server-go/web/src/types/api.ts` (line 86-94)
- Modify: `server-go/web/src/routes/Dashboard.tsx` (line 117, 126)
- Modify: `server-go/web/src/routes/Me.tsx` (lines 97, 100, 174 — 三处)
- Modify: `server-go/web/src/components/admin/SummaryCards.tsx` (line 25)
- Modify: `server-go/web/src/components/admin/TrendChart.tsx` (line 44)

- [ ] **Step 1: 补 AdminDashboardData.checkpointByEditKind 类型字段**

编辑 `server-go/web/src/types/api.ts`，把 `AdminDashboardData` interface 替换为：

```ts
export interface AdminDashboardData {
  range: AdminRangeKey;
  summary: AdminDashboardSummary;
  trend: AdminTrendPoint[];
  topUsers: AdminTopUser[];
  topOrgs: AdminTopOrg[];
  agentDistribution: AdminDistributionRow[];
  modelDistribution: AdminDistributionRow[];
  checkpointByEditKind: AdminDistributionRow[];
}
```

- [ ] **Step 2: Dashboard.tsx 把 Leaderboard 列头 "Prompt" 改为 "会话"**

编辑 `server-go/web/src/routes/Dashboard.tsx`，把：

```tsx
                { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                { header: "AI 行数", render: (r) => r.committedAiLines.toLocaleString(), align: "right" },
```

替换为（topUsers 部分，约 line 117）：

```tsx
                { header: "会话", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                { header: "AI 行数", render: (r) => r.committedAiLines.toLocaleString(), align: "right" },
```

并把 topOrgs 部分（约 line 126）：

```tsx
                { header: "Prompt", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                { header: "成员", render: (r) => r.memberCount.toLocaleString(), align: "right" },
```

替换为：

```tsx
                { header: "会话", render: (r) => r.promptCount.toLocaleString(), align: "right" },
                { header: "成员", render: (r) => r.memberCount.toLocaleString(), align: "right" },
```

- [ ] **Step 3: Me.tsx 三处文案改**

编辑 `server-go/web/src/routes/Me.tsx`：

a) 把：
```tsx
          <p className="metric-label">活跃 Prompt 数</p>
          <p className="kpi">{act?.activePromptCount ?? "—"}</p>
```
改为：
```tsx
          <p className="metric-label">活跃会话数</p>
          <p className="kpi">{act?.activePromptCount ?? "—"}</p>
```

b) 把：
```tsx
            过去 7 天内独立 Prompt 统计
```
改为：
```tsx
            过去 7 天内独立会话统计
```

c) 把：
```tsx
            今日已有 <strong>{today!.activityCount}</strong> 条活动记录，涵盖 <strong>{today!.promptCount}</strong> 个 Prompt 及 <strong>{today!.fileCount}</strong> 个文件。
```
改为：
```tsx
            今日已有 <strong>{today!.activityCount}</strong> 条活动记录，涵盖 <strong>{today!.promptCount}</strong> 个会话及 <strong>{today!.fileCount}</strong> 个文件。
```

- [ ] **Step 4: SummaryCards.tsx "总 Prompt" 改 "总会话数"**

编辑 `server-go/web/src/components/admin/SummaryCards.tsx`，把：

```tsx
    {
      label: `${rangeLabel}总 Prompt`,
      value: summary.totalPrompts.toLocaleString(),
    },
```

替换为：

```tsx
    {
      label: `${rangeLabel}总会话数`,
      value: summary.totalPrompts.toLocaleString(),
    },
```

- [ ] **Step 5: TrendChart.tsx line name "Prompt 数" 改 "会话数"**

编辑 `server-go/web/src/components/admin/TrendChart.tsx`，把：

```tsx
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="promptCount"
                name="Prompt 数"
```

替换为：

```tsx
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="promptCount"
                name="会话数"
```

- [ ] **Step 6: typecheck + build 通过**

```bash
cd /Users/hg/git/git-ai/server-go/web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm build
```

期望：`tsc -b --noEmit` 无 error；`vite build` 输出 `dist/` 无报错。

> 如果 `pnpm install --frozen-lockfile` 因为 lock 不一致失败，可改用 `pnpm install`。本仓 web 默认使用 pnpm。

- [ ] **Step 7: grep 验证无残留 "Prompt" 文案**

```bash
cd /Users/hg/git/git-ai/server-go/web/src && grep -rn '"Prompt\|Prompt 数\|总 Prompt\|活跃 Prompt\|个 Prompt\|独立 Prompt' .
```

期望：零命中（所有用户可见的 "Prompt" 文案均已改为 "会话"）。`promptCount`、`activePromptCount`、`PromptCount` 等程序标识符不命中本 grep。

- [ ] **Step 8: 提交 Task 2**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/web/src/types/api.ts \
  server-go/web/src/routes/Dashboard.tsx \
  server-go/web/src/routes/Me.tsx \
  server-go/web/src/components/admin/SummaryCards.tsx \
  server-go/web/src/components/admin/TrendChart.tsx
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go/web: rename UI "Prompt" to "会话" and add checkpointByEditKind type

Backend now counts distinct session_id (prompt_id is tombstoned). The
JSON field name promptCount stays for component compatibility
(DistributionDonut / TrendChart key off it), but the user-facing label
should be "会话" to match the new semantics. Touches the four user-
visible strings on Dashboard.tsx, SummaryCards.tsx, TrendChart.tsx,
and three places on Me.tsx.

Also adds the missing AdminDashboardData.checkpointByEditKind field so
the next commit can render it without TS errors.
EOF
)"
```

---

## Task 3: Dashboard 加 CheckpointByEditKind 卡片

**Files:**
- Modify: `server-go/web/src/routes/Dashboard.tsx` (third donut row)

- [ ] **Step 1: 在第三排 grid 加 DistributionDonut**

编辑 `server-go/web/src/routes/Dashboard.tsx`，在现有最后一排 grid（line 132-135）：

```tsx
          <div className="grid">
            <DistributionDonut title="Agent 分布" rows={state.data.agentDistribution} />
            <DistributionDonut title="模型分布" rows={state.data.modelDistribution} />
          </div>
```

整体替换为（追加一行 grid）：

```tsx
          <div className="grid">
            <DistributionDonut title="Agent 分布" rows={state.data.agentDistribution} />
            <DistributionDonut title="模型分布" rows={state.data.modelDistribution} />
          </div>

          <div className="grid">
            <DistributionDonut title="Checkpoint 类型分布" rows={state.data.checkpointByEditKind} />
          </div>
```

> 单独一行的原因：Checkpoint 类型分布只有 2-3 个 buckets（file_edit / bash / (unknown)），与上一排 agent/model 不同语义；分独立行视觉上更清楚，rangeLabel 与 trend 区域的关系也保持单一对话。

- [ ] **Step 2: typecheck + build 通过**

```bash
cd /Users/hg/git/git-ai/server-go/web && pnpm typecheck && pnpm build
```

期望：无 error。

- [ ] **Step 3: 提交 Task 3**

```bash
cd /Users/hg/git/git-ai && git add server-go/web/src/routes/Dashboard.tsx
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go/web: render CheckpointByEditKind donut on team dashboard

Backend has been returning data.checkpointByEditKind since
a1bef4db but Dashboard.tsx had no widget. Add a third donut row
under the agent/model distributions, reusing DistributionDonut.
file_edit vs bash buckets give admins a quick read on whether the
team's AI usage skews toward editing files or executing shell tasks.
EOF
)"
```

---

## Task 4: 修复移动端 grid 溢出

**Files:**
- Modify: `server-go/web/src/routes/Dashboard.tsx` (lines 42, 106)

- [ ] **Step 1: Dashboard.tsx 两处 `minmax` 改宽松**

编辑 `server-go/web/src/routes/Dashboard.tsx`，把两处：

```tsx
        <div className="admin-page__chart-stack" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(400px, 1fr))", gap: 24 }}>
```

都改为：

```tsx
        <div className="admin-page__chart-stack" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(min(400px, 100%), 1fr))", gap: 24 }}>
```

> `minmax(min(400px, 100%), 1fr)`：当容器宽度 ≥ 400px 时与原行为一致（每列至少 400px），<400px 时退化为 `minmax(100%, 1fr)` 即单列铺满，不再溢出。两处都改：line 42（skeleton）+ line 106（实际内容）。

- [ ] **Step 2: typecheck + build 通过**

```bash
cd /Users/hg/git/git-ai/server-go/web && pnpm typecheck && pnpm build
```

期望：无 error。

- [ ] **Step 3: 提交 Task 4**

```bash
cd /Users/hg/git/git-ai && git add server-go/web/src/routes/Dashboard.tsx
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go/web: prevent admin chart grid from overflowing on narrow viewports

Dashboard.tsx used `minmax(400px, 1fr)` for the trend/adoption stack
and the skeleton row, so any viewport under 400px wide pushes the
column past the container and triggers horizontal scroll. Wrap the
minimum in `min(400px, 100%)` so narrow viewports collapse to a
single full-width column instead.
EOF
)"
```

---

## Task 5: 端到端本地 smoke

**Files:** 无代码修改。

- [ ] **Step 1: 重建 fresh smoke DB + 起 server**

```bash
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_smoke;" -c "CREATE DATABASE git_ai_smoke;"
cd /Users/hg/git/git-ai && go build -o /tmp/git-ai-server-smoke ./server-go/cmd/server
PORT=37337 APP_ENV=development \
  JWT_SECRET=smoke-jwt-secret-not-for-prod-just-for-test \
  DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=$USER DB_PASSWORD= DB_NAME=git_ai_smoke DB_SSL=false \
  GIT_AI_API_KEY=smoke-api-key \
  /tmp/git-ai-server-smoke > /tmp/git-ai-server-smoke.log 2>&1 &
sleep 2
curl -s http://127.0.0.1:37337/health
```

期望：`{"service":"git-ai-private-deploy-server","status":"ok"}`。

- [ ] **Step 2: 上传 3 个 AgentUsage 事件（2 个 session）**

```bash
curl -sS -X POST http://127.0.0.1:37337/worker/metrics/upload \
  -H "Content-Type: application/json" -H "X-API-Key: smoke-api-key" \
  -d '{
    "v": 1,
    "events": [
      {"t": 1715000000, "e": 2, "v": {}, "a": {"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
      {"t": 1715000001, "e": 2, "v": {}, "a": {"0": "1.4.7", "24": "s1", "20": "claude-code", "21": "gpt-5.4"}},
      {"t": 1715000002, "e": 2, "v": {}, "a": {"0": "1.4.7", "24": "s2", "20": "cursor",      "21": "gpt-5.4"}}
    ]
  }'
```

期望：`{"errors":[],"success":true}`。

注：所有 attrs 都不写 "22"（模拟 tombstone 后状态）。

- [ ] **Step 3: 直查 DB 验证 SQL 替换生效**

```bash
psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT count(distinct coalesce(session_id, attrs_json->>'24'))
   FROM metrics_events
   WHERE event_id = 2
     AND coalesce(coalesce(session_id, attrs_json->>'24'), '') <> '';"
```

期望：返回 `2`（s1, s2）。

对照查询（旧 SQL）：
```bash
psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT count(distinct attrs_json->>'22')
   FROM metrics_events
   WHERE event_id = 2 AND coalesce(attrs_json->>'22', '') <> '';"
```

期望：返回 `0`（证明旧 SQL 在新数据下确实失效）。

- [ ] **Step 4: 模拟登录后调 admin dashboard，验证 totalPrompts**

由于 `/api/dashboard/global` 需要 JWT，本步直接用 SQL 验证 service 层返回的 totalPrompts 与 Step 3 一致。已在 Task 1 的集成测试覆盖；smoke 这一步仅 sanity check：

```bash
psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT coalesce(count(distinct coalesce(session_id, attrs_json->>'24')) filter (where event_id = 2 and coalesce(coalesce(session_id, attrs_json->>'24'), '') <> ''), 0) as total_prompts FROM metrics_events WHERE event_timestamp >= now() - interval '7 days';"
```

期望：`total_prompts = 2`。

- [ ] **Step 5: 前端 build + 浏览器手动检查（可选）**

```bash
cd /Users/hg/git/git-ai/server-go/web && pnpm build
```

期望：`dist/` 生成，无 error。

可选浏览器手动检查：起 vite dev server (`pnpm dev`)，访问 `/me` 看"活跃会话数"、`/admin/activity` 看团队看板含三个 donut 行（Agent / 模型 / Checkpoint 类型）。如无现成的 admin token 登录工作流，跳过浏览器步骤，仅以 build 通过为验证。

- [ ] **Step 6: 关停 server**

```bash
pkill -f /tmp/git-ai-server-smoke || true
```

Task 5 无代码改动，不 commit。

---

## 完成标准

- 后端集成测试 `TestGetSummaryCountsBySessionId` PASS：在 attrs["22"] 为空的种子数据下，`Summary.TotalPrompts == 2`、`AgentDistribution` 含 `claude-code=1` / `cursor=1`。
- `go build ./...` + `go test ./...` 全 PASS（含 Task 1 polish 引入的 metrics 集成测试）。
- `pnpm typecheck && pnpm build` 在 `server-go/web/` 下成功，无 ts error。
- `grep '"Prompt\|Prompt 数\|总 Prompt\|活跃 Prompt\|个 Prompt\|独立 Prompt' server-go/web/src/` 零命中。
- `Dashboard.tsx` 渲染三行 donut（Agent / 模型 / Checkpoint 类型分布）。
- 本地 smoke 上传 3 个 AgentUsage 事件（无 attrs["22"]）后，新 SQL 返回 totalPrompts=2，旧 SQL 返回 0。
- 移动端窄屏（<400px）admin grid 不溢出（手动浏览或基于 `min(400px, 100%)` CSS 语义判定）。

## 回滚

- Task 4：revert Dashboard.tsx 中 minmax 改动。
- Task 3：revert Dashboard.tsx 中新增 DistributionDonut 行。
- Task 2：revert types/api.ts + UI 文案改动（仅前端层 revert，无 schema 变更）。
- Task 1：revert dashboard.go / admin_dashboard.go 的 SQL 改动；DB 无 schema 变更，无需 migrate down。代价：dashboard 指标回到 0（恢复到 tombstone 引入的 regression 状态）。

---

## 本地验证记录（2026-05-11）

实际在本机 macOS + PostgreSQL 15 上完成端到端 smoke（重建 `git_ai_smoke` fresh DB）。

**Commits chain**（base `b99cc68b` → HEAD `2ea8cb10`，共 7 个 commit）：

```
7a79807b server-go: count distinct sessions instead of tombstoned prompt_id
53fc9372 server-go: fix misleading comment in admin dashboard session count test
f48427c7 server-go/web: rename UI "Prompt" to "会话" and add checkpointByEditKind type
ec9fd817 server-go/web: render CheckpointByEditKind donut and unify "会话数" wording
f095119a server-go/web: prevent admin chart grid from overflowing on narrow viewports
1c512de0 server-go: simplify nested coalesce in dashboard session-count filters
2ea8cb10 server-go/web: match dashboard skeleton row count and harden .grid responsive
```

其中后两个是 final reviewer 标记的 Important polish（SQL 嵌套 coalesce 简化、Skeleton 行数对齐 + `.grid` CSS 兜底）。

**环境：**
- 独立 smoke 库：`git_ai_smoke` / 集成测试库：`git_ai_metrics_test`
- 二进制：`/tmp/git-ai-server-smoke`
- 启动参数：`PORT=37337 APP_ENV=development JWT_SECRET=… DB_NAME=git_ai_smoke GIT_AI_API_KEY=smoke-api-key`

**验证项与结果：**

| # | 项 | 结果 |
|---|----|------|
| 1 | `go build ./...` 通过；`go test ./...` 全 PASS（含集成测试在无 DSN 时 SKIP） | ✅ |
| 2 | `pnpm typecheck` + `pnpm build` 通过 | ✅ |
| 3 | 全仓 `grep "attrs_json->>'22'" server-go/internal/` 零命中 | ✅ |
| 4 | 全仓 `grep "coalesce(coalesce" server-go/internal/service/` 零命中（polish A 简化后） | ✅ |
| 5 | 上传 3 个 `event_id=2` 事件（attrs["22"] 全空、session_id 在 attrs["24"]） | ✅ `{"errors":[],"success":true}` |
| 6 | 新 SQL `count(distinct coalesce(session_id, attrs_json->>'24'))` → 返回 `2`（s1, s2） | ✅ |
| 7 | 旧 SQL `count(distinct attrs_json->>'22')` → 返回 `0`（证明 regression 在 base 状态下存在） | ✅ |
| 8 | Agent distribution 按 session_id 聚合：claude-code=1, cursor=1 | ✅ |
| 9 | 集成测试 `TestGetSummaryCountsBySessionId` (METRICS_TEST_DATABASE_URL 设置后) PASS | ✅ |
| 10 | 前端 grep "Prompt" 字面文案（"Prompt 数"/"总 Prompt"/"活跃 Prompt"/"个 Prompt"/"独立 Prompt"）零命中 | ✅ |
| 11 | DashboardSkeleton 三行 grid，与 loaded 状态 3 行 donut+leaderboard 对齐 | ✅ |
| 12 | `.grid` CSS 用 `minmax(min(300px, 100%), 1fr)`，移动端窄屏不溢出 | ✅ |

**SQL 对照样本：**

```sql
-- 新 SQL（simplified after Polish A）
SELECT count(distinct coalesce(session_id, attrs_json->>'24'))
FROM metrics_events
WHERE event_id = 2 AND coalesce(session_id, attrs_json->>'24') <> '';
-- 返回 2

-- 旧 SQL（base 状态，新数据下衰减为 0）
SELECT count(distinct attrs_json->>'22')
FROM metrics_events
WHERE event_id = 2 AND coalesce(attrs_json->>'22', '') <> '';
-- 返回 0
```

**结论：** 7 个 commit 在本地 fresh DB 端到端工作，回归彻底修复，新维度可视化、移动端响应、loading 一致性均达成。Plan 完成。

**已知 follow-up（未在本 plan 范围内）：**
- D1: Dashboard drill-down + 维度过滤面板（独立 PR）
- D2: Leaderboard 分页 / "Load more"
- P2: 按 author/branch/edit_kind 趋势聚合（需后端配套 API）
- Final reviewer minor #3: `.metrics-grid` 用 `minmax(200px, 1fr)`，<200px 仍可能溢出（实际 device 罕见，可后续统一）
- Final reviewer minor #1: `testing.Short()` 与 DSN guard 双层 skip 略冗余，可统一为单 DSN guard
- Final reviewer minor #2: `model.AdminDistributionRow.PromptCount` Go 注释里残留 "distinct prompt_id" 字眼，与新语义不符；待下次顺手修

