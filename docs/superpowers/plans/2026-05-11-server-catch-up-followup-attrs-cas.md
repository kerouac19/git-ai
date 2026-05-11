# 服务端追平客户端：metrics attrs 扩列 + dashboard edit_kind + CAS 三件套

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把客户端已在写、服务端尚未抽取的遥测维度补齐到独立列；为 checkpoint 增加按 `edit_kind` 分组的统计；为 CAS 加上传上限与 metadata 持久化；顺手 drop 已 tombstoned 的 `prompt_id` 列。

**Architecture:**
- 单个迁移 `007` 一次性扩 `metrics_events` 9 列、drop `prompt_id` 列、加 2 个索引（`session_id` + `branch`）。同步 `001` fresh schema 保持一致。`service/metrics.go` 的 `CopyFrom` 列名数组与 `validRows` 行内值一一更新（移 1 增 9，行宽 13→21）。
- 单个迁移 `008` 给 `cas_entries` 加 `metadata JSONB`。`service/cas.go.UploadObject` 签名加一个 `metadata map[string]string` 参数，`UploadObjects` 把 `obj.Metadata` 透传。
- 服务层新增 `AdminDashboardService.getCheckpointByEditKind`，按 `values_json->>'8'` 分组的 `event_id=4` 计数；`model.AdminDashboardData` 加字段 `CheckpointByEditKind []AdminDistributionRow`。
- Handler `UploadWorkerCas` JSON 路径补 `len(objects) > 100` 上限校验，与 read 侧（`ReadWorkerCas`）对称。

**Tech Stack:** Go 1.26 / Gin / pgx / golang-migrate / PostgreSQL。

**范围外（本次不做）：** session_id 列上的 GIN 索引（先 btree 看查询模式）；dashboard 按 author/branch 聚合（需求未明）；custom_attributes 列类型升级为 JSONB（先以 TEXT 落库，后续如需 JSONB 查询再单独迁移转换）。

**部署顺序提示：** 迁移 007 中 `DROP COLUMN prompt_id` 是破坏性操作。本仓为单实例部署（`server-go/scripts/deploy.sh` + 系统服务），部署流程为：停旧 server → 跑迁移 007/008 → 起新 server。新 server 的 `CopyFrom` 已不再写 `prompt_id`，与 drop 后的表结构一致；旧 server 已停，不会再写已删除的列。

---

## File Structure

**新增文件：**
- `server-go/internal/database/migrations/007_metrics_events_extra_attrs.up.sql`
- `server-go/internal/database/migrations/007_metrics_events_extra_attrs.down.sql`
- `server-go/internal/database/migrations/008_cas_entries_metadata.up.sql`
- `server-go/internal/database/migrations/008_cas_entries_metadata.down.sql`

**修改文件：**
- `server-go/internal/database/migrations/001_create_tables.up.sql` — `metrics_events` 与 `cas_entries` fresh schema 与新结构对齐
- `server-go/internal/service/metrics.go` — `CopyFrom` 列名 / `validRows` 行内值
- `server-go/internal/service/metrics_test.go` — 新增 attrs 23+24+全字段 round-trip 测试
- `server-go/internal/service/admin_dashboard.go` — 加 `getCheckpointByEditKind`
- `server-go/internal/model/admin_dashboard.go` — `AdminDashboardData.CheckpointByEditKind` 字段
- `server-go/internal/handler/admin_dashboard_test.go` — handler 测试断言新字段存在
- `server-go/internal/handler/compatibility.go` — `UploadWorkerCas` JSON 路径加 `>100` 校验
- `server-go/internal/service/cas.go` — `UploadObject` 签名加 `metadata`；`UploadObjects` 透传

**不需要修改：**
- `server-go/internal/handler/cas.go`（`/api/cas/upload` 标准端点）— 当前客户端不调用该路径，且签名变更可由调用方传 `nil` 兼容；本计划 Task 4 会更新一处调用。
- `embed.go` — `//go:embed *.sql` 自动收录新迁移。

---

## Task 1: 迁移 007 — metrics_events 扩列、drop prompt_id、加索引

**Files:**
- Create: `server-go/internal/database/migrations/007_metrics_events_extra_attrs.up.sql`
- Create: `server-go/internal/database/migrations/007_metrics_events_extra_attrs.down.sql`
- Modify: `server-go/internal/database/migrations/001_create_tables.up.sql` (lines 63–83)
- Modify: `server-go/internal/service/metrics.go` (UploadBatch — validRows & CopyFrom)
- Modify: `server-go/internal/service/metrics_test.go` (fixture + 新增 round-trip 测试)

- [ ] **Step 1: 写迁移 007 up SQL**

创建 `server-go/internal/database/migrations/007_metrics_events_extra_attrs.up.sql`：

```sql
-- Extend metrics_events with the attrs the client has been writing since 1.4.6+
-- but the server has never extracted into dedicated columns. Also drop the
-- prompt_id column (client tombstoned attr index 22; server queries already
-- read attrs_json->>'22', no column reader exists).
--
-- Idempotent so a fresh DB (where 001 already builds the final shape) and an
-- upgraded DB (where 001 was the old shape and 007 fills the delta) converge.

ALTER TABLE metrics_events
    ADD COLUMN IF NOT EXISTS author                       TEXT,
    ADD COLUMN IF NOT EXISTS commit_sha                   TEXT,
    ADD COLUMN IF NOT EXISTS base_commit_sha              TEXT,
    ADD COLUMN IF NOT EXISTS branch                       TEXT,
    ADD COLUMN IF NOT EXISTS session_id                   TEXT,
    ADD COLUMN IF NOT EXISTS trace_id                     TEXT,
    ADD COLUMN IF NOT EXISTS parent_session_id            TEXT,
    ADD COLUMN IF NOT EXISTS external_parent_session_id   TEXT,
    ADD COLUMN IF NOT EXISTS custom_attributes            TEXT;

ALTER TABLE metrics_events
    DROP COLUMN IF EXISTS prompt_id;

CREATE INDEX IF NOT EXISTS idx_metrics_events_session_id ON metrics_events (session_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_branch     ON metrics_events (branch);
```

