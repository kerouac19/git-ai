# 2026-05-14 Client/Server Catch-up

本轮按 `server-go/docs/client-server-catch-up-process.md` 执行一次追平。

- Trigger: 从上游同步 `main` 后的**完整盘点**（非每周轻量、非发布前验收）。
- Upstream client: src @ `ef8bc3157` (`Merge pull request #1363 from git-ai-project/fix/codex-e2e-tests`)，2026-05-14 通过 merge commit `ee1415e9b` (`Merge branch 'main' into server-feature`) 合入 `server-feature`。上轮锚点为 `212dad50b`。
- 上一轮文档：[`2026-05-13-client-server-catch-up.md`](2026-05-13-client-server-catch-up.md)。
- 范围说明：**本轮只盘点客户端契约、服务端缺口、`schema.sql` 与文档**；**不实现 server-go 侧的 notes HTTP backend**——该实现按 leftover §1 在下一轮落地（用户已确认本轮范围，详见验收清单"实施范围决定"行）。
- 流程文档：`server-go/docs/client-server-catch-up-process.md`（每节标题后括注流程文档对应步骤）。
- 流程重跑记录：

| 序次 | 日期 | 触发原因 | 初稿结论 | 复核 / 回填结论 |
| --- | --- | --- | --- | --- |
| 1 | 2026-05-14 | 完整盘点（首跑），上游通过 PR #1253 引入 HTTP notes backend | 见下文各小节 | — |

- 关联文档：
  - 客户端契约：`docs/http-client-contracts.md`（本轮新增 `## Notes (HTTP Authorship Notes Backend)` 章节）。
  - 组织模型现状：`server-go/docs/organization-model-current-state.md`。
  - 部署：`server-go/README.md`、`server-go/docs/production-deployment.md`。

## 输入范围（Step 1 实际扫过的文件）

- `src/api/*`（含本轮新增 `src/api/notes.rs`）
- `src/auth/client.rs`、`src/auth/types.rs`、`src/auth/state.rs`、`src/auth/credentials.rs`
- `src/metrics/*`
- `src/commands/upgrade.rs`
- 本轮新增范围：`src/notes/{mod,db,reference_server}.rs`、`src/git/notes_api.rs`、`src/commands/notes_migrate.rs`、`src/commands/fetch_notes.rs`、`src/commands/hooks/push_hooks.rs`、`src/git/sync_authorship.rs`（dispatch 触发面）
- `docs/http-client-contracts.md`
- `server-go/cmd/server/main.go`
- `server-go/internal/handler/*`
- `server-go/internal/service/*`
- `server-go/internal/database/migrations/*`
- `server-go/internal/database/schema.sql`
- `server-go/README.md`、`server-go/docs/production-deployment.md`
- `server-go/scripts/smoke-test.sh`

## 上一轮遗留项处理状态

对照 `2026-05-13-client-server-catch-up.md` "遗留项" 12+3 条逐项标注。**结论：上一轮以来除了 2026-05-14 流程文档加固提交 `238db217b` 外，server-go 侧无任何实质修改，所有遗留项一律 `rolled-over`。**

| 上一轮编号 | 描述 | 本轮状态 | 备注 |
| --- | --- | --- | --- |
| §1 | `generate-report` 实际报表未实现 | rolled-over | 仍 `501`；P2/永久 backlog 不变 |
| §2 | 存量数据库迁移方案待单独编写 | rolled-over | 仍未成文；本轮新增 §1（notes 表）后建议合并写入 |
| §3 | 组织模型仍是扁平 `users.org_id` | rolled-over | 未启动 |
| §4 | `oauth_device_codes.subject_json` 与 token 源切换的隐式联动 | rolled-over | 未补集成测试 |
| §5 | Server-only API surface 待评审落地 | rolled-over | 表 B 的 `deprecate` / `consolidate` 判断仍未转化为代码动作 |
| §6 | `DELETE /api/config/:key` 不存在 key 返回 500（应 404） | rolled-over | `service/sysconfig.go:194` 仍是旧写法 |
| §7 | `/worker/releases*` 链路缺 smoke 覆盖 | rolled-over | `smoke-test.sh` 仍 0 命中 `worker/releases` |
| §8 | smoke test 走 `install_nonce` 旁路 | rolled-over | 未引入 headless |
| §9 | smoke test 与 CI 未集成 | rolled-over | 仍依赖本地环境变量 |
| §10 | `metrics_events` 三列仅留在 `attrs_json` | rolled-over | 与 §2 合并 |
| §11 | CORS / 反向代理 nginx 样例 | rolled-over | `production-deployment.md` 未补 |
| §12 | `server-go/scripts/` 三个脚本无文档 | rolled-over | 仍未补/未删 |
| §13 | `metrics_events` 复合索引在存量库缺失 | rolled-over | **本轮双路径自检重复触发同样 diff**，详见 §"验证 / fresh install schema 双路径自检" |
| §14 | `audit_logs.created_at` 索引在存量库缺失 | rolled-over | 同上 |
| §15 | `users.org_id` / `bundles.user_id` 列顺序差异 | rolled-over | 已知可接受偏差，本轮自检再次出现，不作为新差异处理 |

