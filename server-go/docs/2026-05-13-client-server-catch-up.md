# 2026-05-13 Client/Server Catch-up

本轮按 `server-go/docs/client-server-catch-up-process.md` 执行一次追平。

- Trigger: 从上游同步 `main` 后的完整盘点（非每周轻量、非发布前验收）。
- Upstream client: src @ `212dad50b` (`chore: bump version to 1.4.9 [skip ci]`，于 2026-05-13 10:16 +0800 通过 merge commit `c56cf029c` 合入 `server-feature`；本字段于 2026-05-14 流程更新后回填，原文未记录)。
- 范围说明：**本轮只盘点客户端契约、服务端实现、`schema.sql` 目标状态与文档**；**不在本文件落数据库存量迁移方案**。功能稳定后，存量库的迁移脚本与回滚策略单独成文。
- 流程文档：`server-go/docs/client-server-catch-up-process.md`（每节标题后括注流程文档对应步骤）。
- 流程重跑记录：

| 序次 | 日期 | 触发原因 | 初稿结论 | 复核 / 回填结论 |
| --- | --- | --- | --- | --- |
| 1 | 2026-05-13 | 完整盘点（首跑） | 见下文各小节 | — |
| 2 | 2026-05-13 | 同日按流程文档第 1–6 步逐项复核 | 矩阵中 `/api/cas/*` 列为客户端契约 | 实际属 server-only，已迁入"Server-only 路径"表；其余结论维持 |
| 3 | 2026-05-14 | 流程文档增补 Step 3 fresh install 双路径自检 + 附录 A 报告骨架后回填 | 当时声明"schema.sql 自身可用、存量迁移延后"，未跑双路径 diff；遗留项无优先级；CLI 联调环境未留底 | 实跑发现 4 类真实 diff（详见 §"验证 / fresh install schema 双路径自检"），新增遗留项 §13–§15；遗留项全部加 P0/P1/P2 + 时机标签；server-only 表加去留判断 |

- 关联文档：
  - 客户端契约：`docs/http-client-contracts.md`
  - 组织模型现状：`server-go/docs/organization-model-current-state.md`
  - 部署：`server-go/README.md`、`server-go/docs/production-deployment.md`

## 上一轮遗留项处理状态

本轮为首轮 catch-up，无上一轮遗留项。下一轮（含轻量盘点）起按流程文档附录 A 必填本节，对照本文件"遗留项"小节逐条标注 `done / rolled-over / dropped`。

## 输入范围

- `src/api/*`
- `src/auth/client.rs`
- `src/metrics/*`
- `src/commands/upgrade.rs`
- `src/daemon/telemetry_worker.rs`
- `docs/http-client-contracts.md`
- `server-go/cmd/server/main.go`
- `server-go/internal/handler/*`
- `server-go/internal/service/*`
- `server-go/internal/database/migrations/*`
- `server-go/internal/database/schema.sql`
- `server-go/README.md`
- `server-go/scripts/smoke-test.sh`

## 本轮协议差异分类（流程 Step 1）

按流程文档第 1 步要求，明确分四类。本节客户端事实均来自重跑时直接扫 `src/`：

### 新增

- Metrics event `5`（`session_event`）。
- Metrics session_event values `0/1/2/3`。
- Metrics committed values `13/14`（authorship 相关）。
- Metrics checkpoint values `7/8`（tool_use/file_edit 相关）。
- Metrics attrs `24/25/26/27/30`。
- `bundles` 在 fresh install 目标状态下持有 `user_id`、`updated_at` 字段与 `idx_bundles_user_id` 索引（仅 `schema.sql`，存量迁移见单独方案）。

### 改名

- attr `23`：客户端契约修正为 `external_session_id`（之前文档侧仍写 `external_prompt_id`）。

### 删除 / 收敛