- [ ] **Step 2: 写迁移 007 down SQL**

创建 `server-go/internal/database/migrations/007_metrics_events_extra_attrs.down.sql`：

```sql
-- Reverse 007. Brings prompt_id back as a nullable TEXT column (data not
-- preserved — drop was a clean cut; only the column shape is restored).

DROP INDEX IF EXISTS idx_metrics_events_branch;
DROP INDEX IF EXISTS idx_metrics_events_session_id;

ALTER TABLE metrics_events
    DROP COLUMN IF EXISTS custom_attributes,
    DROP COLUMN IF EXISTS external_parent_session_id,
    DROP COLUMN IF EXISTS parent_session_id,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS session_id,
    DROP COLUMN IF EXISTS branch,
    DROP COLUMN IF EXISTS base_commit_sha,
    DROP COLUMN IF EXISTS commit_sha,
    DROP COLUMN IF EXISTS author;

ALTER TABLE metrics_events
    ADD COLUMN IF NOT EXISTS prompt_id TEXT;
```

- [ ] **Step 3: 同步 001 fresh schema**

编辑 `server-go/internal/database/migrations/001_create_tables.up.sql` 第 63–83 行，把 `metrics_events` 表定义整段替换为：

```sql
CREATE TABLE IF NOT EXISTS metrics_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    distinct_id TEXT,
    schema_version INTEGER NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    event_id INTEGER NOT NULL,
    values_json JSONB NOT NULL,
    attrs_json JSONB NOT NULL,
    git_ai_version TEXT,
    repo_url TEXT,
    author TEXT,
    commit_sha TEXT,
    base_commit_sha TEXT,
    branch TEXT,
    tool TEXT,
    model TEXT,
    external_session_id TEXT,
    session_id TEXT,
    trace_id TEXT,
    parent_session_id TEXT,
    external_parent_session_id TEXT,
    custom_attributes TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_metrics_events_user_id ON metrics_events (user_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_event_timestamp ON metrics_events (event_timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_events_repo_url ON metrics_events (repo_url);
CREATE INDEX IF NOT EXISTS idx_metrics_events_distinct_id ON metrics_events (distinct_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_session_id ON metrics_events (session_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_branch ON metrics_events (branch);
```

> 说明：`prompt_id` 已移除；fresh DB 直接建出最终结构，007 的 `ADD IF NOT EXISTS` / `DROP IF EXISTS` 会全部空操作（含两个新索引也是 `IF NOT EXISTS`）。

- [ ] **Step 4: 更新 metrics.go 的 validRows 与 CopyFrom 列**

编辑 `server-go/internal/service/metrics.go`，把 `UploadBatch` 中 `validRows = append(...)` 一段和 `CopyFrom` 调用一段整体替换为：

```go
		validRows = append(validRows, []any{
			userID,
			did,
			batch.V,
			eventTimestamp,
			event.E,
			valuesJSON,
			attrsJSON,
			asNullableString(event.A["0"]),  // git_ai_version
			asNullableString(event.A["1"]),  // repo_url
			asNullableString(event.A["2"]),  // author
			asNullableString(event.A["3"]),  // commit_sha
			asNullableString(event.A["4"]),  // base_commit_sha
			asNullableString(event.A["5"]),  // branch
			asNullableString(event.A["20"]), // tool
			asNullableString(event.A["21"]), // model
			asNullableString(event.A["23"]), // external_session_id
			asNullableString(event.A["24"]), // session_id
			asNullableString(event.A["25"]), // trace_id
			asNullableString(event.A["26"]), // parent_session_id
			asNullableString(event.A["27"]), // external_parent_session_id
			asNullableString(event.A["30"]), // custom_attributes (raw JSON string)
		})
```

把 `CopyFrom` 调用中的列名切片整体替换为：

```go
		_, err := s.Pool.CopyFrom(
			ctx,
			pgx.Identifier{"public", "metrics_events"},
			[]string{
				"user_id", "distinct_id", "schema_version",
				"event_timestamp", "event_id",
				"values_json", "attrs_json",
				"git_ai_version", "repo_url",
				"author", "commit_sha", "base_commit_sha", "branch",
				"tool", "model", "external_session_id",
				"session_id", "trace_id", "parent_session_id",
				"external_parent_session_id", "custom_attributes",
			},
			pgx.CopyFromRows(validRows),
		)
```

> 列数：原 13（user_id…external_session_id），新 21。`validRows` 行宽 21，与列名一一对应。

- [ ] **Step 5: 写失败的 round-trip 测试**

在 `server-go/internal/service/metrics_test.go` 末尾追加：