## 本轮协议差异分类（流程 Step 1）

按流程文档第 1 步要求六分类。客户端事实均来自直接扫 `src/`。

### 新增

- **HTTP authorship notes backend**（来自上游 PR #1253 `feat(notes): config, notes-db, and API types for HTTP backend` + 后续 `feat(notes): wire HTTP backend dispatch in notes_api` / `feat(notes): warm notes cache on git pull when kind=http` / `feat(notes): add git-ai notes migrate bulk uploader` / `fix: address review findings in HTTP notes backend`）。客户端新增两个端点：
  - `POST /worker/notes/upload`（client：`src/api/notes.rs:27`；reference server 契约：`src/notes/reference_server.rs:24-50`）。请求体 `{entries:[{commit_sha, content}]}`，200 返回 `{success_count, failure_count}`，400 返回 `ApiErrorResponse`。
  - `GET /worker/notes/?commits=<sha1>,<sha2>,...`（client：`src/api/notes.rs:73`；reference server 契约：`src/notes/reference_server.rs:45-58`）。返回 `{notes: {sha: content}}`，404 视为空 map。
- **新 config 字段**：`notes_backend.kind`（`git_notes` / `http`，默认 `git_notes`）+ `notes_backend.backend_url`（kind=http 必填，可与 `api_base_url` 不同主机）。证据：`src/commands/config.rs:116-117,343-487`、`src/git/notes_api.rs:5,22-155`。
- **新 CLI 子命令**：`git-ai notes migrate`（bulk uploader，把本地 `refs/notes/ai` 全量推到 HTTP 后端）。证据：`src/commands/notes_migrate.rs:22`。
- **新触发面**：
  - `git pull` warmup：`src/git/notes_api.rs::warm_cache_for_remote`（chunks of 100 SHAs）。
  - `git push` post hook：`src/commands/hooks/push_hooks.rs:11`。
  - `git fetch` 后：`src/commands/fetch_notes.rs:97`。
  - 后台 sync：`src/git/sync_authorship.rs:240`。
  - 全部 dispatch 都受 `Config::get().notes_backend_kind() == NotesBackendKind::Http` 守卫；默认 `git_notes` 走原本地 `refs/notes/ai`，**对存量用户零影响**。

### 改名

- 本轮无。

### 删除 / 收敛

- 本轮无。

### 行为变化

- **`ApiContext::build_url` 行为变化**（PR #1253 内 `src/api/client.rs:214-244`）：原来用 `Url::parse(base).join(endpoint)`，会**丢弃** base URL 的 path 段（如 `https://host/api/gitai` join `/worker/x` 得到 `https://host/worker/x`）。本轮改成字符串拼接，**保留 base 路径前缀**——`https://host/api/gitai` + `/worker/x` 现在得到 `https://host/api/gitai/worker/x`。新增 3 个测试覆盖（`test_build_url_preserves_path_prefix*`、`test_build_url_preserves_query_string`）。
  - 影响：所有 `/worker/*`、`/api/*` 调用一致受影响，**不限于 notes**。
  - 对 server-go 的影响：当前 `api_base_url` 一般指向 root（`https://<domain>`），不带 path 前缀，**实际行为一致**。但若部署在反向代理 path 前缀下（`https://corp/edge/git-ai`），原客户端会丢前缀打到根路径，新客户端会保留——这反而是**修正了一个长期 bug**。production deployment 需要 nginx upstream 把前缀正确映射到 server-go 根路径。**已纳入 §11 nginx 样例**（仍是 P2 / 视情况）。

