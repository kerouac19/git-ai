# Client/Server Catch-up Process

本文档定义 `server-go` 私有化托管服务跟随上游 `git-ai` 客户端演进的固定流程。

适用场景：

- `main` 分支从上游同步后，需要确认客户端协议是否变化。
- 客户端新增 Metrics、CAS、OAuth、Release、Bundle 或 Dashboard 相关能力。
- 服务端已有实现与客户端、数据库、部署文档之间出现漂移。
- 需要定期补齐私有化服务能力，并形成可追踪的迭代记录。

## 原则

1. 先分析客户端真实行为，再改服务端。
2. 先定义数据结构和数据库迁移，再实现 handler/service。
3. 每次只追平一组清晰的协议或功能，避免混入无关重构。
4. 所有客户端上报的原始数据必须保留，常用字段再按查询需要 promote 成独立列。
5. `schema.sql`、增量 migrations、Go model/service、接口文档必须保持一致。
6. 每次完成后必须留下验证命令和结果，方便下次继续。

## 固定周期

按以下三种节奏执行，每种节奏对应不同的执行范围：

- **完整盘点**：每次从上游同步 `main` 后执行。跑 Step 0–6 全套，产出 `YYYY-MM-DD-client-server-catch-up.md`。
- **轻量盘点**：每周固定执行一次，即使没有同步上游。只跑 Step 0（锚定上游 sha）+ Step 1（客户端协议盘点）+ Step 5 的 `go test ./...`；如发现差异升级为完整盘点。
- **发布前验收**：发布私有化服务前执行。在完整盘点基础上额外补 Step 5 的"真实 CLI 联调"全链路，并 cross-check 上一轮所有遗留项。

## 输入材料

每轮盘点先读取这些文件或目录：

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

## 输出物

每轮工作至少更新或确认以下内容：

- 客户端 HTTP 契约文档：`docs/http-client-contracts.md`
- 服务端接口覆盖情况：本目录下新增或更新对应设计/追平文档，命名为 `YYYY-MM-DD-client-server-catch-up.md`，结构遵循附录 A。
- 数据库变更：`server-go/internal/database/migrations/*`
- fresh install 目标 schema：`server-go/internal/database/schema.sql`
- 服务端实现：`server-go/internal/handler/*`、`server-go/internal/service/*`
- 测试：Go 单元测试、冒烟测试、必要时真实 CLI 联调步骤
- 部署说明：`server-go/README.md` 或 `server-go/docs/production-deployment.md`

## 执行流程

### 0. 上游版本锚点

在客户端仓库根目录运行 `git rev-parse HEAD`，把 sha 写入本轮执行文档头部（`Upstream client: src @ <sha>`）。轻量盘点也必须记录。

完成标准：

- 本轮执行文档头部含明确的上游 sha 行。
- 与上一轮 sha 不同时，能据此用 `git log <prev>..<cur> -- src/api src/auth src/metrics src/commands/upgrade.rs` 列出客户端侧增量提交，作为 Step 1 的盘点起点。
- 若 sha 与上一轮一致（轻量盘点常见），可在协议差异分类小节直接标记"本轮无变化"并跳过 Step 1 的细分检查。

### 1. 客户端协议盘点

只以 Rust 客户端源码为准，不从服务端实现反推协议。

检查项：

- 搜索所有 product API 调用路径、method、headers、query、request body、response body。
- 重点检查 `ApiContext` 自动附带的认证和身份头。
- 重点检查 Metrics event id、values position、attrs position 是否新增或改名。
- 重点检查 CAS object payload、metadata、read/write 路径是否变化。
- 重点检查 OAuth device flow grant type 和错误码是否变化。
- 重点检查 Release channel、下载文件名、checksum 校验规则是否变化。

固化检查命令（在客户端仓库根目录执行；命中行号需在本轮执行文档"本轮协议差异分类"小节作为证据引用）：

```bash
# Headers / auth identity
rg '"X-API-Key"|"X-Author-Identity"|"X-Distinct-ID"|"User-Agent"' src/api/ src/auth/

# OAuth grants & error codes
rg 'grant_type|device_code|install_nonce|refresh_token' src/auth/
rg 'authorization_pending|slow_down|access_denied|expired_token' src/auth/

# Metrics event id / values / attrs / tombstones
rg 'EventId|event_id\s*=|values\[|attrs\[|TOMBSTONE|tombstone' src/metrics/

# CAS payload schema & routes
rg '"/worker/cas|"/api/cas|hashes=' src/api/cas.rs

# Release channel allowlist & artifact names
rg 'latest|next|enterprise-latest|enterprise-next|SHA256SUMS|install\.(sh|ps1)' src/commands/upgrade.rs

# Server-only path 反向核查（应为 0 命中；非 0 表示该路径已被客户端调用，需从 Table B 迁回 Table A）
rg '"/api/cas|"/api/me|"/api/oauth/device/(info|approve|deny)"|"/worker/cas/checkout"' src/
```