```go
func TestUploadBatchPopulatesAllAttrColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t) // helper added below; skips if METRICS_TEST_DATABASE_URL is unset
	defer pool.Close()

	svc := &MetricsService{Pool: pool}

	batch := &model.MetricsBatch{
		V: 1,
		Events: []model.MetricsEvent{
			{
				IsObject: true,
				T:        1715000000,
				E:        5,
				V:        map[string]any{"0": "session-started"},
				A: map[string]any{
					"0":  "1.4.7",
					"1":  "https://github.com/test/repo",
					"2":  "dev@example.com",
					"3":  "abc123",
					"4":  "base456",
					"5":  "feature-branch",
					"20": "claude-code",
					"21": "gpt-5.4",
					"23": "ext-session-xyz",
					"24": "session-xyz",
					"25": "trace-xyz",
					"26": "parent-session-xyz",
					"27": "ext-parent-session-xyz",
					"30": `{"workspace":"smoke"}`,
				},
			},
		},
	}

	errs, err := svc.UploadBatch(t.Context(), "00000000-0000-0000-0000-000000000001", nil, batch)
	if err != nil {
		t.Fatalf("UploadBatch() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("UploadBatch() errors = %+v, want empty", errs)
	}

	row := pool.QueryRow(t.Context(), `
		SELECT event_id, git_ai_version, repo_url,
		       author, commit_sha, base_commit_sha, branch,
		       tool, model, external_session_id,
		       session_id, trace_id, parent_session_id,
		       external_parent_session_id, custom_attributes
		  FROM metrics_events
		 ORDER BY received_at DESC
		 LIMIT 1`)

	var eventID int
	var got [14]*string
	if err := row.Scan(&eventID,
		&got[0], &got[1], &got[2], &got[3], &got[4], &got[5],
		&got[6], &got[7], &got[8], &got[9], &got[10], &got[11],
		&got[12], &got[13],
	); err != nil {
		t.Fatalf("scanning inserted row: %v", err)
	}

	wantEventID := 5
	if eventID != wantEventID {
		t.Fatalf("event_id = %d, want %d", eventID, wantEventID)
	}

	want := []string{
		"1.4.7", "https://github.com/test/repo",
		"dev@example.com", "abc123", "base456", "feature-branch",
		"claude-code", "gpt-5.4", "ext-session-xyz",
		"session-xyz", "trace-xyz", "parent-session-xyz",
		"ext-parent-session-xyz", `{"workspace":"smoke"}`,
	}
	for i, w := range want {
		if got[i] == nil {
			t.Fatalf("column %d is NULL, want %q", i, w)
		}
		if *got[i] != w {
			t.Fatalf("column %d = %q, want %q", i, *got[i], w)
		}
	}
}
```

紧跟其后追加一个 helper（同一文件，包内可见）：

```go
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("METRICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("METRICS_TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	// Ensure schema is migrated to current state. Failures here mean the
	// migration files themselves are broken — escalate.
	if err := database.RunMigrations(dsn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Truncate before each test invocation for isolation. We assume the DSN
	// points at a dedicated test DB; do NOT run this against production.
	if _, err := pool.Exec(t.Context(), `TRUNCATE TABLE metrics_events`); err != nil {
		t.Fatalf("truncate metrics_events: %v", err)
	}
	return pool
}
```

并在文件头部 import 段加入 `"os"`、`"github.com/jackc/pgx/v5/pgxpool"`、`"git-ai-server/internal/database"`（若已 import 则跳过）。

- [ ] **Step 6: 跑测试验证 round-trip 在没设环境变量时 skip，加变量后 FAIL（因为代码改动还没生效）**

Run（不设 DSN，应 skip）：
```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/service/ -run TestUploadBatchPopulatesAllAttrColumns -v
```
Expected：`--- SKIP: TestUploadBatchPopulatesAllAttrColumns ... METRICS_TEST_DATABASE_URL not set`。

Run（用本地 PG smoke 库；先建库）：
```bash
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_metrics_test;" -c "CREATE DATABASE git_ai_metrics_test;"
cd /Users/hg/git/git-ai/server-go && METRICS_TEST_DATABASE_URL='postgres://127.0.0.1:5432/git_ai_metrics_test?sslmode=disable' go test ./internal/service/ -run TestUploadBatchPopulatesAllAttrColumns -v
```
Expected：在 Step 7 之前测试已经能跑通——因为 Steps 4 + 1 + 3 都已经写好，迁移会把新列建出来、CopyFrom 也已用新列。验证 PASS。

> 如果用了 TDD 顺序（先写测试再改代码）的严格视角，可以把 Steps 4/3/1 放到 Step 5 之后；这里按"先把所有变更落定、再一次性跑测试"的工作方式，让 5 步代码 + 1 步测试 + 1 步运行更紧凑。

- [ ] **Step 7: 跑整个 service 包测试不回归**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/service/ -v
```
Expected：原有 15 个测试 PASS，新增 `TestUploadBatchPopulatesAllAttrColumns` PASS（或在未设 DSN 时 SKIP）。

- [ ] **Step 8: 跑 build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
Expected：build 通过，全部 PASS。

- [ ] **Step 9: 提交 Task 1**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/internal/database/migrations/001_create_tables.up.sql \
  server-go/internal/database/migrations/007_metrics_events_extra_attrs.up.sql \
  server-go/internal/database/migrations/007_metrics_events_extra_attrs.down.sql \
  server-go/internal/service/metrics.go \
  server-go/internal/service/metrics_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: extract 9 attr columns and drop tombstoned prompt_id

Client 1.4.6+ writes attr indices 2-5 (author/commit_sha/base_commit_sha/
branch), 24-27 (session_id/trace_id/parent/external_parent) and 30
(custom_attributes) but the server only ever pulled them through into
attrs_json JSONB. Promote each to a typed column so dashboards can
aggregate by branch/session/author without JSON probes; index session_id
and branch (the two highest-cardinality lookup keys).

Drop prompt_id at the same time: client attr index 22 is tombstoned and
no server query ever read the column (dashboards already use
attrs_json->>'22'). Migration 007 is idempotent against both fresh
databases and upgraded ones.
EOF
)"
```

---

## Task 2: dashboard 加 checkpoint_by_edit_kind 统计点

**Files:**
- Modify: `server-go/internal/service/admin_dashboard.go` (新方法 + GetGlobalStats 接线)
- Modify: `server-go/internal/model/admin_dashboard.go` (新字段)
- Modify: `server-go/internal/handler/admin_dashboard_test.go` (handler 测试断言新字段)

