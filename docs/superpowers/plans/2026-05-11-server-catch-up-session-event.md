# 服务端追平客户端（SessionEvent + 索引 23 列名）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `server-go` 支持客户端 1.4.6+ 上报的 `SessionEvent` (event_id=5)，并把 metrics 表中错位的 `external_prompt_id` 列重命名为 `external_session_id`，与客户端 `attr_pos::EXTERNAL_SESSION_ID = 23` 语义对齐。

**Architecture:**
- 在 `service/metrics.go` 的 `supportedEventIDs` 白名单加入 `5`，让 `validateEvent` 通过 SessionEvent。
- 新增 PG 迁移 `006_rename_external_prompt_id_to_external_session_id.{up,down}.sql`，用 `ALTER TABLE … RENAME COLUMN` 一句改名。迁移由 `golang-migrate/iofs` 在 server 启动时自动应用（`internal/database/migrate.go`）。
- 同步更新 `service/metrics.go` 的 `CopyFrom` 列名字符串及现有 `metrics_test.go` 中的 fixture 注释/字面值。

**Tech Stack:** Go 1.26 / Gin / pgx / golang-migrate (iofs 嵌入式) / PostgreSQL。

**范围外（本次不做，留作后续 plan）:** 补 `session_id (24)` / `trace_id (25)` / `parent_session_id (26)` / `external_parent_session_id (27)` / `author (2)` / `commit_sha (3)` / `base_commit_sha (4)` / `branch (5)` / `custom_attributes (30)` 列，CAS metadata 持久化，CAS upload 数量上限。

---

## File Structure

**新增文件：**
- `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.up.sql` — 重命名列
- `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.down.sql` — 回滚

**修改文件：**
- `server-go/internal/service/metrics.go:22` — `supportedEventIDs` 加入 `5`
- `server-go/internal/service/metrics.go:132` — `CopyFrom` 列名 `external_prompt_id` → `external_session_id`
- `server-go/internal/database/migrations/001_create_tables.up.sql:77` — 列名同步重命名（保持新部署的 fresh schema 一致）
- `server-go/internal/service/metrics_test.go` — 新增 SessionEvent 接受测试 + 更新现有 fixture 的注释/字面值

**不需要修改的文件：**
- `internal/database/migrations/embed.go` — `//go:embed *.sql` 通配，自动包含新文件
- `internal/database/migrate.go` — 已通用，不感知具体迁移内容

---

## Task 1: 支持 event_id=5 (SessionEvent)

**Files:**
- Test: `server-go/internal/service/metrics_test.go` (新增测试函数)
- Modify: `server-go/internal/service/metrics.go:22`

- [ ] **Step 1: 写失败测试 — SessionEvent (event_id=5) 应被接受**

在 `server-go/internal/service/metrics_test.go` 末尾追加：

```go
func TestValidateEventAcceptsSessionEvent(t *testing.T) {
	svc := &MetricsService{}

	batch, err := svc.ValidateBatchShape(map[string]any{
		"v": float64(1),
		"events": []any{
			map[string]any{
				"t": float64(1712000000),
				"e": float64(5), // SessionEvent
				"v": map[string]any{
					"0": "session-started",
				},
				"a": map[string]any{
					"0":  "1.4.7",
					"1":  "https://github.com/test/repo",
					"23": "external-session-abc",
					"24": "session-123",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ValidateBatchShape() error = %v", err)
	}

	if len(batch.Events) != 1 {
		t.Fatalf("ValidateBatchShape() events = %d, want 1", len(batch.Events))
	}

	if errMsg := validateEvent(batch.Events[0]); errMsg != "" {
		t.Fatalf("validateEvent(SessionEvent) error = %q, want empty (event_id=5 should be supported)", errMsg)
	}
}
```

- [ ] **Step 2: 跑测试验证它失败**

Run（在 `server-go/` 目录下）：
```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/service/ -run TestValidateEventAcceptsSessionEvent -v
```