- `metrics_events.parent_session_id`（attr 26 列）：`schema.sql` 目标状态不再 promote，保留在 `attrs_json` 中。
- `metrics_events.external_parent_session_id`（attr 27 列）：同上。
- `metrics_events.custom_attributes`（attr 30 列）：同上。
- 已核查 server 侧无 SQL 读引用上述列，仅 `metrics_test.go` 的注释提及（注释说明仍可从 `attrs_json` 回读）。
- **`committed` event 历史 tombstone 位**：客户端 `src/metrics/events.rs` 中 position `4`（旧 `mixed_additions`）、`7`（旧 `total_ai_additions`）、`8`（旧 `total_ai_deletions`）、`9`（旧 `time_waiting_for_ai`）已移除；attr `22`（`prompt_id`）仍标记为 TOMBSTONED（不再写入，仅读取兼容）。client contracts.md 已显式标注 removed/tombstoned。

### 行为变化

- `/api/dashboard/global`：改为 `jwtMW + adminOnly()`，非 admin 返回 403。
- `/api/dashboard/generate-report`：不再返回假成功，改为 `501 not_implemented`。
- `/api/bundles`：`user_id` 改为来自 JWT context（之前不绑定具体创建者）。
- 登录 token 中 `personal_org_id` / `orgs[]`：来自 `users.org_id JOIN orgs`，不再依赖 `DEFAULT_*` 环境变量（仅默认 / API key / 兼容路径仍走环境变量）。

### 仅文档变化

- README：标注 SPA 页面由 nginx/static hosting 托管；修正 Bundle/CAS 认证说明；修正 Metrics event id 与字段说明；标注 `/api/dashboard/global` 仅 admin；标注 `generate-report` 当前 `501`。
- `docs/http-client-contracts.md`：补齐上述 Metrics event/values/attrs。

### 客户端侧无变化（本轮已核查）

- `ApiContext` headers（`User-Agent: git-ai/<CARGO_PKG_VERSION>` / `X-Distinct-ID` / `X-API-Key` 可选 / `X-Author-Identity` 仅当 API Key 设置 / `Authorization: Bearer`）。证据：`src/api/client.rs` L127-266。
- CAS payload schema 与客户端实际调用路径 `POST /worker/cas/upload`、`GET /worker/cas/?hashes=...`（**注意**：`/worker/cas/checkout` 是 server-only，客户端不调用——见 §"Server-only 路径"）。证据：`src/api/cas.rs`。
- OAuth grant types (`device_code` / `refresh_token` / `install_nonce`) 与错误码 (`authorization_pending` / `slow_down` / `access_denied` / `expired_token`)。证据：`src/auth/client.rs` L60-218。
- Release channel allowlist (`latest` / `next` / `enterprise-latest` / `enterprise-next`)、`SHA256SUMS`、`install.sh` / `install.ps1` 下载文件名。证据：`src/commands/upgrade.rs` L322-406 + `docs/http-client-contracts.md` L411-415。

## 覆盖矩阵（流程 Step 2）

按流程文档第 2 步格式：客户端功能 → endpoint → payload → server-go route → handler → service → 表/存储 → 测试 → 状态。**只列出 Rust 客户端会实际调用的端点**；server 端额外存在的 SPA / admin / debug 路径见后续"Server-only 路径"小节。