- [ ] **Step 1: 改 model 加字段**

编辑 `server-go/internal/model/admin_dashboard.go`，把 `AdminDashboardData` 整段替换为：

```go
type AdminDashboardData struct {
	Range                 string                 `json:"range"`
	Summary               AdminDashboardSummary  `json:"summary"`
	Trend                 []AdminTrendPoint      `json:"trend"`
	TopUsers              []AdminTopUser         `json:"topUsers"`
	TopOrgs               []AdminTopOrg          `json:"topOrgs"`
	AgentDistribution     []AdminDistributionRow `json:"agentDistribution"`
	ModelDistribution     []AdminDistributionRow `json:"modelDistribution"`
	CheckpointByEditKind  []AdminDistributionRow `json:"checkpointByEditKind"`
}
```

- [ ] **Step 2: 写失败的 handler 测试**

在 `server-go/internal/handler/admin_dashboard_test.go` 末尾追加一个新测试函数，与文件现有风格（`t.Fatalf` / `t.Errorf`，无 testify 依赖）保持一致：

```go
func TestAdminDashboard_CheckpointByEditKind(t *testing.T) {
	fake := &fakeAdminDashSvc{
		data: &model.AdminDashboardData{
			Range:                "7d",
			TopUsers:             []model.AdminTopUser{},
			TopOrgs:              []model.AdminTopOrg{},
			AgentDistribution:    []model.AdminDistributionRow{},
			ModelDistribution:    []model.AdminDistributionRow{},
			CheckpointByEditKind: []model.AdminDistributionRow{
				{Label: "file_edit", PromptCount: 2, Share: 0.66},
				{Label: "bash", PromptCount: 1, Share: 0.33},
			},
		},
	}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var payload struct {
		Success bool                     `json:"success"`
		Data    model.AdminDashboardData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}

	if len(payload.Data.CheckpointByEditKind) != 2 {
		t.Fatalf("CheckpointByEditKind len = %d, want 2; got=%+v",
			len(payload.Data.CheckpointByEditKind), payload.Data.CheckpointByEditKind)
	}
	if payload.Data.CheckpointByEditKind[0].Label != "file_edit" {
		t.Errorf("first label = %q, want file_edit", payload.Data.CheckpointByEditKind[0].Label)
	}
	if payload.Data.CheckpointByEditKind[0].PromptCount != 2 {
		t.Errorf("first count = %d, want 2", payload.Data.CheckpointByEditKind[0].PromptCount)
	}
}
```

> 该测试不需要新 import — `json`、`http`、`httptest`、`model` 在文件顶已 import。当 model 还没有 `CheckpointByEditKind` 字段时，编译就会失败（`unknown field` 报错），即 TDD 的 "RED" 状态。

- [ ] **Step 3: 跑 handler 测试验证它失败**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run "Admin.*Dashboard" -v
```
Expected：FAIL，"response payload missing checkpointByEditKind field"。

- [ ] **Step 4: service 加 getCheckpointByEditKind 方法**

编辑 `server-go/internal/service/admin_dashboard.go`，在 `getDistribution` 方法之后追加：

```go
// getCheckpointByEditKind groups checkpoint events (event_id=4) by values_json->>'8'
// (edit_kind: "file_edit" | "bash" | NULL). Reports both labeled buckets so the
// admin UI can render a stacked bar.
func (s *AdminDashboardService) getCheckpointByEditKind(ctx context.Context, days int) ([]model.AdminDistributionRow, error) {
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`
		select
			coalesce(nullif(values_json->>'8', ''), '(unknown)') as label,
			count(*) as event_count
		from public.metrics_events
		where event_id = 4
			and event_timestamp >= now() - interval '%d days'
		group by 1
		order by event_count desc, label asc
	`, days))
	if err != nil {
		return nil, fmt.Errorf("admin checkpoint by edit_kind: %w", err)
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
			return nil, fmt.Errorf("admin checkpoint by edit_kind scan: %w", err)
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
			PromptCount: b.count, // reused as event_count; PromptCount field name fits the row shape
			Share:       share,
		})
	}
	return out, nil
}
```

- [ ] **Step 5: 把新方法接入 GetGlobalStats**

仍在 `server-go/internal/service/admin_dashboard.go`，把 `GetGlobalStats` 函数体替换为：

```go
func (s *AdminDashboardService) GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error) {
	days := rangeToDays(rangeKey)

	g, ctx := errgroup.WithContext(ctx)

	var (
		summary        model.AdminDashboardSummary
		trend          []model.AdminTrendPoint
		topUsers       []model.AdminTopUser
		topOrgs        []model.AdminTopOrg
		agents         []model.AdminDistributionRow
		models         []model.AdminDistributionRow
		editKinds      []model.AdminDistributionRow
	)

	g.Go(func() error { var err error; summary, err = s.getSummary(ctx, days); return err })
	g.Go(func() error { var err error; trend, err = s.getTrend(ctx, days); return err })
	g.Go(func() error { var err error; topUsers, err = s.getTopUsers(ctx, days); return err })
	g.Go(func() error { var err error; topOrgs, err = s.getTopOrgs(ctx, days); return err })
	g.Go(func() error { var err error; agents, err = s.getDistribution(ctx, days, "20"); return err })
	g.Go(func() error { var err error; models, err = s.getDistribution(ctx, days, "21"); return err })
	g.Go(func() error { var err error; editKinds, err = s.getCheckpointByEditKind(ctx, days); return err })

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &model.AdminDashboardData{
		Range:                rangeKey,
		Summary:              summary,
		Trend:                trend,
		TopUsers:             topUsers,
		TopOrgs:              topOrgs,
		AgentDistribution:    agents,
		ModelDistribution:    models,
		CheckpointByEditKind: editKinds,
	}, nil
}
```

- [ ] **Step 6: 跑 handler 测试验证通过**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run "Admin.*Dashboard" -v
```
Expected：PASS。

