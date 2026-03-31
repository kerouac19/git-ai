# Server 实现审查记录（2026-03-27）

本文档最初记录的是基于企业部署文档对 `server/` 实现做的一次静态审查结论，供后续修复和分阶段落地参考。

注意:

- 下文“详细发现”章节保留的是本次审查当时的历史快照
- 2026-03-27 晚些时候，相关实现已继续推进，最新状态见下方“后续实现更新”

关联文档:

- [Git-AI 企业版部署指南](/Users/hg/git/git-ai/.docs/enterprise-deployment-guide.md)
- [Git-AI 私有化部署拆解: `git-ai login` / `git-ai dash`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md)

## 后续实现更新（2026-03-27 晚）

在本次静态审查之后，`server/` 已继续补齐了一批 P0/P1/P2 能力。当前状态已经不再是文档开头所述的“本地兼容样例”。

已关闭或基本关闭的 gap:

- `JWT_SECRET` 现在必须显式配置，`config` 端点已显式收紧到 `admin`
- `/api/me` 已只认服务端认证身份，不再允许 `query.userId` 或 `x-user-id` 越权覆盖
- `/me` 已是实际 HTML dashboard 页面，不再只是跳到 JSON
- Device Flow 已有真实 `pending / approved / denied / expired / slow_down` 基础状态机，并持久化到 PostgreSQL
- 浏览器授权页已支持 approve / deny，并会给浏览器写入 session cookie
- `POST /worker/metrics/upload` 已实现，dashboard 已能展示 `last_sync_at`、`event_count_7d`、`repo_count_7d`
- CAS 主协议已对齐客户端真实实现，支持 `POST /worker/cas/upload` JSON 批量上传和 `GET /worker/cas?hashes=...` 批量读取
- `trust proxy` 与 HTTPS redirect 已改为显式配置驱动，不再写死
- `server/Dockerfile`、`server/.env.example`、根目录 `.env.example`、`docker-compose.yml` 已补齐到当前部署模型
- `CAS_ENCRYPTION_KEY` 的固定测试默认值已移除；生产环境现在要求显式稳定密钥
- 敏感配置读取已统一返回 `[REDACTED]`
- 审计日志已入库到 `audit_logs`，不再只写 `console.log`
- 已确认本地 PostgreSQL 中 `oauth_device_codes`、`metrics_events`、`audit_logs`、`cas_entries` 可随实际联调写入，`login -> metrics -> CAS` 链路已跑通

仍未关闭的主要 gap:

- 外部 IdP / SSO 接入仍未实现
- 多实例高可用仍缺少更完整的共享状态与运行手册
- Prometheus / metrics exporter 仍未补齐
- CAS 访问治理和更细粒度权限模型仍较弱
- 更完整的 Compose / Kubernetes / 监控交付物仍需继续补
- 客户端当前不会在 `git commit` 后自动调用 `/api/authorship/record` 或 `/api/authorship/commit`，因此 `authorship_records` / `commit_attributions` 目前仍不是自动闭环数据源

最新验证结果:

- `server/` 下的 `pnpm build` 已通过
- 新增 smoke 脚本 [scripts/test-private-flow.sh](/Users/hg/git/git-ai/scripts/test-private-flow.sh#L1)
- 已实际跑通 `device code -> approve -> token -> /api/me -> metrics upload -> /me -> CAS upload -> CAS read`
- 已实际确认最新 commit 对应的 3 个 `messages_url` hash 最终进入 PostgreSQL `cas_entries`
- `authorship_records` / `commit_attributions` 当前没有自动新增，不是本次私有化 server 联调故障，而是客户端尚未实现对应上报链路

因此，本文后续“详细发现”更适合作为 2026-03-27 当次静态审查的历史 gap list，而不是当前仓库状态总览。

## 审查范围

本次审查主要对照以下目标契约:

- 企业部署总体架构与部署假设
- `git-ai login` 所需的 OAuth Device Flow
- `git-ai dash` 所需的 `/me` 页面
- metrics 接收链路
- CAS prompt store 兼容协议
- 基础安全、鉴权、代理和容器化部署

本次检查方式:

- 阅读 `.docs` 下部署文档
- 阅读 `server/src` 当前实现
- 阅读客户端侧对接代码，确认真实协议
- 在 `server/` 目录执行一次 `pnpm build`

限制:

- 本次没有启动 PostgreSQL
- 本次没有启动多实例、负载均衡或反向代理
- 本次没有执行端到端联调

## 结论概览

结论不是“server 完全不可用”，而是“当前实现更接近本地兼容样例，不满足 `.docs` 中描述的企业部署闭环”。

影响最大的差距集中在 6 个方面:

1. JWT 和配置管理接口存在高风险安全缺口
2. `/me` 不是受保护的 dashboard 页面，且存在越权读取问题
3. Device Flow 只是内存级自动放行模拟，不满足真实部署要求
4. `worker` CAS 兼容层与当前客户端真实协议不兼容
5. metrics 上传链路未实现，dashboard 数据基础不成立
6. 文档中的部署方式与当前容器/代理实现不一致

## 详细发现

以下内容是本次静态审查当时的历史发现，部分问题现已修复，阅读时请以上文“后续实现更新”为准。

### 1. 严重: `JWT_SECRET` 默认值可预测，配置接口没有真正的管理员约束

现状:

- `JWT_SECRET` 未配置时回退到固定值 `git-ai-local-dev-secret`
- JWT payload 缺少 `role` 时，`JwtStrategy` 默认把用户角色视为 `admin`
- `ConfigController` 虽然使用了 `PermissionGuard`，但没有任何 `roles` metadata
- `PermissionGuard` 在没有显式角色声明时默认要求 `Role.USER`

这会导致两个问题:

1. 企业部署若漏配 `JWT_SECRET`，攻击者可以伪造 Bearer token
2. 即使 token 只是普通用户身份，也能访问配置管理端点

相关代码:

- [server/src/auth/auth.module.ts](/Users/hg/git/git-ai/server/src/auth/auth.module.ts#L7)
- [server/src/auth/jwt.strategy.ts](/Users/hg/git/git-ai/server/src/auth/jwt.strategy.ts#L17)
- [server/src/config/config.controller.ts](/Users/hg/git/git-ai/server/src/config/config.controller.ts#L16)
- [server/src/guards/permission.guard.ts](/Users/hg/git/git-ai/server/src/guards/permission.guard.ts#L19)

与文档冲突点:

- 企业部署文档默认这是可放到生产环境中的 API server
- 当前实现实际上仍带有本地开发默认口令和默认管理员身份假设

建议修复方向:

- 生产环境必须强制要求显式 `JWT_SECRET`
- `JwtStrategy` 不应把缺失角色默认抬升为 `admin`
- 为 `config` 端点显式声明管理员角色
- 若没有完善 RBAC，至少先把 `config` 整体收紧到单一管理员能力

### 2. 严重: `/me` 不是受保护的 dashboard 页面，且允许调用方覆盖用户身份

客户端真实行为很简单:

- `git-ai dash` 只会打开 `${api_base_url}/me`

当前服务端行为:

- `/me` 直接 302 到 `/api/me`
- `/api/me` 返回 JSON，不是受保护页面
- `/api/me` 会优先使用 `query.userId`
- 其次接受 `x-user-id`
- 没有 Bearer token 时仍会回退到默认用户

结果:

- 与文档要求的“已登录用户可访问的 `/me` 页面”不一致
- 形成明显的越权读取面，调用方可伪造 `userId` 读取他人 dashboard/authorship 数据

相关代码:

- [src/commands/personal_dashboard.rs](/Users/hg/git/git-ai/src/commands/personal_dashboard.rs#L4)
- [server/src/main.ts](/Users/hg/git/git-ai/server/src/main.ts#L77)
- [server/src/compatibility/compatibility.controller.ts](/Users/hg/git/git-ai/server/src/compatibility/compatibility.controller.ts#L81)

相关文档:

- [`.docs/private-deployment-login-dash-implementation-plan.md`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md#L403)

建议修复方向:

- `/me` 应返回真实页面或 SSR 内容，而不是无鉴权 JSON
- 页面身份必须来自服务端认证态，不应接受 `query.userId`
- `/api/me` 若保留为 JSON 接口，也应只读取 token 主体身份

### 3. 严重: Device Flow 是“1 秒后自动成功”的进程内模拟，不满足企业部署和 HA 要求

文档要求的最小闭环包括:

- `POST /worker/oauth/device/code`
- 浏览器授权页
- `POST /worker/oauth/token`
- `pending / approved / denied / expired` 状态流转
- 多实例/企业部署下可持续工作

当前实现不是这个模型:

- `device_code` 状态存在内存 `Map`
- `readyAt = now + 1000`
- 只要轮询时间到了就直接发 token
- 浏览器授权页仅展示文案，不会真正批准或拒绝授权
- 没有 `slow_down`
- 没有 `access_denied`
- 没有持久化，也没有跨实例共享状态

这意味着:

- 重启进程后设备码状态全部丢失
- 多实例部署时轮询请求落到其他节点会失败
- 浏览器授权步骤只是视觉上的兼容页，不是真实审批链路

相关代码:

- [server/src/auth/compatibility-auth.service.ts](/Users/hg/git/git-ai/server/src/auth/compatibility-auth.service.ts#L32)
- [server/src/main.ts](/Users/hg/git/git-ai/server/src/main.ts#L57)

相关文档:

- [`.docs/private-deployment-login-dash-implementation-plan.md`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md#L165)
- [`.docs/enterprise-deployment-guide.md`](/Users/hg/git/git-ai/.docs/enterprise-deployment-guide.md#L166)

建议修复方向:

- 设备码状态持久化到数据库或共享缓存
- 浏览器页增加 approve/deny 动作
- token 轮询实现 `authorization_pending`、`slow_down`、`access_denied`、`expired_token`
- 若要支持多实例，必须去掉进程内单点状态

### 4. 严重: `worker` CAS 兼容协议与客户端真实实现不兼容

客户端当前真实协议:

- 上传: `POST /worker/cas/upload`，JSON body，支持批量 `objects`
- 读取: `GET /worker/cas/?hashes=...`

当前服务端实现:

- 上传: `POST /worker/cas/upload`，但要求 multipart `file`
- 读取: `GET /worker/cas/checkout?id=...` 或 `hash=...`

这不是增强功能缺失，而是协议不匹配:

- 客户端发 JSON 时，当前服务端会因为缺少 multipart `file` 而失败
- 客户端读 CAS 时访问的是 `/worker/cas/?hashes=...`，当前服务端根本没有这个路由

相关代码:

- [src/api/cas.rs](/Users/hg/git/git-ai/src/api/cas.rs#L17)
- [server/src/compatibility/compatibility.controller.ts](/Users/hg/git/git-ai/server/src/compatibility/compatibility.controller.ts#L175)

相关文档:

- [`.docs/private-deployment-login-dash-implementation-plan.md`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md#L708)

建议修复方向:

- 按客户端协议实现批量 JSON 上传
- 增加 `/worker/cas/?hashes=...` 批量读取
- 返回 `results / success_count / failure_count`
- 将当前 `checkout` 视为旧兼容路径，而不是主协议

### 5. 高: metrics 上传链路未实现，dashboard 聚合数据也不符合文档预期

客户端行为已确定:

- 登录后会后台上传 metrics
- 接口为 `POST /worker/metrics/upload`
- 响应体应为 `{ "errors": [] }`

当前服务端情况:

- `server/src` 中没有 `/worker/metrics/upload`
- 没有 metrics 原始事件存储
- dashboard 聚合完全不是基于 metrics
- 当前实现甚至把 `casEntry.count() * 1000` 当成 `totalTokens`

这意味着文档里这些能力当前都不成立:

- 登录后自动上传指标
- `/me` 展示最近同步时间
- 近 7 天事件数 / 最近 repo / 最近 tool-model 统计

相关代码:

- [src/api/metrics.rs](/Users/hg/git/git-ai/src/api/metrics.rs#L111)
- [server/src/dashboard/aggregated-metrics.service.ts](/Users/hg/git/git-ai/server/src/dashboard/aggregated-metrics.service.ts#L15)

相关文档:

- [`.docs/private-deployment-login-dash-implementation-plan.md`](/Users/hg/git/git-ai/.docs/private-deployment-login-dash-implementation-plan.md#L468)

建议修复方向:

- 增加 `/worker/metrics/upload`
- 建 metrics 事件表，至少存原始 `event_id / timestamp / values / attrs / distinct_id`
- dashboard 统计改为从 metrics 聚合而来
- `/me` 页面增加 `last_sync_at`

### 6. 中: 部署文档中的 Redis、多实例、监控、Compose 工件与当前实现对不上

企业部署指南描述了这些能力或前提:

- Redis
- 多实例 API server
- 反向代理和 `TRUST_PROXY`
- Prometheus 指标导出
- Docker Compose / Kubernetes 交付物

但当前实现/仓库状态是:

- 没有 Redis 依赖和接入点
- 没有 metrics exporter
- 没有多实例共享状态设计
- `trust proxy` 在代码里写死为 `1`
- 仓库内没有 `.env.example`
- 仓库内没有 `docker-compose.yml`

相关代码与文档:

- [`.docs/enterprise-deployment-guide.md`](/Users/hg/git/git-ai/.docs/enterprise-deployment-guide.md#L98)
- [server/src/main.ts](/Users/hg/git/git-ai/server/src/main.ts#L41)

建议处理方式:

- 要么收缩文档，只保留当前仓库真实支持的部署模型
- 要么补齐 Redis、共享状态、监控和交付工件

### 7. 中: HTTPS/代理逻辑与文档意图不一致

文档显式提到:

- 生产环境必须 HTTPS
- 可能通过 reverse proxy / load balancer 终止 TLS

当前中间件逻辑:

- 只要 `NODE_ENV === 'production'`，就把请求视为 secure
- 因此即使请求本身不是 HTTPS，也通常不会进入实际 redirect 分支

这会让“HTTPS redirect enabled”在日志上看起来已启用，但实际不成立。

相关代码:

- [server/src/middleware/https-redirect.middleware.ts](/Users/hg/git/git-ai/server/src/middleware/https-redirect.middleware.ts#L9)

建议修复方向:

- 仅根据 `req.secure` 或可信代理头判断是否 HTTPS
- `trust proxy` 应改为环境变量驱动，而不是写死
- `HTTPS_REDIRECT` 应来自显式配置，而不是静态常量

### 8. 中: Dockerfile 基本不可复现当前项目的生产构建

当前 Dockerfile 有几个明显问题:

- 使用 `npm ci --only=production`，但项目实际以 `pnpm-lock.yaml` 为准
- 构建依赖在 `devDependencies` 中，`--only=production` 后通常不可用
- 没有复制 `tsconfig.json`
- 没有复制 `prisma/`
- 只复制 `src/`

因此，文档里描述的容器化部署链路按当前仓库内容并不可信。

相关代码:

- [server/Dockerfile](/Users/hg/git/git-ai/server/Dockerfile#L1)
- [server/package.json](/Users/hg/git/git-ai/server/package.json#L6)

建议修复方向:

- 改为 `pnpm` multi-stage 构建
- 复制 `package.json`、`pnpm-lock.yaml`、`tsconfig.json`、`nest-cli.json`、`prisma/`、`src/`
- 在 builder 阶段执行 `pnpm install --frozen-lockfile` 和 `pnpm build`
- 在 runtime 阶段只保留运行所需产物

### 9. 中: 加密能力带有生产不安全默认值和未兑现的“安全存储”表述

当前实现里:

- `ENCRYPTION_MASTER_KEY` 不存在时会临时生成随机 key
- `CAS_ENCRYPTION_KEY` 不存在时会回退到固定测试默认值
- `ConfigService` 对敏感配置做了加密写入，但读取时不会解密，直接返回存储值
- 审计日志只是 `console.log`

这更像“演示版安全层”，不能直接当成企业级静态加密和审计体系。

相关代码:

- [server/src/security/encryption.service.ts](/Users/hg/git/git-ai/server/src/security/encryption.service.ts#L10)
- [server/src/cas/cas.service.ts](/Users/hg/git/git-ai/server/src/cas/cas.service.ts#L69)
- [server/src/config/config.service.ts](/Users/hg/git/git-ai/server/src/config/config.service.ts#L277)
- [server/src/security/audit-log.middleware.ts](/Users/hg/git/git-ai/server/src/security/audit-log.middleware.ts#L123)

建议修复方向:

- 生产环境强制要求稳定密钥
- 去掉 CAS 测试默认 key
- 明确敏感配置 API 是否允许解密读取；若不允许，应返回 mask，而不是密文
- 将审计日志接入真实存储或外部日志系统

## 与文档契约的对应结论

### 对 `.docs/enterprise-deployment-guide.md` 的结论

当前 `server/` 还不能支撑文档中暗示的这些能力:

- 多实例高可用
- Redis 参与的缓存/共享状态
- 可直接复用的 Docker Compose / K8s 交付物
- Prometheus 监控集成
- 企业级可依赖的 TLS/代理/安全配置

结论:

- 该指南当前更像目标蓝图，不是仓库现状说明

### 对 `.docs/private-deployment-login-dash-implementation-plan.md` 的结论

P0/P1/P2 中有多项目前未达成:

- P0:
  - `/me` 页面闭环未达成
  - Device Flow 状态机未达成
  - 真实浏览器审批未达成
- P1:
  - `/worker/metrics/upload` 未实现
  - dashboard 真实数据闭环未达成
- P2:
  - CAS 主协议与客户端不兼容

结论:

- 当前实现只能算“本地兼容原型”
- 不能按该计划文档宣称为已具备私有化闭环

## 本次验证结果

### 静态审查当次验证

执行结果:

```bash
cd server
pnpm build
```

结果:

- TypeScript 构建通过

解释:

- 这只证明代码在当前依赖下可以编译
- 不能证明数据库迁移、容器构建、OAuth 联调、CAS 协议、metrics 协议、多实例部署真实可用

### 后续联调验证更新

后续已补充 smoke 脚本:

```bash
./scripts/test-private-flow.sh
```

已覆盖:

- `POST /worker/oauth/device/code`
- 浏览器 approve
- `POST /worker/oauth/token`
- `GET /api/me`
- `POST /worker/metrics/upload`
- `GET /me`
- `POST /worker/cas/upload`
- `GET /worker/cas?hashes=...`

结论:

- 当前 `server/` 已具备最小私有化闭环的本地 smoke 级可用性
- 但这仍不等于外部 IdP、多实例 HA、监控与治理层面都已完成

## 后续建议

建议后续按优先级推进，而不是并行发散:

1. 先收口安全面
   - 去掉默认 `JWT_SECRET`
   - 修正 `/api/me` 越权
   - 锁紧 `config` 权限
2. 跑通 P0 真闭环
   - 持久化 Device Flow
   - 做真实 `/me` 页面
   - 确认 `login` / `dash` 联调通过
3. 补 P1
   - metrics upload
   - dashboard 聚合
4. 补 P2
   - CAS 主协议
   - 访问控制和数据治理
5. 最后再处理“企业部署工件”
   - Dockerfile
   - compose / k8s 示例
   - 监控与日志

## 备注

这份文档的目的不是否定当前 `server/`，而是明确:

- 当前代码已经有一套可编译的 NestJS 服务骨架
- 但它与 `.docs` 里描述的企业部署目标之间，仍有明显实现差距

后续继续时，建议以本文件为 gap list，逐项关闭。