| 客户端功能 | Endpoint | Payload | server-go route | handler | service | 表/存储 | 测试 | 状态 | 本轮处理 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| OAuth device code | `POST /worker/oauth/device/code` | 空 body | `compatibility.go` | `CompatibilityHandler.StartDeviceFlow` | `auth/device_flow.go` (`DeviceFlowService`) | `oauth_device_codes` | `device_flow_test.go` + smoke ② | covered | 保持不变 |
| OAuth token exchange | `POST /worker/oauth/token` | `{grant_type, device_code\|refresh_token\|install_nonce}` | `compatibility.go` | `CompatibilityHandler.ExchangeOAuthToken` | `auth/device_flow.go` | `oauth_device_codes` + JWT signing | `device_flow_test.go` + smoke ② | covered | 保持不变 |
| Metrics upload | `POST /worker/metrics/upload` (+ `/workers/*`) | `{v, events[]: {t, e, v{}, a{}}}` | `compatibility.go` | `CompatibilityHandler.UploadWorkerMetrics` | `service/metrics.go` (`MetricsService.UploadBatch`) | `metrics_events` (pgx.CopyFrom) | `metrics_test.go` + smoke ⑤ | covered | 文档补齐 event `5`、新 values/attrs |
| CAS upload | `POST /worker/cas/upload` | `{objects:[{hash, content, metadata?}]}` | `compatibility.go` | `CompatibilityHandler.UploadWorkerCas` | `service/cas.go` (`CasService`) | `cas_entries`（zlib + AES-256-GCM） | `cas_test.go` + smoke ④ | covered | 保持不变 |
| CAS read | `GET /worker/cas/?hashes=h1,h2,...` | query | `compatibility.go` | `CompatibilityHandler.ReadWorkerCas` | `service/cas.go` | `cas_entries` | `cas_test.go` + smoke ④ | covered | 保持不变。`/worker/cas/checkout` 是 server-only，客户端不调，移到下方清单 |
| Bundle create | `POST /api/bundles` | `{title, data:{prompts:{...}}}` | `bundle.go` | `BundleHandler.Create` | `service/bundle.go` (`BundleService.CreateBundle`) | `bundles` | `bundle_test.go` + smoke ⑩ | partial → covered | 代码侧补齐 `user_id` 归属；`schema.sql` 目标态包含 `user_id` 列。存量库迁移见单独方案 |
| Release check | `GET /worker/releases` | 无 | `releases.go` | `ReleaseHandler.GetReleases` | `service/release_store.go` (`ReleaseStore`) | 文件系统 manifest | `releases_test.go`、smoke 未覆盖 ⚠️ | covered (test gap) | README 去掉脚本"占位"描述；smoke 缺失列入遗留 §7 |
| Release download | `GET /worker/releases/:channel/download/:name` (含 `SHA256SUMS` / `install.sh` / `install.ps1`) | path | `releases.go` | `ReleaseHandler.Download` | `service/release_store.go` | 文件系统 artifact | `releases_test.go`、smoke 未覆盖 ⚠️ | covered (test gap) | 同上 |

## Server-only 路径（流程 Step 2，Table B）

下列路径 server-go 实现但 **Rust 客户端从不调用**。证据：`rg '"/api/cas|"/api/me|"/api/oauth/device/info|"/worker/cas/checkout' src/` 0 命中。本轮按流程文档新版要求加上**去留判断**字段（依据：调用方实际依赖；判断理由写在"备注"列）：

| Endpoint | 用途 | server-go route | 调用方 | 去留判断 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `GET /worker/cas/checkout?hash=...` | 单对象读取（手测 / 兼容） | `compatibility.go::CheckoutWorkerCas` | 无（仅手测） | **deprecate** | 客户端走 `/worker/cas/?hashes=`；建议下一轮下线 |
| `POST /api/cas/upload`、`GET /api/cas/read/:hash` | SPA / 调试通道（与 `/worker/cas/*` 并存） | `cas.go::CasHandler.{Upload,Read}` | SPA（待确认） | **consolidate** | 与 `/worker/cas/*` 路径重复，建议合并或仅保留一套；先列入遗留 §5 评审 |
| `GET /api/me` | SPA "我的"页 | `compatibility.go::GetMe` | SPA | **keep** | SPA 必需 |
| `GET /api/oauth/device/info`、`POST /api/oauth/device/approve\|deny` | SPA 浏览器审批 | `device_flow.go` | SPA | **keep** | device flow UI 必需；smoke test 已覆盖 |
| `POST /api/user/login`、`GET\|POST /api/user/logout`、`POST /api/user/register` | SPA / admin 注册批量脚本 | `login.go` | SPA + `register-users.sh` | **keep** | 仍在使用 |
| `PUT /api/releases/:channel/artifacts/:tag/:name`、`PUT\|GET /api/releases/:channel/current.json` | release admin 上传（`uploadAuth` Bearer） | `release_admin.go` | `sync-releases.sh` | **keep** | release 上传通道，下游脚本依赖 |
| `GET /api/dashboard/{public,stats}`、`POST /api/dashboard/generate-report`、`GET /api/dashboard/global` | SPA 仪表板，`generate-report` 当前 501，`global` admin-only | `dashboard.go` / `admin_dashboard.go` | SPA | **keep** | `generate-report` 实现前保持 501（遗留 §1） |
| `GET\|POST\|PATCH\|DELETE /api/config[/:key]` | SPA admin 配置 | `sysconfig.go` | SPA | **keep** | SPA admin 必需；`DELETE` 404 语义 bug 见遗留 §6 |
| `GET /health`、`/api/health`、`/api/health/database`、`/api/status`、`/api/version` | 基础设施 / 负载均衡探活 | `health.go` / `compatibility.go` | LB / 监控 | **keep** | 探活必需 |

