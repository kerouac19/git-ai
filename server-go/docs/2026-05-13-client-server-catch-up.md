# 2026-05-13 Client/Server Catch-up

本轮按 `server-go/docs/client-server-catch-up-process.md` 执行一次追平。

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

## 覆盖矩阵

| 客户端功能 | Endpoint | server-go 覆盖 | 状态 | 本轮处理 |
| --- | --- | --- | --- | --- |
| OAuth device code | `POST /worker/oauth/device/code` | `CompatibilityHandler.StartDeviceFlow` / `DeviceFlowService` | covered | 保持不变 |
| OAuth token exchange | `POST /worker/oauth/token` | `CompatibilityHandler.ExchangeOAuthToken` / `DeviceFlowService` | covered | 保持不变 |
| Metrics upload | `POST /worker/metrics/upload` | `MetricsService.UploadBatch` | covered | 文档补齐 event `5`、新 values/attrs |
| CAS upload/read | `POST /worker/cas/upload`, `GET /worker/cas/` | `CasService` | covered | 保持不变 |
| Bundle create | `POST /api/bundles` | `BundleService.CreateBundle` | partial -> covered | 补齐 `user_id` 归属和 DB migration |
| Release check/download | `GET /worker/releases`, `GET /worker/releases/:channel/download/:name` | `ReleaseStore` | covered | README 去掉脚本“占位”描述 |
| Dashboard | `/api/dashboard/*` | `DashboardService` / `AdminDashboardService` | covered | `/global` 改为 admin-only，`generate-report` 改为 501 |

## 已完成

- 更新 `docs/http-client-contracts.md`：
  - 增加 Metrics event id `5` (`session_event`)。
  - 修正 attr `23` 为 `external_session_id`。
  - 补齐 attrs `24/25/26/27/30`。
  - 补齐 committed values `13/14`。
  - 补齐 checkpoint values `7/8`。
  - 补齐 session_event values `0/1/2/3`。
- 新增 migration `009_bundle_user_and_metrics_cleanup`：
  - 为 `bundles` 增加 `user_id`、`updated_at`。
  - 回填旧 bundle 的 `user_id = 'unknown'`。
  - 创建 `idx_bundles_user_id`。
  - 删除 `metrics_events` 中不再 promote 的 parent/custom attr 独立列，继续保留在 `attrs_json`。
- 更新 `server-go/internal/database/schema.sql`：
  - fresh install 目标版本更新为 migration version `9`。
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

## 遗留项

- 本轮遗留项已清零。
- 已在真实运行服务上执行 smoke test；该步骤仍依赖数据库、`JWT_SECRET`、CAS 加密 key 等环境变量。