### 仅文档变化

- `docs/http-client-contracts.md`：本轮新增 `## Notes (HTTP Authorship Notes Backend)` 章节（"上传 / 读取 + auth 与 dispatch 触发面"）。

### 客户端侧无变化（本轮已核查）

- `MetricEventId` 枚举（仍为 `Committed=1`、`AgentUsage=2`、`InstallHooks=3`、`Checkpoint=4`、`SessionEvent=5`）。证据：`src/metrics/types.rs:18-24` 与 `git diff 212dad50b..HEAD -- src/metrics` 输出为空。
- Metrics attrs（仍为 `0/1/2/3/4/5/20/21/22-tombstoned/23/24/25/26/27/30`）。证据：`src/metrics/attrs.rs:30-44`。
- `ApiContext` headers（`User-Agent: git-ai/<CARGO_PKG_VERSION>` / `X-Distinct-ID` / `X-API-Key` 可选 / `X-Author-Identity` 仅当 API Key 设置 / `Authorization: Bearer`）。证据：`src/api/client.rs:127-272`。
- CAS 客户端调用路径仍仅 `POST /worker/cas/upload` + `GET /worker/cas/?hashes=`。证据：`src/api/cas.rs:18,84`；server-only `/worker/cas/checkout` 反向核查 0 命中。
- OAuth grant types (`device_code` / `refresh_token` / `install_nonce`) 与错误码 (`authorization_pending` / `slow_down` / `access_denied` / `expired_token`)。证据：`src/auth/client.rs:139,205,217,175-187`。
- Release channel allowlist (`latest` / `next` / `enterprise-latest` / `enterprise-next`)、`SHA256SUMS`、`install.sh` / `install.ps1` 下载文件名。证据：`src/commands/upgrade.rs:366,398-400` + `docs/http-client-contracts.md` Releases 章节。
- Server-only path 反向核查命令（`rg '"/api/cas|"/api/me|"/api/oauth/device/(info|approve|deny)"|"/worker/cas/checkout"' src/`）**0 命中**——表 B 各项仍属 server-only。

## 覆盖矩阵（流程 Step 2 - Table A：客户端契约矩阵）

只列出 Rust 客户端会实际调用的端点。新增 2 行 notes，余者沿用上轮（`bundle_create` 自上轮起为 `covered`）。