Expected:
```
--- FAIL: TestValidateEventAcceptsSessionEvent
    metrics_test.go:NN: validateEvent(SessionEvent) error = "e must be a supported event id", want empty (event_id=5 should be supported)
FAIL
```

- [ ] **Step 3: 修改 supportedEventIDs 把 5 加进去**

编辑 `server-go/internal/service/metrics.go:22`，把：

```go
var supportedEventIDs = map[int]bool{1: true, 2: true, 3: true, 4: true}
```

替换为：

```go
var supportedEventIDs = map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}
```

- [ ] **Step 4: 跑测试验证通过 + 整个 service 包测试不回归**

Run：
```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/service/ -v
```

Expected：`PASS`，含 `TestValidateEventAcceptsSessionEvent` 在内的所有 service 测试通过。

- [ ] **Step 5: 提交 Task 1**

```bash
cd /Users/hg/git/git-ai && git add server-go/internal/service/metrics.go server-go/internal/service/metrics_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: accept SessionEvent (event_id=5) in metrics validation

Client 1.4.6+ emits SessionEvent with event_id=5. Add it to
supportedEventIDs so uploads aren't rejected as "e must be a
supported event id".
EOF
)"
```

---

## Task 2: 重命名 metrics_events.external_prompt_id → external_session_id

**Files:**
- Create: `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.up.sql`
- Create: `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.down.sql`
- Modify: `server-go/internal/service/metrics.go:132`
- Modify: `server-go/internal/database/migrations/001_create_tables.up.sql:77`
- Modify: `server-go/internal/service/metrics_test.go` (现有 fixture 字面值)

- [ ] **Step 1: 写迁移 up SQL**

创建 `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.up.sql`，内容：

```sql
-- Align column name with client attr index 23 (EXTERNAL_SESSION_ID).
-- The column was originally named external_prompt_id but the client always
-- writes attr index 23 as external_session_id; this rename fixes the semantic
-- drift. No data conversion needed — values were already correct, only the
-- label was wrong.
ALTER TABLE metrics_events RENAME COLUMN external_prompt_id TO external_session_id;
```

- [ ] **Step 2: 写迁移 down SQL**

创建 `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.down.sql`，内容：

```sql
ALTER TABLE metrics_events RENAME COLUMN external_session_id TO external_prompt_id;
```

- [ ] **Step 3: 更新 service/metrics.go 中 CopyFrom 列名**

编辑 `server-go/internal/service/metrics.go`，把：

```go
				"git_ai_version", "repo_url",
				"tool", "model", "prompt_id", "external_prompt_id",
```

替换为：

```go
				"git_ai_version", "repo_url",
				"tool", "model", "prompt_id", "external_session_id",
```

- [ ] **Step 4: 更新 001_create_tables.up.sql 让 fresh schema 一致**

编辑 `server-go/internal/database/migrations/001_create_tables.up.sql:77`，把：

```sql
    external_prompt_id TEXT,
```

替换为：

```sql
    external_session_id TEXT,
```

> 说明：保留这个修改而不只靠 006 迁移的原因 — 一台干净库会顺序执行 001→006，先建 `external_prompt_id` 再 rename 成 `external_session_id`，行为正确；但让 001 也用新名是为了 fresh schema 阅读时一目了然，且 006 在没有列时也不会失败（已存在旧库才会真正生效）。

⚠️ 由于 001 改名后，已有旧库重跑 006 会找到 `external_session_id` 而找不到 `external_prompt_id` 报错。为兼容旧库 + 新库两条路径，把 006 改成幂等：见 Step 5。

- [ ] **Step 5: 让 006 迁移幂等**

覆盖 `server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.up.sql`：