> 本表的 `deprecate` / `consolidate` 判断尚未真正落地（无对应 PR），列入遗留 §5。下一轮 catch-up 必须把这些判断转化为具体动作或确认保留。

## 已完成（流程 Step 3 / 4 / 6）

> 本节只记录"代码、`schema.sql` 目标状态、客户端契约、smoke、README"四类落地。存量数据库的升级 / 回填 / 回滚不属于本轮范围。

- 更新 `docs/http-client-contracts.md`：
  - 增加 Metrics event id `5` (`session_event`)。
  - 修正 attr `23` 为 `external_session_id`。
  - 补齐 attrs `24/25/26/27/30`。
  - 补齐 committed values `13/14`。
  - 补齐 checkpoint values `7/8`。
  - 补齐 session_event values `0/1/2/3`。
- 更新 `server-go/internal/database/schema.sql`（fresh install 目标态）：
  - `bundles` 含 `user_id`、`updated_at` 与 `idx_bundles_user_id`。
  - `metrics_events` 不再持有 `parent_session_id` / `external_parent_session_id` / `custom_attributes` 列。
  - 该文件作为新库唯一权威源；存量库如何收敛到该目标态由后续迁移方案承担，本文不展开。
- 修复 bundle 创建链路：
  - handler 从 JWT context 取当前用户 id。
  - service 写入 `bundles.user_id`。
  - model 返回 `user_id` 和 `updated_at`。
- 更新 `server-go/scripts/smoke-test.sh`：
  - 改用 `/api/oauth/device/info`、`/api/oauth/device/deny`。
  - 非交互 token 改用 `install_nonce`，避免依赖已废弃的 HTML/form route。
  - Metrics payload 覆盖 event `5` 和当前 sparse fields。
- 更新 `server-go/README.md`：
  - 标注 SPA 页面由 nginx/static hosting 托管。
  - 修正 Bundle、CAS 认证说明。
  - 修正 Metrics event id 和字段说明。
  - 标注 `/api/dashboard/global` 仅 admin 可访问。
  - 标注 `generate-report` 当前返回 `501 not_implemented`。
- 后续修正：
  - 登录 token 中 `personal_org_id` / `orgs[]` 改为来自 `users.org_id JOIN orgs`。
  - `/api/dashboard/global` 路由改为 `jwtMW + adminOnly()`。
  - `/api/dashboard/generate-report` 不再返回假成功，改为 `501 not_implemented`。
  - smoke test 覆盖 global 权限边界和 generate-report 未实现响应。

## 验证（流程 Step 5）

按流程文档第 5 步与"每次完成后必须留下验证命令和结果"的要求记录。

### Go 单元测试

```bash
cd server-go
go test ./...
```

结果（2026-05-13 首跑）：

```
?   	git-ai-server/cmd/server	[no test files]
ok  	git-ai-server/internal/auth	1.354s
ok  	git-ai-server/internal/config	0.906s
?   	git-ai-server/internal/crypto	[no test files]
?   	git-ai-server/internal/database	[no test files]
?   	git-ai-server/internal/database/migrations	[no test files]
ok  	git-ai-server/internal/handler	1.682s
ok  	git-ai-server/internal/middleware	2.105s
?   	git-ai-server/internal/model	[no test files]
ok  	git-ai-server/internal/service	2.604s
```

结果（同日重跑，cache 命中说明 server 代码无改动）：

```
ok  	git-ai-server/internal/auth	(cached)
ok  	git-ai-server/internal/config	(cached)
ok  	git-ai-server/internal/handler	(cached)
ok  	git-ai-server/internal/middleware	(cached)
ok  	git-ai-server/internal/service	(cached)
```

所有有测试的包均通过。

### fresh install schema 双路径自检（流程 Step 3 完成标准，2026-05-14 回填）