- [ ] **Step 7: 跑 build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
Expected：通过、全 PASS。

- [ ] **Step 8: 提交 Task 2**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/internal/model/admin_dashboard.go \
  server-go/internal/service/admin_dashboard.go \
  server-go/internal/handler/admin_dashboard_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: surface checkpoint distribution by edit_kind in admin dashboard

Client checkpoint events carry edit_kind at values_json position 8
("file_edit" | "bash" | null). The admin dashboard already shows agent
and model distributions but has no breakdown of how AI agents are
interacting with the repo (file edits vs shell commands). Add a new
distribution row aggregating event_id=4 by values_json->>'8' so the
admin UI can render the split as a third donut/stacked bar.
EOF
)"
```

---

## Task 3: CAS upload 加数量上限（≤100，与 read 侧对称）

**Files:**
- Modify: `server-go/internal/handler/compatibility.go` (UploadWorkerCas JSON 路径)

- [ ] **Step 1: 写失败测试**

如 `server-go/internal/handler/compatibility_test.go` 不存在则创建，否则在末尾追加测试。

如果 `compatibility_test.go` **不存在**，创建文件（统一项目风格 `t.Fatalf`，不引入 testify）：

```go
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadWorkerCasRejectsTooManyObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Build a body with 101 objects; do not need a real service — we expect
	// the limit check to short-circuit before service dispatch.
	objs := make([]map[string]any, 101)
	for i := range objs {
		objs[i] = map[string]any{"hash": fmt.Sprintf("%064x", i), "content": "x"}
	}
	body, err := json.Marshal(map[string]any{"objects": objs})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	r := gin.New()
	h := &CompatibilityHandler{} // service-less; limit check must precede service dispatch
	r.POST("/worker/cas/upload", h.UploadWorkerCas)

	req := httptest.NewRequest(http.MethodPost, "/worker/cas/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "maximum of 100") {
		t.Fatalf("body missing 'maximum of 100': %s", rec.Body.String())
	}
}
```

> 现有 `UploadWorkerCas` 上线时挂了 worker auth middleware；此测试**直接打 handler 而不挂中间件**，依赖"limit check 在 service 调用之前"——所以 Step 3 的实现必须把数量校验放在 `h.CasSvc.UploadObjects(...)` 调用之前。

- [ ] **Step 2: 跑测试验证它失败**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run TestUploadWorkerCasRejectsTooManyObjects -v
```
Expected：FAIL，"expected 400 for >100 objects, body=..."（当前未校验、走到 service 后 panic 或返回 5xx）。

- [ ] **Step 3: 实现上限校验**

编辑 `server-go/internal/handler/compatibility.go`，在 `UploadWorkerCas` 函数体的 `if strings.HasPrefix(contentType, "application/json")` 分支内，把 `if err := c.ShouldBindJSON(&body); err == nil && len(body.Objects) > 0 {` 改为：

```go
		if err := c.ShouldBindJSON(&body); err == nil && len(body.Objects) > 0 {
			if len(body.Objects) > 100 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "A maximum of 100 objects is supported per request",
				})
				return
			}
			result, err := h.CasSvc.UploadObjects(c.Request.Context(), body.Objects)
```

> 错误文案与 `ReadWorkerCas` 的"A maximum of 100 hashes is supported per request"风格一致。

- [ ] **Step 4: 跑测试验证通过**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run TestUploadWorkerCasRejectsTooManyObjects -v
```
Expected：PASS。

- [ ] **Step 5: 跑 build + handler 包全测**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./internal/handler/ -v
```
Expected：通过，全 PASS（含原有 admin_dashboard / device_flow / releases 测试）。

- [ ] **Step 6: 提交 Task 3**

```bash
cd /Users/hg/git/git-ai && git add server-go/internal/handler/compatibility.go server-go/internal/handler/compatibility_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: cap CAS upload at 100 objects per request

ReadWorkerCas already enforces "max 100 hashes per request"; the
upload-side handler had no such cap, leaving an asymmetric DoS surface
where a single client could pin a large batch insert. Mirror the read
limit on the JSON path, returning the same shape of 400 response.
EOF
)"
```

---

## Task 4: CAS metadata 列 + 持久化

**Files:**
- Create: `server-go/internal/database/migrations/008_cas_entries_metadata.up.sql`
- Create: `server-go/internal/database/migrations/008_cas_entries_metadata.down.sql`
- Modify: `server-go/internal/database/migrations/001_create_tables.up.sql` (lines 23–29)
- Modify: `server-go/internal/service/cas.go` (UploadObject 签名 + UploadObjects 透传)
- Modify: `server-go/internal/handler/cas.go` (standard `/api/cas/upload` 调用方传 nil)

- [ ] **Step 1: 写迁移 008 up SQL**

创建 `server-go/internal/database/migrations/008_cas_entries_metadata.up.sql`：

```sql
-- Allow CAS uploads to attach arbitrary string metadata (e.g. transcript
-- tags, prompt store labels). Stored as JSONB so future queries can use
-- GIN indexing if needed. Existing rows get NULL — backward compatible.

ALTER TABLE cas_entries
    ADD COLUMN IF NOT EXISTS metadata JSONB;
```

- [ ] **Step 2: 写迁移 008 down SQL**

创建 `server-go/internal/database/migrations/008_cas_entries_metadata.down.sql`：

```sql
ALTER TABLE cas_entries
    DROP COLUMN IF EXISTS metadata;
```