| 客户端功能 | Endpoint | Payload | server-go route | handler | service | 表/存储 | 测试 | 状态 | 本轮处理 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| OAuth device code | `POST /worker/oauth/device/code` | 空 body | `compatibility.go` | `CompatibilityHandler.StartDeviceFlow` | `auth/device_flow.go` | `oauth_device_codes` | `device_flow_test.go` + smoke ② | covered | 保持不变 |
| OAuth token exchange | `POST /worker/oauth/token` | `{grant_type, device_code\|refresh_token\|install_nonce}` | `compatibility.go` | `CompatibilityHandler.ExchangeOAuthToken` | `auth/device_flow.go` | `oauth_device_codes` + JWT signing | `device_flow_test.go` + smoke ② | covered | 保持不变 |
| Metrics upload | `POST /worker/metrics/upload` (+ `/workers/*`) | `{v, events[]: {t, e, v{}, a{}}}` | `compatibility.go` | `CompatibilityHandler.UploadWorkerMetrics` | `service/metrics.go` (`MetricsService.UploadBatch`) | `metrics_events` (pgx.CopyFrom) | `metrics_test.go` + smoke ⑤ | covered | 保持不变 |
| CAS upload | `POST /worker/cas/upload` | `{objects:[{hash, content, metadata?}]}` | `compatibility.go` | `CompatibilityHandler.UploadWorkerCas` | `service/cas.go` | `cas_entries` | `cas_test.go` + smoke ④ | covered | 保持不变 |
| CAS read | `GET /worker/cas/?hashes=h1,h2,...` | query | `compatibility.go` | `CompatibilityHandler.ReadWorkerCas` | `service/cas.go` | `cas_entries` | `cas_test.go` + smoke ④ | covered | 保持不变 |
| Bundle create | `POST /api/bundles` | `{title, data:{prompts:{...}}}` | `bundle.go` | `BundleHandler.Create` | `service/bundle.go` | `bundles` | `bundle_test.go` + smoke ⑩ | covered | 保持不变 |
| Release check | `GET /worker/releases` | 无 | `releases.go` | `ReleaseHandler.GetReleases` | `service/release_store.go` | 文件系统 manifest | `releases_test.go`、smoke 未覆盖 ⚠️ | covered (test gap) | 沿用上轮，缺 smoke 仍是 §7 |
| Release download | `GET /worker/releases/:channel/download/:name` | path | `releases.go` | `ReleaseHandler.Download` | `service/release_store.go` | 文件系统 artifact | `releases_test.go`、smoke 未覆盖 ⚠️ | covered (test gap) | 同上 |
| **Notes upload** *(new)* | `POST /worker/notes/upload` | `{entries:[{commit_sha, content}]}` → `{success_count, failure_count}` | **(无)** | **(无)** | **(无)** | **(无表)** | **(无 server 测试)** | **missing** | **本轮只盘点；server-go 落地见 leftover §1（P0 / 下一轮）** |
| **Notes read** *(new)* | `GET /worker/notes/?commits=<csv>` | `{notes:{sha:content}}`，404→空 map | **(无)** | **(无)** | **(无)** | **(无表)** | **(无 server 测试)** | **missing** | **本轮只盘点；server-go 落地见 leftover §1（P0 / 下一轮）** |

> Table A 出现 2 个 `missing`，**已在 leftover §1 给出明确后续任务**（实现路径、表设计、auth 对齐、smoke 覆盖）。符合流程文档 Step 2 完成标准。

## Server-only 路径（流程 Step 2 - Table B）

下列路径 server-go 实现但 **Rust 客户端从不调用**。证据：`rg '"/api/cas|"/api/me|"/api/oauth/device/(info|approve|deny)"|"/worker/cas/checkout"' src/` 0 命中。沿用上轮表，去留判断未变（仍是 leftover §5 待落地）。

| Endpoint | 用途 | server-go route | 调用方 | 去留判断 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `GET /worker/cas/checkout?hash=...` | 单对象读取（手测 / 兼容） | `compatibility.go::CheckoutWorkerCas` | 无（仅手测） | **deprecate** | 同上轮，仍待下线 |
| `POST /api/cas/upload`、`GET /api/cas/read/:hash` | SPA / 调试通道 | `cas.go::CasHandler.{Upload,Read}` | SPA（待确认） | **consolidate** | 同上轮 |
| `GET /api/me` | SPA "我的"页 | `compatibility.go::GetMe` | SPA | **keep** | SPA 必需 |
| `GET /api/oauth/device/info`、`POST /api/oauth/device/approve\|deny` | SPA 浏览器审批 | `device_flow.go` | SPA | **keep** | device flow UI 必需 |
| `POST /api/user/login`、`GET\|POST /api/user/logout`、`POST /api/user/register` | SPA / admin 注册批量脚本 | `login.go` | SPA + `register-users.sh` | **keep** | 仍在使用 |
| `PUT /api/releases/:channel/artifacts/:tag/:name`、`PUT\|GET /api/releases/:channel/current.json` | release admin 上传 | `release_admin.go` | `sync-releases.sh` | **keep** | release 上传通道 |
| `GET /api/dashboard/{public,stats}`、`POST /api/dashboard/generate-report`、`GET /api/dashboard/global` | SPA 仪表板 | `dashboard.go` / `admin_dashboard.go` | SPA | **keep** | `generate-report` 仍 501（§1） |
| `GET\|POST\|PATCH\|DELETE /api/config[/:key]` | SPA admin 配置 | `sysconfig.go` | SPA | **keep** | `DELETE` 404 语义 bug 见 §6 |
| `GET /health`、`/api/health`、`/api/health/database`、`/api/status`、`/api/version` | 探活 | `health.go` / `compatibility.go` | LB / 监控 | **keep** | 必需 |

## 已完成（流程 Step 3 / 4 / 6）