本轮原始执行时只跑了"`schema.sql` 直建启动"，未跑流程文档第 116 行要求的"空库跑 migrations 后与 `schema.sql` 关键表结构一致"对比。流程文档 2026-05-14 版本把双路径自检列为 Step 3 必跑项后，本节回填实跑结果。

命令（在 `git-ai` 仓库根目录，psql 18.3 / Homebrew 本地）：

```bash
dropdb --if-exists git_ai_check_migrations 2>/dev/null
dropdb --if-exists git_ai_check_schema 2>/dev/null
createdb git_ai_check_migrations
createdb git_ai_check_schema

# Path A: 顺序跑 *.up.sql（不走 RunMigrations，避免起 server / 配 env）
for f in server-go/internal/database/migrations/*.up.sql; do
  psql -d git_ai_check_migrations -q -v ON_ERROR_STOP=1 -f "$f"
done

# Path B: 直接导入 schema.sql
psql -d git_ai_check_schema -q -v ON_ERROR_STOP=1 -f server-go/internal/database/schema.sql

# 关键表 \d 对比（清单见流程 Step 3 关键表清单）
for t in orgs users oauth_device_codes metrics_events audit_logs bundles cas_entries config; do
  diff <(psql -d git_ai_check_migrations -c "\d $t" 2>&1) \
       <(psql -d git_ai_check_schema -c "\d $t" 2>&1)
done
```

实跑结果（2026-05-14，仅列有 diff 的表）：

```
--- users ---
10a11
>  org_id        | uuid | not null | '00000000-0000-0000-0000-0000000000a1'::uuid
13d13
<  org_id        | uuid | not null | '00000000-0000-0000-0000-0000000000a1'::uuid

--- metrics_events ---
29c29,30
<     "idx_metrics_events_repo_url" btree (repo_url)
---
>     "idx_metrics_events_event_ts" btree (event_id, event_timestamp)
>     "idx_metrics_events_repo_ts" btree (repo_url, event_timestamp)
31c32
<     "idx_metrics_events_user_id" btree (user_id)
---
>     "idx_metrics_events_user_ts" btree (user_id, event_timestamp DESC)

--- audit_logs ---
16a17
>     "idx_audit_logs_created_at" btree (created_at)

--- bundles ---
4a5
>  user_id    | text | not null |
8d8
<  user_id    | text | not null |
```

`orgs / oauth_device_codes / cas_entries / config` 四张表无 diff。

附加观察：Path A 跑 `007_metrics_events_extra_attrs.up.sql` 时产生多条 `column ... already exists, skipping` NOTICE，原因是 `metrics_events` 在 `001_create_tables.up.sql` 已经创建了相同列名——`007` 文件里大量 `ADD COLUMN IF NOT EXISTS` 实际是 no-op。属于 migration 文件之间的冗余 DDL，无功能影响，但可在下一轮合并清理。

结论：

- **结构 diff（cosmetic）**：`users.org_id`、`bundles.user_id` 列顺序不同（migrations `ALTER ADD COLUMN` 挂在表末尾，schema.sql 把它们重新组织到了 NOT NULL 列簇）。PostgreSQL 不依赖列顺序；仅当应用代码靠列序（位置 INSERT、`SELECT *` 解构）或对 `pg_dump` 输出做精确 diff 时才有意义。判定为**可接受偏差**，转入遗留 §15。
- **索引 diff（functional，重大）**：存量库走 migrations 后缺 3 个 `metrics_events` 复合索引（`idx_metrics_events_event_ts`、`idx_metrics_events_repo_ts`、`idx_metrics_events_user_ts`）和 1 个 `audit_logs.created_at` 索引；migrations 路径保留了 2 个单列索引（`idx_metrics_events_repo_url`、`idx_metrics_events_user_id`），schema.sql 已用复合索引替代。**直接影响 Dashboard 按时间窗 + user_id / repo_url 过滤的查询性能**。转入遗留 §13（metrics_events 索引补 migration）和 §14（audit_logs.created_at 索引补 migration）。
- 这正是流程文档新增双路径自检要求的目的：本轮原始执行时跳过，今天回填发现真实分歧。下一轮起按流程必跑。

### 接口冒烟

```bash
bash server-go/scripts/smoke-test.sh http://127.0.0.1:3000
```