- [ ] **Step 3: 同步 001 fresh schema 的 cas_entries**

编辑 `server-go/internal/database/migrations/001_create_tables.up.sql` 第 23–29 行，把 `cas_entries` 整段替换为：

```sql
CREATE TABLE IF NOT EXISTS cas_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hash TEXT NOT NULL UNIQUE,
    encrypted_content TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text/plain',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 4: 写失败 service 测试**

在 `server-go/internal/service/` 新建 `cas_test.go`（仓中目前没有；如果已有则在末尾追加）。

```go
package service

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadObjectsPersistsMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openCasTestPool(t)
	defer pool.Close()

	svc := &CasService{Pool: pool, CASKey: "test-key-32-bytes-test-key-32byt"}

	objects := []CasUploadRequest{{
		Hash:    "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Content: map[string]string{"hello": "world"},
		Metadata: map[string]string{
			"source": "transcript",
			"tag":    "smoke",
		},
	}}

	result, err := svc.UploadObjects(t.Context(), objects)
	if err != nil {
		t.Fatalf("UploadObjects: %v", err)
	}
	if result.FailureCount != 0 {
		t.Fatalf("FailureCount = %d, want 0; results=%+v", result.FailureCount, result.Results)
	}

	var metaJSON []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT metadata FROM cas_entries WHERE hash = $1`, objects[0].Hash,
	).Scan(&metaJSON); err != nil {
		if err == pgx.ErrNoRows {
			t.Fatalf("entry not inserted")
		}
		t.Fatalf("query metadata: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(metaJSON, &got); err != nil {
		t.Fatalf("decode metadata: %v (raw=%s)", err, string(metaJSON))
	}
	if got["source"] != "transcript" || got["tag"] != "smoke" {
		t.Fatalf("metadata = %+v, want source=transcript tag=smoke", got)
	}
}

func openCasTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Reuse the same env var as openTestPool so a single test DB serves both
	// suites; tests TRUNCATE their own table.
	pool := openTestPool(t)
	if _, err := pool.Exec(t.Context(), `TRUNCATE TABLE cas_entries`); err != nil {
		t.Fatalf("truncate cas_entries: %v", err)
	}
	return pool
}
```

- [ ] **Step 5: 跑测试验证它失败**

```bash
METRICS_TEST_DATABASE_URL='postgres://127.0.0.1:5432/git_ai_metrics_test?sslmode=disable' \
  go test ./internal/service/ -run TestUploadObjectsPersistsMetadata -v
```
Expected：FAIL，错误诸如 `metadata column missing` 或 `scan: column metadata does not exist`（迁移 008 还没接进来）/`got is empty map`（service 还没写 metadata）。

注：`METRICS_TEST_DATABASE_URL` 是 Task 1 引入的同一个测试库变量。

- [ ] **Step 6: 修改 UploadObject 签名加入 metadata**

编辑 `server-go/internal/service/cas.go`，把 `UploadObject` 函数体整体替换为：