完成标准：

- `docs/http-client-contracts.md` 与当前 `src/` 中的真实客户端行为一致。
- 明确列出本轮**新增 / 改名 / 删除收敛 / 行为变化 / 仅文档变化 / 客户端侧无变化**六类差异，不能漏类。无变化的类别显式写"本轮无"。

### 2. 服务端覆盖矩阵

本步骤产出**两张表**：

**Table A — 客户端契约矩阵**：只列 Rust 客户端实际调用的 endpoint。每行格式：

```text
客户端功能 -> endpoint -> payload -> server-go route -> handler -> service -> 表/存储 -> 测试 -> 状态 -> 本轮处理
```

Table A 状态：

- `covered`：已实现且测试覆盖。
- `partial`：主路径可用，但字段、错误码、权限或测试缺失。
- `missing`：客户端会调用，但服务端没有实现。
- `deprecated`：历史保留，当前客户端不再调用。

**Table B — Server-only 路径表**：server-go 实现但 Rust 客户端不调用的端点。每行格式：

```text
endpoint -> 用途 -> server-go route -> 调用方 -> 去留判断
```

Table B 去留判断：

- `keep`：有 SPA / admin / 基础设施依赖，必须保留。
- `consolidate`：可与 `/worker/*` 等已有路径合并。
- `deprecate`：无依赖，下一轮可下线。
- `pending-review`：依赖尚未确认，列入遗留项。

完成标准：

- 每个客户端 API 都能定位到 server-go route，Table A 中无未解释的 `missing`。
- 每个 `partial` / `missing` / `pending-review` 都有明确后续任务。
- Step 1 末尾的"Server-only path 反向核查"命令必须 0 命中（否则需把该路径从 Table B 迁回 Table A）。

### 3. 数据库设计先行

任何需要落库的服务端能力，先定义数据库形态。

检查项：

- 是否需要新增表、字段、索引或约束。
- 是否需要保留原始 JSON，以兼容客户端未来字段。
- 是否需要将常用字段 promote 为独立列。
- 是否需要数据回填或旧数据兼容。
- fresh install 的 `schema.sql` 是否与 migrations 最终状态一致。

关键表清单（每轮必须明确列出"必须含 / 必须不含"字段，与本轮协议差异分类的"新增 / 删除收敛"对齐）：

- `metrics_events`：必须含 `user_id`、`event_id`、`attrs_json`、`values_json` 及当前 promoted 列；必须不含已下沉到 `attrs_json` 的列。
- `bundles`：必须含 `user_id`、`updated_at`、`idx_bundles_user_id`。
- `cas_entries`：必须含 `hash`、加密载荷字段、`metadata_json`。
- `users` / `orgs`：必须含 `users.org_id` → `orgs.id` 的 FK；`orgs` 含登录 token 来源所需字段。
- `oauth_device_codes`：必须含 `subject_json`、`expires_at`、过期清理路径。

完成标准：

- 新增 migration 可从旧库平滑升级。
- `schema.sql` 表示 fresh install 的目标状态。
- **直接导入 `schema.sql` 与空库跑 migrations 后的关键表结构一致**——本项必须实跑，结果记入 Step 5 的"fresh install schema 双路径自检"小节。即使本轮决定延后存量迁移方案，这条 fresh install 双路径自检不能跳过。

### 4. 服务端实现

按模块补齐 handler 和 service：

- OAuth：device code、token exchange、refresh token、install nonce、错误码。
- Metrics：批量校验、原始 sparse payload 存储、常用 attrs promote、部分错误返回。
- CAS：JSON object upload/read、metadata、hash 规范、缺失对象返回。
- Bundles：鉴权、创建者归属、payload 校验、URL 生成。
- Releases：channel、artifact allowlist、SHA256SUMS、install script 下载。
- Dashboard：用户级、组织级、全局级聚合口径。
- Config/Admin：权限、CSRF、审计、敏感字段脱敏。

完成标准：

- handler 只做 HTTP、认证、参数和错误映射。
- service 承担业务逻辑和数据库访问。
- 错误响应与客户端处理逻辑兼容。
- 安全边界在路由层清楚表达。

### 5. 测试与联调

基础验证：

```bash
cd server-go
go test ./...
```

fresh install schema 双路径自检（对应 Step 3 完成标准，必须实跑）：

```bash
# 路径 A: 空库跑 migrations
createdb git_ai_migrations
DB_NAME=git_ai_migrations ./bin/git-ai-server  # 触发 migrations 后退出

# 路径 B: 直接导入 schema.sql
createdb git_ai_schema
psql -d git_ai_schema -f server-go/internal/database/schema.sql

# 对比关键表（清单见 Step 3）
for t in metrics_events bundles cas_entries users orgs oauth_device_codes; do
  diff <(psql -d git_ai_migrations -c "\d $t") <(psql -d git_ai_schema -c "\d $t")
done
```