> 本节只记录代码 / `schema.sql` 目标态 / 客户端契约 / 文档四类落地。

- `docs/http-client-contracts.md` 新增 `## Notes (HTTP Authorship Notes Backend)` 章节（在 `## CAS` 之后、`## Metrics` 之前），覆盖：
  - 触发条件（`notes_backend.kind = http` opt-in、默认 `git_notes` 不调用、auth 与 daemon-flush 跳过逻辑）。
  - `POST /worker/notes/upload` 完整请求/响应/错误体，含 reference server 50 MiB body cap 提示。
  - `GET /worker/notes/?commits=` 含 100 SHA 一批的 chunk 行为、404 → 空 map 语义、warm-cache 视失败为 cache miss 的容忍逻辑。
- 关键表清单（流程 Step 3）"必须含 / 必须不含"对账：

  | 表 | 必须含 | 必须不含 | 本轮状态 |
  | --- | --- | --- | --- |
  | `metrics_events` | `user_id` / `event_id` / `attrs_json` / `values_json` + 上轮 promoted 列 | 已下沉到 `attrs_json` 的 `parent_session_id` / `external_parent_session_id` / `custom_attributes` 列 | ✅ schema.sql 已对齐；migrations 路径 §13 索引仍缺 |
  | `bundles` | `user_id` / `updated_at` / `idx_bundles_user_id` | — | ✅ schema.sql 已对齐 |
  | `cas_entries` | `hash` / 加密载荷字段 / `metadata_json` | — | ✅ |
  | `users` / `orgs` | `users.org_id` → `orgs.id` FK；`orgs` 含登录 token 来源字段 | — | ✅ |
  | `oauth_device_codes` | `subject_json` / `expires_at` / 过期清理路径 | — | ✅ |
  | **`notes`（新表，本轮未建）** | 至少 `commit_sha PRIMARY KEY` / `user_id` / `content` / `created_at` / `updated_at` | — | **❌ leftover §1 设计要求详见后文** |

- `server-go/internal/database/schema.sql` 与 migrations 一致性：仅有上轮已记录的差异（详见验证小节），无新增 diff。
- **本轮无 server-go 代码改动**（用户确认实施范围："只做报告 + 留 P0 leftover"）。

## 验证（流程 Step 5）

按流程文档第 5 步与"每次完成后必须留下验证命令和结果"的要求记录。

### Go 单元测试

```bash
cd server-go && go test ./...
```

结果（2026-05-14）：

```
?   	git-ai-server/cmd/server	[no test files]
ok  	git-ai-server/internal/auth	(cached)
ok  	git-ai-server/internal/config	(cached)
?   	git-ai-server/internal/crypto	[no test files]
?   	git-ai-server/internal/database	[no test files]
?   	git-ai-server/internal/database/migrations	[no test files]
ok  	git-ai-server/internal/handler	(cached)
ok  	git-ai-server/internal/middleware	(cached)
?   	git-ai-server/internal/model	[no test files]
ok  	git-ai-server/internal/service	(cached)
```

全部 cache 命中（server-go 自上轮以来零代码改动），所有有测试的包通过。

### fresh install schema 双路径自检（流程 Step 3 完成标准）

命令（在 `git-ai` 仓库根目录，psql 18.3 / Homebrew 本地）：