本轮真实运行通过；该步骤仍依赖数据库、`JWT_SECRET`、CAS 加密 key 等环境变量，CI 未集成。重跑确认脚本含 **82** 个 `assert_*` 断言。

注意：smoke test 当前**未覆盖** `/worker/releases*`、`register-users.sh`、`batch-create-users.sh`、`deploy.sh`、`sync-releases.sh` 等脚本，且其 `DELETE /api/config/nonexistent` 用例断言的是 `500`（见遗留项 §6）。

### 真实 CLI 联调

```bash
git-ai config set api_base_url https://<private-domain>
git-ai logout
git-ai login
git-ai whoami        # User ID 与浏览器 /me 页面"用户识别码"一致
git-ai bg restart
git-ai checkpoint mock_ai <file>
git-ai flush-metrics-db
```

随后核验 `metrics_events` 写入：

```bash
psql <db_name> -c "select user_id, count(*) as events, max(received_at) as last_sync from metrics_events group by user_id;"
```

本轮在生产环境完成 login → checkpoint → flush 全链路，Dashboard "正在同步" / "最后同步" 显示如预期。

**测试环境信息（域名 / DB 名 / user_id）原文未记录**——流程更新前未要求显式留底；下一轮起按附录 A "真实 CLI 联调（含测试环境）" 必填，便于复现和回查 metrics 写入。

## 验收清单（流程 Step 6）

逐项确认：

- [x] 已记录 Step 0 上游客户端 commit sha（2026-05-14 回填，src @ `212dad50b`）。
- [x] 已从 Rust 客户端源码重新盘点 API 和 Metrics 协议（本轮重跑直接扫 `src/api/*`、`src/auth/client.rs`、`src/metrics/events.rs`、`src/metrics/attrs.rs`、`src/commands/upgrade.rs`）。
- [x] `docs/http-client-contracts.md` 已更新且与客户端真实行为一致（含 committed `4/7/8/9 removed`、attr `22 tombstoned`）。
- [x] 覆盖矩阵中没有未解释的 `missing`；`server-only` 路径单独列出且本次加去留判断（`generate-report` 501、release smoke 缺失列为遗留）。
- [x] `schema.sql` 自身能直建并启动服务。**fresh install 双路径自检 2026-05-14 实跑**，发现 4 类 diff：2 处列顺序（可接受 / §15）、3 个 metrics_events 索引缺失（§13）、1 个 audit_logs 索引缺失（§14）。存量库 in-place 改造方案延后单独编写（见遗留项 §2）。
- [x] 新增字段有测试或明确的查询理由（`bundles.user_id` 有 `bundle_test.go` 和归属查询；`MetricsService.UploadBatch` 已核查写入 11 个 promoted 列与 schema 列一一对齐）。
- [x] `go test ./...` 通过（首跑实跑 + 重跑 cache 命中两次确认）。
- [x] smoke test 跟当前路由一致并通过（含 install_nonce、global 权限边界、generate-report 501）；脚本含 82 条断言。
- [x] 真实 CLI 联调路径已验证；测试环境信息原文未记录（下一轮起按附录 A 必填）。
- [x] README / 部署文档与实际行为一致（除 §12 中三个未文档化的脚本）。
- [n/a] 上一轮遗留项处理状态：本轮为首轮，无可对照项。
- [x] 本轮遗留问题已写入对应计划文档，每项含优先级（P0/P1/P2）和预计处理时机（见下方"遗留项"）。

## 遗留项

按"本轮没动 / 仍未实现 / 已知 bug / 已知未覆盖 / 双路径自检新发现"五类整理。每项格式：`【优先级】【时机】 标题`，其中：

- 优先级：`P0`（必须立刻修） / `P1`（下次 catch-up 前应解决） / `P2`（可延后）
- 时机：`下一轮` / `视情况` / `永久 backlog`