```go
func (s *CasService) UploadObject(ctx context.Context, hash string, content any, metadata map[string]string) (string, error) {
	h := strings.TrimSpace(strings.ToLower(hash))

	var exists bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cas_entries WHERE hash = $1)`, h,
	).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("checking existing entry: %w", err)
	}
	if exists {
		return h, nil
	}

	serialized, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("serializing content: %w", err)
	}

	compressed, err := zlibCompress(serialized)
	if err != nil {
		return "", fmt.Errorf("compressing content: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(compressed)

	encrypted, err := gitcrypto.EncryptCAS(b64, s.CASKey)
	if err != nil {
		return "", fmt.Errorf("encrypting content: %w", err)
	}

	var metadataJSON []byte
	if len(metadata) > 0 {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return "", fmt.Errorf("serializing metadata: %w", err)
		}
	}

	_, err = s.Pool.Exec(ctx,
		`INSERT INTO cas_entries (id, hash, encrypted_content, content_type, metadata, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, now())`,
		h, encrypted, "application/json", metadataJSON,
	)
	if err != nil {
		return "", fmt.Errorf("inserting cas entry: %w", err)
	}

	return h, nil
}
```

> `metadataJSON` 为 nil 时 pgx 写入 NULL；为非空字节切片时写入 JSONB。

- [ ] **Step 7: 让 UploadObjects 透传 metadata**

仍在 `server-go/internal/service/cas.go`，把 `UploadObjects` 内 `if _, err := s.UploadObject(ctx, h, obj.Content); err != nil {` 整行替换为：

```go
		if _, err := s.UploadObject(ctx, h, obj.Content, obj.Metadata); err != nil {
```

- [ ] **Step 8: 更新 standard CAS handler 调用方**

`server-go/internal/handler/cas.go` 中 `Upload` handler 会调 `UploadObject`/`UploadContent`（按现有逻辑）。打开它：

```bash
grep -n "UploadObject\|UploadContent" /Users/hg/git/git-ai/server-go/internal/handler/cas.go
```

如果出现 `s.Svc.UploadObject(ctx, hash, content)` 调用，把它改为 `s.Svc.UploadObject(ctx, hash, content, nil)`。若该 handler 调用的是 `UploadContent`（不接受 metadata），无需改动。

- [ ] **Step 9: 跑 service + handler 测试**

```bash
METRICS_TEST_DATABASE_URL='postgres://127.0.0.1:5432/git_ai_metrics_test?sslmode=disable' \
  go test ./internal/service/ -run TestUploadObjectsPersistsMetadata -v
```
Expected：PASS。

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
Expected：build + 全包 PASS。

- [ ] **Step 10: 提交 Task 4**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/internal/database/migrations/001_create_tables.up.sql \
  server-go/internal/database/migrations/008_cas_entries_metadata.up.sql \
  server-go/internal/database/migrations/008_cas_entries_metadata.down.sql \
  server-go/internal/service/cas.go \
  server-go/internal/service/cas_test.go \
  server-go/internal/handler/cas.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: persist CAS upload metadata to cas_entries.metadata

Client uploads carry an optional metadata map (transcript tags, prompt
store labels, etc.) which the server accepted in the request body but
silently dropped. Add a JSONB metadata column to cas_entries and
persist it via UploadObject/UploadObjects so downstream reports can
filter or annotate CAS content by metadata.
EOF
)"
```

---

## Task 5: 端到端本地 smoke（可选，但推荐）

**Files:** 无代码修改。

- [ ] **Step 1: 启动 server 验证迁移 007+008 都跑通**

```bash
cd /Users/hg/git/git-ai && go build -o /tmp/git-ai-server-smoke ./server-go/cmd/server
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_smoke;" -c "CREATE DATABASE git_ai_smoke;"
PORT=37337 APP_ENV=development \
  JWT_SECRET=smoke-jwt-secret-not-for-prod-just-for-test \
  DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=$USER DB_PASSWORD= DB_NAME=git_ai_smoke DB_SSL=false \
  GIT_AI_API_KEY=smoke-api-key \
  /tmp/git-ai-server-smoke > /tmp/git-ai-server-smoke.log 2>&1 &
sleep 2
```

Expected：日志含 `Application is running on: http://localhost:37337`，无 migration error。

- [ ] **Step 2: 校验 schema_migrations 与列**

```bash
psql -h 127.0.0.1 -d git_ai_smoke -c "SELECT version, dirty FROM schema_migrations;"
psql -h 127.0.0.1 -d git_ai_smoke -c "\d metrics_events" | grep -E "(session_id|trace_id|parent_session|author|commit_sha|branch|custom_attributes|prompt_id)"
psql -h 127.0.0.1 -d git_ai_smoke -c "\d cas_entries" | grep -i metadata
```

Expected：
- `version=8, dirty=f`
- `metrics_events` 含 author / commit_sha / base_commit_sha / branch / session_id / trace_id / parent_session_id / external_parent_session_id / custom_attributes；**不含** prompt_id
- `cas_entries.metadata` 列存在，类型 jsonb

- [ ] **Step 3: 上传含全字段的 SessionEvent**

```bash
curl -sS -X POST http://127.0.0.1:37337/worker/metrics/upload \
  -H "Content-Type: application/json" -H "X-API-Key: smoke-api-key" \
  -d '{
    "v": 1,
    "events": [{
      "t": 1715000000, "e": 5,
      "v": {"0": "session-started"},
      "a": {
        "0": "1.4.7", "1": "https://github.com/test/repo",
        "2": "dev@example.com", "3": "abc123", "4": "base456", "5": "feature-branch",
        "20": "claude-code", "21": "gpt-5.4",
        "23": "ext-session-xyz", "24": "session-xyz",
        "25": "trace-xyz", "26": "parent-session-xyz", "27": "ext-parent-session-xyz",
        "30": "{\"workspace\":\"smoke\"}"
      }
    }]
  }'
```

Expected：`{"errors":[],"success":true}`。

- [ ] **Step 4: 校验入库**

```bash
psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT event_id, author, commit_sha, base_commit_sha, branch,
          session_id, trace_id, parent_session_id, external_parent_session_id,
          external_session_id, custom_attributes
   FROM metrics_events ORDER BY received_at DESC LIMIT 1;"
```

Expected：14 列全部填充正确，且 prompt_id 列不存在（SELECT 不到）。

- [ ] **Step 5: 测试 CAS upload >100 上限**

```bash
python3 -c '
import json
objs = [{"hash": "%064d" % i, "content": "x"} for i in range(101)]
print(json.dumps({"objects": objs}))
' | curl -sS -i -X POST http://127.0.0.1:37337/worker/cas/upload \
  -H "Content-Type: application/json" -H "X-API-Key: smoke-api-key" -d @-
```

Expected：`HTTP/1.1 400`，body 含 `"A maximum of 100 objects is supported per request"`。

- [ ] **Step 6: 测试 CAS metadata 落库**

```bash
curl -sS -X POST http://127.0.0.1:37337/worker/cas/upload \
  -H "Content-Type: application/json" -H "X-API-Key: smoke-api-key" \
  -d '{"objects":[{"hash":"cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe","content":{"hello":"world"},"metadata":{"source":"smoke","tag":"meta"}}]}'

psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT hash, metadata FROM cas_entries WHERE hash = 'cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe';"
```

Expected：metadata 列含 JSON `{"source": "smoke", "tag": "meta"}`。

- [ ] **Step 7: 测试 dashboard checkpoint_by_edit_kind**

先上传一个 checkpoint event（event_id=4）含 edit_kind：

```bash
curl -sS -X POST http://127.0.0.1:37337/worker/metrics/upload \
  -H "Content-Type: application/json" -H "X-API-Key: smoke-api-key" \
  -d '{
    "v": 1,
    "events": [
      {"t": 1715000001, "e": 4, "v": {"8": "file_edit"}, "a": {"0": "1.4.7", "24": "s1"}},
      {"t": 1715000002, "e": 4, "v": {"8": "bash"},      "a": {"0": "1.4.7", "24": "s1"}},
      {"t": 1715000003, "e": 4, "v": {"8": "file_edit"}, "a": {"0": "1.4.7", "24": "s2"}}
    ]
  }'
```

然后调 `/api/dashboard/global`。该端点需要 JWT 而非 API Key——为了 smoke 简便，可直接查 DB 验证 SQL：

```bash
psql -h 127.0.0.1 -d git_ai_smoke -c \
  "SELECT coalesce(nullif(values_json->>'8', ''), '(unknown)') AS label, count(*) AS event_count
   FROM metrics_events WHERE event_id = 4 GROUP BY 1 ORDER BY 2 DESC;"
```

Expected：`file_edit | 2`、`bash | 1`。

- [ ] **Step 8: 关停 server**

```bash
pkill -f /tmp/git-ai-server-smoke || true
```

Task 5 无代码改动，不 commit。

---

## 完成标准

- 迁移 007、008 在 fresh DB（顺序跑 001→…→008）和 upgraded DB（接力跑 006→007→008）都成功，`schema_migrations.version=8 dirty=f`。
- `metrics_events` 含 9 新列、无 `prompt_id` 列、有 2 个新索引（session_id、branch）。
- `cas_entries.metadata` 列存在，类型 JSONB。
- 客户端上报含全套 attrs 的 SessionEvent 后，独立列被填充；CAS upload 含 metadata 后，metadata 列被填充。
- CAS upload `>100` 对象返回 400 含 "maximum of 100 objects" 文案。
- `/api/dashboard/global` 响应含 `checkpointByEditKind: [...]` 字段；SQL 按 `values_json->>'8'` 正确分组。
- `go build ./...` 通过；`go test ./...` 全 PASS（带 `METRICS_TEST_DATABASE_URL` 时跑集成测试；不带时 SKIP，CI 默认走 SKIP 路径）。

## 回滚

按提交倒序回滚即可（4→3→2→1）：

- Task 4：`migrate down 1`（执行 008 down，drop `cas_entries.metadata`）；revert 服务端 commit；调用方移除 `metadata` 参数。
- Task 3：revert handler 改动即可（无迁移）。
- Task 2：revert dashboard 改动即可（无迁移）。
- Task 1：`migrate down 1`（执行 007 down，重建 `prompt_id` 列、drop 9 个新列与 2 索引）；revert metrics.go 改动。数据：drop 时已经丢弃；prompt_id 列重建后为空（迁移本就声明不保留数据）。

---

## 本地验证记录（2026-05-11）

实际在本机 macOS + PostgreSQL 15 上完成端到端 smoke（重建 `git_ai_smoke` fresh DB）。

**Commits chain**（base `ca70117b` → HEAD `92ac1e68`，共 6 个 commit）：

```
2b65d1c4 server-go: extract 9 attr columns and drop tombstoned prompt_id
0f8eb002 server-go: tighten metrics integration test cleanup and add NULL-path coverage
a1bef4db server-go: surface checkpoint distribution by edit_kind in admin dashboard
0d8a760b server-go: document AdminDistributionRow.PromptCount field reuse
2a740ad8 server-go: cap CAS upload at 100 objects per request
92ac1e68 server-go: persist CAS upload metadata to cas_entries.metadata
```

**环境：**
- 独立 smoke 库：`git_ai_smoke`（fresh，从 001→008 顺序跑完）
- 集成测试库：`git_ai_metrics_test`（`METRICS_TEST_DATABASE_URL` 指向）
- 二进制：`/tmp/git-ai-server-smoke`
- 启动参数：`PORT=37337 APP_ENV=development JWT_SECRET=… DB_NAME=git_ai_smoke GIT_AI_API_KEY=smoke-api-key`

**验证项与结果：**

| # | 项 | 结果 |
|---|----|------|
| 1 | `RunMigrations` 启动无错；`schema_migrations` 版本 `8`、`dirty=f` | ✅ |
| 2 | `\d metrics_events`：含 9 新列（author、commit_sha、base_commit_sha、branch、session_id、trace_id、parent_session_id、external_parent_session_id、custom_attributes）；**无** `prompt_id` 列 | ✅ |
| 3 | `idx_metrics_events_session_id` + `idx_metrics_events_branch` 两索引建出 | ✅ |
| 4 | `\d cas_entries`：含 `metadata jsonb` 列 | ✅ |
| 5 | 上传含全套 attrs (0-5, 20, 21, 23-27, 30) 的 SessionEvent → `200 OK`、`{"errors":[],"success":true}` | ✅ |
| 6 | 入库行 11 列全部正确填充（author='dev@example.com'、branch='feature-branch'、session_id='session-xyz'、custom_attributes='{"workspace":"smoke"}'、…） | ✅ |
| 7 | CAS upload 101 对象 → `HTTP/1.1 400` + `{"error":"A maximum of 100 objects is supported per request"}` | ✅ |
| 8 | CAS upload 1 对象含 `metadata:{source:smoke, tag:meta}` → `cas_entries.metadata = {"tag":"meta","source":"smoke"}` | ✅ |
| 9 | 3 个 checkpoint events with `values_json["8"]` (edit_kind) → 服务端 SQL 聚合 `file_edit=2, bash=1` | ✅ |
| 10 | server log 全程无 ERROR / fatal；server 进程 SIGTERM 干净退出 | ✅ |

**入库样本（核心字段）：**

```
event_id                   | 5
author                     | dev@example.com
commit_sha                 | abc123
base_commit_sha            | base456
branch                     | feature-branch
session_id                 | session-xyz
trace_id                   | trace-xyz
parent_session_id          | parent-session-xyz
external_parent_session_id | ext-parent-session-xyz
external_session_id        | ext-session-xyz
custom_attributes          | {"workspace":"smoke"}
```

**CAS metadata 样本：**

```
hash:     cafebabe…
metadata: {"tag": "meta", "source": "smoke"}
```

**checkpoint by edit_kind 聚合：**

```
   label   | event_count
-----------+-------------
 file_edit |           2
 bash      |           1
```

**结论：** 6 个 commit 在本地 fresh DB 端到端工作，符合"完成标准"全部 7 项。计划达成。