```sql
-- Align column name with client attr index 23 (EXTERNAL_SESSION_ID).
-- The column was originally named external_prompt_id but the client always
-- writes attr index 23 as external_session_id; this rename fixes the semantic
-- drift. No data conversion needed.
--
-- Idempotent: skip when the rename was already applied (e.g. a database
-- created from the updated 001 migration where the column is already named
-- external_session_id).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'metrics_events'
          AND column_name = 'external_prompt_id'
    ) THEN
        ALTER TABLE metrics_events RENAME COLUMN external_prompt_id TO external_session_id;
    END IF;
END $$;
```

覆盖 down sql 让回滚也幂等：

```sql
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'metrics_events'
          AND column_name = 'external_session_id'
    ) THEN
        ALTER TABLE metrics_events RENAME COLUMN external_session_id TO external_prompt_id;
    END IF;
END $$;
```

- [ ] **Step 6: 更新 metrics_test.go 中现有 fixture 的语义字符串**

编辑 `server-go/internal/service/metrics_test.go`，在 `TestValidateBatchShapeAcceptsCurrentMetricsSchema` 的 attrs map 里，把：

```go
					"23": "external-prompt-123",
```

替换为：

```go
					"23": "external-session-123",
```

（只是把测试字面量改成新语义，不影响通过逻辑，但与现实数据保持一致避免误导后续读者。）

- [ ] **Step 7: 验证全仓内没有 external_prompt_id 残留**

Run：
```bash
cd /Users/hg/git/git-ai && grep -rn "external_prompt_id\|ExternalPromptID\|ExternalPromptId" server-go/
```

Expected: 仅 006 迁移文件内的 `external_prompt_id` 字符串（DO $$ 检测块、down 迁移）— 其他文件应当全部为零命中。

- [ ] **Step 8: 编译并跑全量测试**

Run：
```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```

Expected: build 通过；所有包测试 PASS，含上一任务新增的 `TestValidateEventAcceptsSessionEvent` 与已有 `TestValidateBatchShapeAcceptsCurrentMetricsSchema`。

- [ ] **Step 9: 提交 Task 2**

```bash
cd /Users/hg/git/git-ai && git add server-go/internal/database/migrations/001_create_tables.up.sql \
  server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.up.sql \
  server-go/internal/database/migrations/006_rename_external_prompt_id_to_external_session_id.down.sql \
  server-go/internal/service/metrics.go \
  server-go/internal/service/metrics_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: rename metrics_events.external_prompt_id to external_session_id

The column was misnamed: client attr index 23 has always been
external_session_id (src/metrics/attrs.rs EXTERNAL_SESSION_ID = 23),
but server was labeling that column external_prompt_id. Values were
correct, the label was wrong — rename in place via idempotent
migration 006 so both fresh and upgraded deployments converge on the
correct name.
EOF
)"
```

---

## Task 3: 端到端 smoke 验证（可选，但推荐）

**Files:** 无文件修改，仅本地验证。

- [ ] **Step 1: 启动本地 server 跑迁移**

Run（按本仓的部署习惯调整 DB URL；若你本地没现成 PG，可跳过 Task 3）：
```bash
cd /Users/hg/git/git-ai/server-go && DATABASE_URL=postgres://localhost/gitai_dev?sslmode=disable go run ./cmd/server 2>&1 | head -40
```

Expected: 日志含迁移成功（无 "running migrations" 错误），server 监听 `:3000`。

- [ ] **Step 2: 用 psql 校验列名**

Run：
```bash
psql postgres://localhost/gitai_dev -c "\d metrics_events" | grep -E "external_(prompt|session)_id"
```

Expected：仅匹配 `external_session_id`，不再出现 `external_prompt_id`。

- [ ] **Step 3: 构造 SessionEvent 上传请求**

按需登录或带 API Key 后：
```bash
curl -sS -X POST http://localhost:3000/worker/metrics/upload \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $TEST_API_KEY" \
  -d '{
    "v": 1,
    "events": [{
      "t": 1715000000,
      "e": 5,
      "v": {"0": "session-started"},
      "a": {"0": "1.4.7", "24": "sess-smoke", "23": "ext-sess-smoke"}
    }]
  }'
```