1. **【P2｜永久 backlog】`generate-report` 实际报表未实现**：当前返回 `501 not_implemented`。需要后续设计真实报表存储 / 异步任务 / 下载流程后再放开。
2. **【P1｜视情况】存量数据库迁移方案待单独编写**：本轮只把 `schema.sql` 收敛到目标态（`bundles.user_id` / `updated_at` 新增、`metrics_events` 三列下沉到 `attrs_json`），未提供存量库 `up` / `down` / 数据回填 / 回滚策略。等本轮功能在 fresh install 环境稳定后，单独成文（建议放在 `server-go/docs/` 下，与本文件交叉引用），需覆盖：
   - 存量 `bundles` 的 `user_id` 历史归属来源（CAS / metrics / audit_logs 回查 vs 统一标记 `unknown`）。
   - `metrics_events` 三列下沉的 in-place 改造（`DROP COLUMN` vs 改写为视图）。
   - 与 §13、§14 索引补齐 migration 的合并。
   - 回滚路径：列被删除后，新写入数据丢失对应 promoted 值，回滚需要明确"是否补 JSONB → 列回填"。
   - 与 `schema_migrations` 表的衔接（fresh install 走 `schema.sql` seed，存量走 migration 文件）。
3. **【P1｜视情况】组织模型仍是扁平 `users.org_id`，无 membership**：见 [`organization-model-current-state.md`](organization-model-current-state.md)。该文档的"后续改造方向"七项（`organization_memberships` 表、`orgs.slug`、登录逻辑、API key subject、组织管理接口、下游 `org_id` 写入 / 查询）本轮均未动。
4. **【P2｜下一轮】`oauth_device_codes.subject_json` 与本轮 token 源切换的隐式联动**：登录 token 的 `personal_org_id` / `orgs[]` 现在来自 `users.org_id JOIN orgs`；device flow 审批时会把 JWT claims 复制进 `subject_json`，因此 CLI 拿到的组织信息也跟着切换数据来源——未单独写测试覆盖。补 1 个 device_flow_test.go 集成 case 即可。
5. **【P1｜下一轮】Server-only API surface 待评审**：本轮已在"Server-only 路径"表给出去留判断（`deprecate` / `consolidate` / `keep`），但**判断尚未转化为代码动作**。下一轮必须落地：`/worker/cas/checkout` 下线、`/api/cas/*` ↔ `/worker/cas/*` 合并方案、其余 `keep` 项补契约文档。
6. **【P1｜下一轮】`DELETE /api/config/:key` 不存在的 key 返回 500（应 404）**：`SysConfigService.DeleteConfig` 在 `RowsAffected()==0` 时返回普通 error，`handler.Delete` 统一映射到 `Internal()`。smoke test 第 484-486 行的断言 `→ 500` 在文档化这个 bug。需要在 service 层新增 `ErrConfigNotFound`，在 handler 映射为 404。
7. **【P1｜下一轮】`/worker/releases*` 链路缺 smoke 覆盖**：已有 `releases_test.go`，但 `smoke-test.sh` 没有任何 `/worker/releases` 断言，无法在联调阶段保护真实下载 / SHA256SUMS 链路。
8. **【P2｜视情况】smoke test 走 `install_nonce` 旁路**：真实 `device_code` 浏览器审批 + cookie session 路径没有 headless 测试覆盖；登录页 UX 回归只能靠手测。需要引入 headless browser 框架（playwright/puppeteer）成本较高。
9. **【P1｜视情况】smoke test 与 CI 未集成**：当前依赖 `JWT_SECRET` / CAS key / 数据库等环境变量，需要一份 docker-compose 或 dev secrets 才能纳入 CI。
10. **【P2｜视情况】`metrics_events` 被下沉的 3 列仅保留在 `attrs_json`**：已核查 server 端无 SQL 引用。若 dashboard / admin_dashboard 后续要按 `parent_session_id` / `custom_attributes` 过滤，需要新增 JSONB 索引或重新 promote 为列；本轮不作此优化，纳入 §2 迁移方案一并设计。
11. **【P2｜下一轮】CORS / 反向代理示例**：README 注明 SPA 由 nginx 托管，但没有配套 nginx + Go server 的样例 conf；初次部署用户容易踩坑。建议 `production-deployment.md` 补一份样例。
12. **【P2｜下一轮】`server-go/scripts/` 中三个脚本无文档**：本轮重跑发现仓库还有 `batch-create-users.sh`、`deploy.sh`、`sync-releases.sh`，README 只描述了 `register-users.sh` 和 `smoke-test.sh`。要么补文档，要么从仓库移除以保持 README 与脚本一致。