```bash
dropdb --if-exists git_ai_check_migrations 2>/dev/null
dropdb --if-exists git_ai_check_schema 2>/dev/null
createdb git_ai_check_migrations
createdb git_ai_check_schema

for f in server-go/internal/database/migrations/*.up.sql; do
  psql -d git_ai_check_migrations -q -v ON_ERROR_STOP=1 -f "$f"
done

psql -d git_ai_check_schema -q -v ON_ERROR_STOP=1 -f server-go/internal/database/schema.sql

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

附加观察（与上轮一致）：Path A 跑 `007_metrics_events_extra_attrs.up.sql` 时仍产生 `column "prompt_id" of relation "metrics_events" does not exist, skipping`、`relation "idx_metrics_events_session_id" already exists, skipping` 等 NOTICE；008 也有 `column "metadata" of relation "cas_entries" already exists, skipping`。属于 migration 文件之间的冗余 DDL，无功能影响。

结论：

- **结构 diff 与上轮 100% 重复**：上轮 §13（3 个 `metrics_events` 复合索引）、§14（1 个 `audit_logs.created_at` 索引）、§15（2 处列顺序）一项都没修。本轮**不重复创建 leftover**，按"上一轮遗留项处理状态"统一标 `rolled-over`。
- 这正是流程文档"每次完成后必须留下验证命令和结果"的价值：可以一眼看出哪些 leftover 是真没动。

### 接口冒烟

```bash
bash server-go/scripts/smoke-test.sh http://127.0.0.1:3000
```

**本轮未实跑**——server-go 自上轮以来零代码改动，且本轮范围不包括启动服务做端到端验证。脚本本身仍是上轮校准过的版本（含 install_nonce、global 权限边界、generate-report 501、82 条断言）。下一轮如果落地 leftover §1（notes 实现），smoke 必须新增 `/worker/notes/upload` + `/worker/notes/?commits=` 用例。

### 真实 CLI 联调（含测试环境）

**本轮未实跑**——同上原因，server-go 无代码改动，本轮范围限定为盘点+文档。同时 notes backend 的 dispatch 默认走 `git_notes`，要测试 `kind = http` 路径必须先把 server-go 侧的 `/worker/notes/*` 实现，才有意义。

下一轮（实现 notes backend）必须验证：

```bash
git-ai config set api_base_url https://<private-domain>
git-ai config set notes_backend.kind http
git-ai config set notes_backend.backend_url https://<private-domain>   # 同 API host 时
# 或独立服务: git-ai config set notes_backend.backend_url https://<notes-domain>
git-ai logout && git-ai login && git-ai whoami
git-ai bg restart
git-ai checkpoint mock_ai <file>      # 触发 commit + 后台 sync notes
git-ai notes migrate                   # bulk uploader, 把存量 refs/notes/ai 推到 HTTP
git pull                               # 触发 warm_cache_for_remote (chunks of 100)
```

并核验：

```bash
psql <db_name> -c "select user_id, count(*) from <notes_table> group by user_id;"
```

测试环境信息字段（域名 / DB / user_id）届时按附录 A 必填。

## 验收清单（流程 Step 6）

逐项确认：

- [x] 已记录 Step 0 上游客户端 commit sha（src @ `ef8bc3157`，merge 进 `server-feature` via `ee1415e9b`）。
- [x] 已从 Rust 客户端源码重新盘点 API 和 Metrics 协议（流程文档 6 条 rg 命令全部跑过；命中行号已作为证据引用）。
- [x] `docs/http-client-contracts.md` 已更新（新增 `## Notes (HTTP Authorship Notes Backend)` 章节）。
- [x] Table A 中**有** 2 个 `missing`（notes upload / read），**已在 leftover §1 给出明确后续任务**；Table B 中无未解释的 `pending-review`，所有 `deprecate` / `consolidate` 沿用上轮 §5 leftover。
- [x] 实施范围决定：用户确认本轮范围为"只做报告 + P0 leftover"。所有需要 server-go 代码改动的项（含 notes backend、§13/§14 索引 migration、§6 sysconfig 404）一律按 leftover 处理。
- [x] migrations 与 `schema.sql` 一致性：双路径自检 2026-05-14 实跑，diff 与上轮 100% 重复，沿用上轮 §13/§14/§15 标注。
- [x] 新增字段有测试或明确的查询理由：本轮无新增 server-go 字段；新表 `notes` 的字段设计在 leftover §1 写明。
- [x] `go test ./...` 通过（cache 命中两次确认 server-go 无变化）。
- [n/a] smoke test 跟当前路由一致并通过：本轮范围内未实跑（见验证小节）；下一轮 leftover §1 落地必须 smoke 覆盖 notes 两端点。
- [n/a] 真实 CLI 联调路径：本轮范围内未实跑；下一轮 leftover §1 落地必须按上文给的命令清单走全链路。
- [x] README / 部署文档与实际行为一致：未变化；nginx 反向代理+ build_url 行为变化的影响纳入上轮 §11。
- [x] 上一轮遗留项处理状态：12+3 条全部 `rolled-over`，详见专门小节。
- [x] 本轮新增遗留问题已写入下方"遗留项"，每项含优先级（P0/P1/P2）和预计处理时机。

## 遗留项

按"本轮新增 / 上轮转入"两类整理。优先级：`P0`（必须立刻修） / `P1`（下次 catch-up 前应解决） / `P2`（可延后）；时机：`下一轮` / `视情况` / `永久 backlog`。

### 本轮新增

1. **【P0｜下一轮】server-go 实现 HTTP authorship notes backend** —— 上游 PR #1253 已让客户端代码就绪，对应 endpoints `POST /worker/notes/upload` / `GET /worker/notes/?commits=` 在 server-go 侧 0 实现。下一轮（或独立 PR）必须落地：
   - **路由 / handler / service**：
     - 路由建议放在 `compatibility.go`（与 `/worker/*` 系列同组），新增 `NotesHandler.{Upload, Read}`；service 层 `NotesService.{Upload, Read}` 承担业务逻辑。
     - auth 沿用 `WorkerAuth` middleware（与 metrics / cas 一致），既支持 API key 也支持 Bearer JWT；同时尊重 daemon flush_notes "未登录则跳过" 的语义——server 不必特殊处理，鉴权失败正常 401 即可。
     - body 上限：参考 reference server 的 50 MiB（实际可按 metrics/cas 的现有 `bodyCap` 中间件复用）。
     - 上传 batch ceiling：建议 server 强制 ≤ 1000 entries / 请求（客户端 `git-ai notes migrate` 自己分块，无客户端固定上限）。
     - 读取 batch ceiling：客户端 chunks of 100，server 至少接受 100；建议 ≤ 200 防滥用。
   - **数据库**：新增 `notes` 表（migration `010_create_notes.up.sql` + 同步 `schema.sql`）。最小字段集：
     - `commit_sha TEXT PRIMARY KEY`
     - `user_id TEXT NOT NULL`（来自 JWT subject 或 API key 绑定，便于多租户隔离与 rate limit）
     - `org_id UUID NOT NULL REFERENCES orgs(id)`（与 `metrics_events` / `bundles` 对齐组织模型）
     - `content TEXT NOT NULL`（authorship-log payload 原文，UTF-8 序列化的 JSON 串）
     - `content_size INT NOT NULL`（promoted 字段，便于 dashboard 统计 / quota 控制）
     - `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
     - `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`（重复上传同 SHA 时更新）
     - `idx_notes_user_id`、`idx_notes_org_id`，**复合索引**：`(org_id, updated_at DESC)` 便于 dashboard "最近 N 天 notes" 查询，避免重复 §13 教训。
   - **写语义**：upload 走 UPSERT (`ON CONFLICT (commit_sha) DO UPDATE SET content = EXCLUDED.content, content_size = ..., updated_at = NOW()`)；逐条入库；返回的 `success_count` / `failure_count` 对应实际成功 / 失败数（DB 异常或 SHA 非 hex 算 failure）。
   - **读语义**：`GET /worker/notes/?commits=h1,h2,...` server 侧也按 csv 切分；至少返回客户端期望的 `{notes: {sha: content}}`；找不到任何一个 SHA 时返回 404；多租户隔离 —— 只返回当前 user_id / org_id 自己的 notes（防止跨租户读取），找不到的 SHA 直接当不存在处理。
   - **测试**：`notes_test.go`（service 层 happy path + UPSERT + auth + 多租户隔离 + 非 hex SHA）+ `notes_handler_test.go`（HTTP 层 200/400/404）。
   - **smoke**：`smoke-test.sh` 新增至少 3 个用例（upload 1 条、read 命中、read miss → 404）。
   - **审计**：写路径走 `AuditMiddleware`（与 cas/metrics 一致）。
   - **文档**：README 增 "Notes" 段（auth、batch limit、与 git_notes 后端的二选一）；`production-deployment.md` 提一句"notes 表数据量随提交数线性增长，建议监控表大小并设 30/90/365 天保留策略"。
   - **估时**：4–6h（含 migration + handler/service + tests + smoke + 文档）。
   - **阻塞**：是 —— 上游已发布带 HTTP backend 的客户端，私有化用户一旦把 `notes_backend.kind` 切到 `http` 就立刻 404；本项是发布前验收的硬阻塞。

2. **【P2｜视情况】`build_url` 行为变化与反向代理路径前缀** —— 客户端从 `Url::join` 改为字符串拼接，**保留 base URL path 前缀**。如果存在私有化部署把 server-go 挂在 `https://corp/edge/git-ai/` 之类的子路径下，旧客户端会丢前缀打到根（500/404），新客户端会保留前缀。
   - 行动项：在 `production-deployment.md` 的 nginx 样例（已是上轮 §11 的 leftover）里**显式说明**两种部署形态——根路径 vs 子路径——以及 `proxy_pass` 是否 trim 前缀的语义。
   - 与上轮 §11 合并处理。

### 上轮转入（详见本轮"上一轮遗留项处理状态"小节）

- §1 (`generate-report` 未实现) — P2 / 永久 backlog。
- §2 (存量数据库迁移方案) — P1 / 视情况。**建议下一轮把本轮 §1 的 `notes` 表迁移与 §13/§14 索引补齐合并写入同一份"存量库 in-place 改造方案"**。
- §3 (organization model) — P1 / 视情况。
- §4 (oauth subject_json 测试) — P2 / 下一轮。
- §5 (Server-only API surface 落地) — P1 / 下一轮。
- §6 (DELETE /api/config 404) — P1 / 下一轮。
- §7 (release smoke 缺失) — P1 / 下一轮。
- §8 (smoke headless 旁路) — P2 / 视情况。
- §9 (smoke + CI) — P1 / 视情况。
- §10 (metrics_events 三列只在 attrs_json) — P2 / 视情况。
- §11 (CORS / nginx 样例) — P2 / 下一轮，**与本轮新增 §2 合并**。
- §12 (server-go/scripts 三脚本无文档) — P2 / 下一轮。
- §13 (metrics_events 复合索引在存量库缺失) — P1 / 下一轮。
- §14 (audit_logs.created_at 索引在存量库缺失) — P1 / 下一轮。
- §15 (列顺序差异) — P2 / 永久 backlog（已知可接受偏差，不修）。

## 下一轮预读清单

按优先级排序，每项含估时与是否阻塞下次 catch-up。**本轮把"实现 notes backend"明确放在第一位**。

1. **新增 server-go notes backend 实现**（本轮 §1）：路由 + handler + service + `notes` 表 migration + smoke + 文档。**估时：4–6h，阻塞：是**——上游 PR #1253 已合，私有化用户切 `kind=http` 就 404。
2. **新增 migration `010_metrics_events_composite_indexes.up.sql` + `011_audit_logs_created_at_index.up.sql`**（上轮 §13、§14）。**估时：1h（写）+ 0.5h（再跑双路径自检验证），阻塞：是**——下一轮 catch-up 跑双路径自检时必须先把这两个补上，否则 diff 重复出现。
3. **存量数据库迁移方案单独成文**（上轮 §2，含 §13/§14 索引补齐 + 本轮 §1 notes 表 migration 的合并策略）。**估时：4–8h，阻塞：是**——下一轮发布前验收会要求存量库可以无损升级。
4. **Server-only API surface 评审落地**（上轮 §5，把 Table B 的 `deprecate` / `consolidate` 转化为代码动作或确认保留）。**估时：2–3h，阻塞：是**。
5. `server-go/internal/service/sysconfig.go` + `handler/sysconfig.go` — 修复 `DeleteConfig` 404 语义（上轮 §6）。**估时：1h，阻塞：否**。
6. `server-go/scripts/smoke-test.sh` — 补 `/worker/releases` 与 `/worker/releases/:channel/download/SHA256SUMS` 用例（上轮 §7）；同时补 notes 端点 smoke。**估时：1h，阻塞：否**（与本轮 §1 同 PR 顺手做掉更省事）。
7. `production-deployment.md` 补 nginx 反向代理样例 + 子路径前缀部署说明（上轮 §11 + 本轮 §2）。**估时：2h，阻塞：否**。
8. 上游 `src/api/*` / `src/metrics/*` / `src/notes/*` 在下次同步 `main` 后是否新增 event id / attr / endpoint（即下一轮 catch-up 的 Step 1 主体）。**估时：2–4h，阻塞：是**。
