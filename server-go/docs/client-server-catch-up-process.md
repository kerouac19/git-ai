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

建议按以下节奏执行：

- 每次从上游同步 `main` 后执行一次完整盘点。
- 每周固定执行一次轻量盘点，即使没有同步上游。
- 发布私有化服务前执行一次完整验收。

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
- 服务端接口覆盖情况：本目录下新增或更新对应设计/追平文档
- 数据库变更：`server-go/internal/database/migrations/*`
- fresh install 目标 schema：`server-go/internal/database/schema.sql`
- 服务端实现：`server-go/internal/handler/*`、`server-go/internal/service/*`
- 测试：Go 单元测试、冒烟测试、必要时真实 CLI 联调步骤
- 部署说明：`server-go/README.md` 或 `server-go/docs/production-deployment.md`

## 执行流程

### 1. 客户端协议盘点

只以 Rust 客户端源码为准，不从服务端实现反推协议。

检查项：

- 搜索所有 product API 调用路径、method、headers、query、request body、response body。
- 重点检查 `ApiContext` 自动附带的认证和身份头。
- 重点检查 Metrics event id、values position、attrs position 是否新增或改名。
- 重点检查 CAS object payload、metadata、read/write 路径是否变化。
- 重点检查 OAuth device flow grant type 和错误码是否变化。
- 重点检查 Release channel、下载文件名、checksum 校验规则是否变化。

完成标准：

- `docs/http-client-contracts.md` 与当前 `src/` 中的真实客户端行为一致。
- 明确列出本轮新增、删除、改名、仅文档变化四类差异。

### 2. 服务端覆盖矩阵

为每个客户端相关功能建立一行覆盖矩阵：

```text
客户端功能 -> endpoint -> payload -> server-go route -> handler -> service -> 表/存储 -> 测试 -> 状态
```

状态建议使用：

- `covered`：已实现且测试覆盖。
- `partial`：主路径可用，但字段、错误码、权限或测试缺失。
- `missing`：客户端会调用，但服务端没有实现。
- `server-only`：仅私有化服务需要，客户端不会直接调用。
- `deprecated`：历史保留，当前客户端不再调用。

完成标准：

- 每个客户端 API 都能定位到 server-go route。
- 每个 `partial` / `missing` 都有明确后续任务。

### 3. 数据库设计先行

任何需要落库的服务端能力，先定义数据库形态。

检查项：

- 是否需要新增表、字段、索引或约束。
- 是否需要保留原始 JSON，以兼容客户端未来字段。
- 是否需要将常用字段 promote 为独立列。
- 是否需要数据回填或旧数据兼容。
- fresh install 的 `schema.sql` 是否与 migrations 最终状态一致。

完成标准：

- 新增 migration 可从旧库平滑升级。
- `schema.sql` 表示 fresh install 的目标状态。
- 直接导入 `schema.sql` 与空库跑 migrations 后的关键表结构一致。

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

数据库相关验证：

- 空库启动，确认 migrations 自动执行。
- 导入 `server-go/internal/database/schema.sql` 后启动，确认 migrations no-op 或只执行新增版本。
- 对比关键表字段，特别是 `metrics_events`、`bundles`、`cas_entries`、`users`、`orgs`。

接口冒烟：

```bash
cd server-go
bash scripts/smoke-test.sh http://127.0.0.1:3000
```

真实 CLI 联调：

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
- smoke test 与当前真实路由一致。
- 真实 CLI 能完成 login、checkpoint、metrics upload、CAS upload。
- Dashboard 能看到当前登录用户的同步数据。

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

- [ ] 已从 Rust 客户端源码重新盘点 API 和 Metrics 协议。
- [ ] `docs/http-client-contracts.md` 已更新。
- [ ] 覆盖矩阵中没有未解释的 `missing`。
- [ ] migrations 与 `schema.sql` 一致。
- [ ] 新增字段有测试或明确的查询理由。
- [ ] `go test ./...` 通过。
- [ ] smoke test 跟当前路由一致并通过。
- [ ] 真实 CLI 联调路径已验证或记录了未验证原因。
- [ ] README / 部署文档与实际行为一致。
- [ ] 本轮遗留问题已写入对应计划文档。

## 当前重点关注项

后续优先按以下方向补齐：

- 更新 `docs/http-client-contracts.md`，补齐 Metrics event `5`、committed values `13/14`、checkpoint values `7/8`、session_event values。
- 收敛 `schema.sql` 与 migrations 的最终表结构，尤其是 `metrics_events` 和 `bundles`。
- 修复 bundle 创建时的 `user_id` 归属链路。
- 梳理 `/oauth/device`、`/me` 等 SPA 路径由 nginx 托管还是 Go server 托管，并同步 README 与 smoke test。
- 保持组织模型一致：登录 token、`users.org_id`、Dashboard org 展示使用同一来源。
- 保持 `/api/dashboard/global` 为 admin-only，并在 smoke test 中覆盖权限边界。
- `generate-report` 在真实报表任务实现前必须返回明确的未实现状态，不能返回假成功。