### 双路径自检新发现（2026-05-14 回填）

13. **【P1｜下一轮】`metrics_events` 复合索引在存量库缺失**：fresh install schema.sql 含 `idx_metrics_events_event_ts(event_id, event_timestamp)`、`idx_metrics_events_repo_ts(repo_url, event_timestamp)`、`idx_metrics_events_user_ts(user_id, event_timestamp DESC)`；存量库走 migrations 后仅有 `idx_metrics_events_repo_url` / `idx_metrics_events_user_id` 两个单列索引。Dashboard 按时间窗过滤查询在存量库上会全表扫 / 选错索引。**修复**：新增 `010_metrics_events_composite_indexes.up.sql` 创建 3 个复合索引并 DROP 2 个单列索引；写法见 schema.sql 第 ~130–140 行。
14. **【P1｜下一轮】`audit_logs.created_at` 索引在存量库缺失**：fresh install 含 `idx_audit_logs_created_at(created_at)`；存量库走 migrations 后无此索引。审计查询按时间排序在存量库会慢。**修复**：在 §13 的 migration 内一并补 `CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at`，或单独成 `011_audit_logs_created_at_index.up.sql`。
15. **【P2｜永久 backlog】`users.org_id` / `bundles.user_id` 列顺序与 schema.sql 不一致**：migrations 通过 `ALTER ADD COLUMN` 把列加在末尾，schema.sql 把它们组织到 NOT NULL 列簇。PostgreSQL 不依赖列顺序，应用代码也不依赖位置 INSERT，判定为**可接受偏差**，**不修复**。本条作为已知 diff 留底，避免下一轮自检时被误报为"新差异"。

## 下一轮预读清单

便于下次直接开工，按优先级排。每项含**估时**和**是否阻塞下次 catch-up**：

1. `server-go/internal/service/sysconfig.go` + `handler/sysconfig.go` — 修复 `DeleteConfig` 404 语义（遗留 §6）。**估时：1h，阻塞：否**（独立 bug fix PR）。
2. `server-go/scripts/smoke-test.sh` — 补 `/worker/releases` 与 `/worker/releases/:channel/download/SHA256SUMS` 用例（遗留 §7）。**估时：1h，阻塞：否**。
3. **新增 migration `010_metrics_events_composite_indexes.up.sql` + `011_audit_logs_created_at_index.up.sql`**（遗留 §13、§14）。**估时：1h（写 migration）+ 0.5h（再跑双路径自检验证），阻塞：是**——下一轮 catch-up 跑双路径自检时必须先把这两个补上，否则 diff 会重复出现。
4. **存量数据库迁移方案单独成文**（遗留 §2，含与 §13/§14 的合并策略 + §15 的可接受偏差记录）。**估时：4–8h（写文档 + 验证 in-place 改造路径），阻塞：是**——下一轮发布前验收会要求存量库可以无损升级。
5. **Server-only API surface 评审落地**（遗留 §5，依据本轮"Server-only 路径"表的去留判断）。**估时：2–3h，阻塞：是**——流程文档新版要求 Table B 每个 `pending-review` 已纳入遗留项；下一轮必须把本轮 `deprecate` / `consolidate` 转化为具体动作或确认保留。
6. `server-go/docs/organization-model-current-state.md` 的"后续改造方向" — 决定是否启动 membership 表改造（遗留 §3）。**估时：评估 1h + 实施 8–16h，阻塞：否**。
7. `server-go/docs/production-deployment.md` — 补 nginx + Go server 反向代理样例（遗留 §11）。**估时：2h，阻塞：否**。
8. `server-go/scripts/` 三个脚本（`batch-create-users.sh` / `deploy.sh` / `sync-releases.sh`）补 README 或下线（遗留 §12）。**估时：1h，阻塞：否**。
9. 上游 `src/api/*` / `src/metrics/*` 在下次同步 `main` 后是否新增 event id 或 attr position（即下一轮 catch-up 的 Step 1 主体）。**估时：2–4h（取决于上游变化量），阻塞：是**——这就是下一轮 catch-up 的核心工作。