Expected: `{"success":true,"errors":[]}`（之前会返回 `errors:[{index:0,error:"e must be a supported event id"}]`）。

- [ ] **Step 4: 校验数据落库**

```bash
psql postgres://localhost/gitai_dev -c \
  "SELECT event_id, external_session_id, attrs_json->>'24' AS session_id_from_attrs \
   FROM metrics_events ORDER BY received_at DESC LIMIT 1"
```

Expected：返回一行，`event_id=5`，`external_session_id='ext-sess-smoke'`，`session_id_from_attrs='sess-smoke'`（session_id 仍只在 attrs_json，本次不补独立列，符合范围声明）。

- [ ] **Step 5: 关停 server，无需 commit**

Task 3 是验证步骤，无代码变更。

---

## 完成标准

- `go test ./...` 全绿，含新增 `TestValidateEventAcceptsSessionEvent`。
- `grep -rn external_prompt_id server-go/` 仅命中 006 迁移内部（用于幂等检测与 down 回滚）。
- 在本地 PG 上跑 server，`metrics_events.external_session_id` 列存在且无 `external_prompt_id`。
- SessionEvent (event_id=5) 上传成功，响应 `errors` 为空数组。

## 回滚

- Task 1 回滚：移除 `5: true`。
- Task 2 回滚：运行 `migrate down` 一步（执行 006 的 down），并把代码回滚到上一个 commit。down 迁移幂等，旧库/新库都安全。

---

## 本地验证记录（2026-05-11）

实际在本机 macOS + PostgreSQL 15 上完成端到端 smoke。

**Commits chain**（base `1cef8ba7` → HEAD `aadef969`）：

```
3346d0a2 server-go: accept SessionEvent (event_id=5) in metrics validation
a294bd8b server-go: rename metrics_events.external_prompt_id to external_session_id
aadef969 server-go/scripts: align smoke-test fixture with external_session_id rename
```

**环境：**
- 独立 smoke 库：`git_ai_smoke`（fresh，从 001→006 顺序跑完）
- 二进制：`/tmp/git-ai-server-smoke`（`go build -o … ./cmd/server`）
- 启动参数：`PORT=37337 APP_ENV=development JWT_SECRET=… DB_NAME=git_ai_smoke GIT_AI_API_KEY=smoke-api-key`

**验证项与结果：**

| # | 项 | 结果 |
|---|----|------|
| 1 | `RunMigrations` 启动无错 + `schema_migrations` 版本 `6`、`dirty=f` | ✅ |
| 2 | `\d metrics_events` 含 `external_session_id`，无 `external_prompt_id`、无索引残留 | ✅ |
| 3 | `POST /worker/metrics/upload` 带 `X-API-Key`，`event_id=5`、`attr 23/24` → `200 OK`、`{"errors":[],"success":true}` | ✅ |
| 4 | 入库行：`event_id=5`、`external_session_id='ext-sess-smoke'`、`attrs_json->>'24'='sess-smoke'`（session_id 留在 JSONB，符合范围声明） | ✅ |
| 5 | 反向对照：`event_id=99` 仍返回 `errors:[{index:0,error:"e must be a supported event id"}]` — 证明只放行了 5，不是放宽全部 | ✅ |

**入库样本：**

```
 event_id | schema_version | git_ai_version |           repo_url           | external_session_id | session_id_attr |       values_json
----------+----------------+----------------+------------------------------+---------------------+-----------------+--------------------------
        5 |              1 | 1.4.7          | https://github.com/test/repo | ext-sess-smoke      | sess-smoke      | {"0": "session-started"}
```

**结论：** 三个 commit 在本地 fresh DB 端到端工作，符合"完成标准"全部 4 项。计划达成。