接口冒烟：

```bash
cd server-go
bash scripts/smoke-test.sh http://127.0.0.1:3000
```

真实 CLI 联调（必须记录测试环境：域名、DB 名、当前 user_id）：

```bash
git-ai config set api_base_url https://<private-domain>
git-ai logout
git-ai login
git-ai whoami
git-ai bg restart
git-ai checkpoint mock_ai <file>
git-ai flush-metrics-db
```

完成标准：

- Go 测试通过。
- fresh install schema 双路径自检无 diff，或所有 diff 都有合理解释（如 `schema_migrations` seed 行）。
- smoke test 与当前真实路由一致。
- 真实 CLI 能完成 login、checkpoint、metrics upload、CAS upload。
- Dashboard 能看到当前登录用户的同步数据。
- 联调记录里写明用了哪个域名、哪个 DB、哪个 user_id，便于复现。

### 6. 文档收口

每轮完成后更新：

- 客户端契约变化。
- 服务端新增 endpoint。
- 数据库 schema/migration 变化。
- 部署环境变量变化。
- 已知限制和后续任务。

完成标准：

- README 不描述已经不存在的路径或占位行为。
- smoke test 不依赖已经废弃的 HTML/form 路由。
- 已知未实现功能明确标注为未实现或移除入口。

## 每轮验收清单

提交或发布前逐项确认：

- [ ] 已记录 Step 0 上游客户端 commit sha。
- [ ] 已从 Rust 客户端源码重新盘点 API 和 Metrics 协议（Step 1 固化命令全部跑过）。
- [ ] `docs/http-client-contracts.md` 已更新。
- [ ] Table A 中没有未解释的 `missing`，Table B 中每个 `pending-review` 已纳入遗留项。
- [ ] migrations 与 `schema.sql` 一致（Step 3 双路径自检已实跑）。
- [ ] 新增字段有测试或明确的查询理由。
- [ ] `go test ./...` 通过。
- [ ] smoke test 跟当前路由一致并通过。
- [ ] 真实 CLI 联调路径已验证或记录了未验证原因，且写明测试环境（域名/DB/user_id）。
- [ ] README / 部署文档与实际行为一致。
- [ ] 上一轮遗留项处理状态已写入本轮文档（done / rolled-over / dropped）。
- [ ] 本轮遗留问题已写入对应计划文档，含优先级（P0/P1/P2）和预计处理时机标签。

## 附录 A：单轮报告骨架

每轮执行文档（`YYYY-MM-DD-client-server-catch-up.md`）必须包含以下小节，顺序与命名固定。可以追加，不能删减；不适用的小节显式写"本轮无"。

````markdown
# YYYY-MM-DD Client/Server Catch-up

- Trigger: 完整盘点 / 轻量盘点 / 发布前验收（择一）
- Upstream client: src @ <sha>           # Step 0 锚点
- 上一轮文档: [YYYY-MM-DD-client-server-catch-up.md]
- 流程文档: server-go/docs/client-server-catch-up-process.md（每节标题后括注流程对应步骤）
- 流程重跑记录:
  - YYYY-MM-DD 初次盘点。
  - YYYY-MM-DD（如有重跑）：触发原因 + 修正结论。
- 关联文档:
  - 客户端契约: docs/http-client-contracts.md
  - 组织模型: server-go/docs/organization-model-current-state.md
  - 部署: server-go/README.md、server-go/docs/production-deployment.md

## 输入范围（Step 1 实际扫过的文件）

## 上一轮遗留项处理状态（非首轮必填）

| 上一轮编号 | 描述 | 本轮状态 (done / rolled-over / dropped) | 备注 |
| --- | --- | --- | --- |

## 本轮协议差异分类（Step 1）

### 新增
### 改名
### 删除 / 收敛
### 行为变化
### 仅文档变化
### 客户端侧无变化（已核查）

## 覆盖矩阵（Step 2 - Table A：客户端契约矩阵）

## Server-only 路径（Step 2 - Table B）

## 已完成（Step 3 / 4 / 6）

## 验证（Step 5）

### Go 单元测试
### fresh install schema 双路径自检（Step 3 完成标准）
### 接口冒烟
### 真实 CLI 联调（含测试环境：域名 / DB / user_id）

## 验收清单

逐项 [x] / [ ] / [n/a]，对照流程文档"每轮验收清单"。

## 遗留项

每项格式：序号 + 标题 + 优先级 (P0/P1/P2) + 预计处理时机（下一轮 / 视情况 / 永久 backlog） + 描述 + 外部 issue ID（如有）。

## 下一轮预读清单

按优先级排序，每项含估时与是否阻塞下一次 catch-up。
````
