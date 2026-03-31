# Git-AI 私有化部署拆解: `git-ai login` / `git-ai dash`

## 当前实现状态（2026-03-27 晚）

这份文档主要描述目标闭环和推荐实施顺序。结合当天后续实现推进，当前仓库状态可先更新如下:

- P0 已基本跑通:
  - `POST /worker/oauth/device/code`
  - 浏览器 `/oauth/device` 授权页
  - `/oauth/device/approve` / `/oauth/device/deny`
  - `POST /worker/oauth/token`
  - `GET /me` HTML dashboard 页面
- P1 已补到最小可用:
  - `POST /worker/metrics/upload`
  - `/me` 上的 `last_sync_at`、`event_count_7d`、`repo_count_7d`
- P2 的主协议兼容已补齐:
  - `POST /worker/cas/upload` JSON 批量上传
  - `GET /worker/cas?hashes=...` 批量读取
- 已新增 smoke 脚本 [scripts/test-private-flow.sh](/Users/hg/git/git-ai/scripts/test-private-flow.sh#L1)，并已实际跑通:
  - `device code -> approve -> token -> metrics upload -> /me -> cas upload/read`

仍未完成的主要边界:

- 外部 IdP / SSO
- 多实例 HA 与更完整的共享状态治理
- `whoami` 所需的更丰富 identity claims
- CAS 权限治理与组织级数据模型
- Prometheus / 监控接入
- 更完整的 Compose / Kubernetes 交付物
- `git commit` 后自动把 authorship/commit attribution 同步到 `/api/authorship/*` 的客户端链路

当前联调结论需要单独强调一条:

- 现在已经实际验证通过的是 `login -> approve -> token -> metrics -> /me -> CAS`
- `refs/notes/ai` 和 `messages_url -> CAS` 会正常生成并最终入库
- 但 `authorship_records` / `commit_attributions` 目前不会因为一次普通 `git commit` 自动出现在 server 数据库里
- 因此，若要检查当前私有化 server 是否工作，应该优先看 `oauth_device_codes`、`metrics_events`、`audit_logs`、`cas_entries`，而不是把 authorship 表为空直接判定为 server 故障

## 1. 目标

本文档面向私有化部署，拆解支持以下能力所需的最小闭环和完整闭环:

- `git-ai login`
- `git-ai dash`
- 登录后的指标同步
- Dashboard 基础可用性
- Transcript / CAS 能力接入

本文档按实施顺序和优先级整理，适合后端、平台、前端协作落地。

## 2. 结论先行

`git-ai login` 和 `git-ai dash` 的 CLI 逻辑都很薄:

- `git-ai login` 的核心是 OAuth Device Flow
- `git-ai dash` 的核心是打开 `${api_base_url}/me`

因此，私有化部署的重点不在 CLI，而在服务端:

- OAuth 设备授权
- Token 签发与刷新
- `/me` Web 页面
- Metrics 接收与聚合
- CAS prompt store

## 3. 信息边界

### 3.1 当前仓库已确认

以下内容可以从当前客户端仓库直接确认:

- CLI 命令入口与行为
- OAuth 请求路径和字段
- Token 刷新机制
- `/me` 页面路径
- Metrics 上传接口
- CAS 上传/读取接口

### 3.2 当前仓库无法直接确认

以下内容当前无法从源码直接确认:

- `/me` 页面内部展示字段
- Dashboard 前端路由结构
- 服务端数据库模型
- 组织级 dashboard 的完整页面实现

原因是服务端已迁到独立仓库，见 [src/server/README.md](/Users/hg/git/git-ai/src/server/README.md#L1)。

## 4. 源码依据

- `git-ai login`: [src/commands/login.rs](/Users/hg/git/git-ai/src/commands/login.rs#L5)
- `git-ai dash`: [src/commands/personal_dashboard.rs](/Users/hg/git/git-ai/src/commands/personal_dashboard.rs#L3)
- OAuth client: [src/auth/client.rs](/Users/hg/git/git-ai/src/auth/client.rs#L7)
- OAuth types: [src/auth/types.rs](/Users/hg/git/git-ai/src/auth/types.rs#L44)
- Token identity claims: [src/auth/identity.rs](/Users/hg/git/git-ai/src/auth/identity.rs#L20)
- API auth headers and refresh: [src/api/client.rs](/Users/hg/git/git-ai/src/api/client.rs#L14)
- Metrics upload: [src/api/metrics.rs](/Users/hg/git/git-ai/src/api/metrics.rs#L101)
- CAS upload/read: [src/api/cas.rs](/Users/hg/git/git-ai/src/api/cas.rs#L7)
- `api_base_url` / `api_key` config: [src/config.rs](/Users/hg/git/git-ai/src/config.rs#L313)

## 5. 通用协议约束

### 5.1 通用请求头

客户端对所有 HTTP 请求都会附带:

- `User-Agent: git-ai/<version>`
- `X-Distinct-ID: <stable-client-id>`

如果 CLI 当前持有 OAuth access token，还会附带:

- `Authorization: Bearer <access_token>`

如果配置了 API key，还会附带:

- `X-API-Key: <api_key>`
- `X-Author-Identity: <git committer ident>`

实现要求:

- 服务端必须接受 Bearer token 认证
- 如启用 API key 模式，服务端必须接受 `X-API-Key`
- 如启用 API key 模式，建议将 `X-Author-Identity` 记录入审计日志

注意:

- 当前客户端没有单独的 `Accept` 协议约束
- 当前客户端 timeout 默认为 30 秒

### 5.2 统一错误响应格式

客户端已确认会消费两类错误格式:

OAuth 错误:

```json
{
  "error": "authorization_pending",
  "error_description": "optional"
}
```

通用 API 错误:

```json
{
  "error": "Invalid request body",
  "details": {}
}
```

建议:

- OAuth 相关接口使用 OAuth 错误格式
- 其余 JSON API 使用通用 API 错误格式

### 5.3 认证模式

建议文档和实现都明确区分两种模式:

- 用户登录模式
  - 依赖 OAuth Device Flow
  - 适合 `git-ai login` 和 `git-ai dash`
- 机器/API key 模式
  - 依赖 `X-API-Key`
  - 适合自动化或内网集成

注意:

- `git-ai dash` 面向用户登录场景
- API key 模式能否访问 `/me`，当前客户端源码未定义，建议服务端明确禁止或重定向到 API key 说明页

## 6. 按顺序的实施拆解

### P0: 跑通 `login` 和 `dash`

这是最小可用闭环。没有这一层，私有化部署无法完成认证，也无法打开 dashboard。

#### P0.1 私有域名和 HTTPS

要求:

- 配置 `GIT_AI_API_BASE_URL` 或配置文件中的 `api_base_url`
- 生产环境必须使用 `https://`

原因:

- release 构建下 OAuth base URL 非 HTTPS 会被拒绝

交付物:

- 一个可被 CLI 和浏览器同时访问的私有域名
- TLS 证书和反向代理配置

验收:

- `git-ai whoami` 输出的 `API Base URL` 指向私有地址
- `git-ai login` 不因 URL scheme 被拒绝

#### P0.2 Device Flow: `POST /worker/oauth/device/code`

用途:

- 启动设备授权流程

客户端行为:

- `git-ai login` 调用该接口
- 成功后打印 URL 和 `user_code`
- 自动尝试打开浏览器

请求:

```json
{}
```

响应:

```json
{
  "device_code": "dev_xxx",
  "user_code": "ABCD-EFGH",
  "verification_uri": "https://git-ai.example.com/device",
  "verification_uri_complete": "https://git-ai.example.com/device?user_code=ABCD-EFGH",
  "expires_in": 600,
  "interval": 5
}
```

实现要求:

- 为 `device_code` 建立服务端状态
- 支持后续浏览器确认授权
- 返回 RFC 8628 兼容字段

建议服务端持久化字段:

- `device_code`
- `user_code`
- `client_id`
- `verification_uri`
- `status`
- `user_id`
- `created_at`
- `expires_at`
- `approved_at`
- `denied_at`
- `last_polled_at`

验收:

- `git-ai login` 能打印授权 URL
- 浏览器能打开授权页

#### P0.3 浏览器授权页

用途:

- 用户在浏览器完成设备授权

建议页面:

- 显示当前登录用户
- 显示待授权 CLI / device
- 确认授权 / 拒绝授权

最小能力:

- 能把 `device_code` 从 pending 改为 approved 或 denied

建议最小页面字段:

- 当前登录用户
- 待授权客户端名称
- `user_code`
- 过期时间
- 授权按钮
- 拒绝按钮

建议状态机:

1. `pending`
2. `approved`
3. `denied`
4. `expired`

建议页面行为:

- 非法或过期 `user_code` 给出明确错误
- 重复授权已 `approved` 的 code 时给出幂等提示
- `expired` 不允许再授权

验收:

- 用户在浏览器确认后，CLI 轮询能收到成功结果

#### P0.4 Token 接口: `POST /worker/oauth/token`

用途:

- 轮询设备授权结果
- 刷新 access token
- 安装脚本自动登录

必须支持的 `grant_type`:

- `urn:ietf:params:oauth:grant-type:device_code`
- `refresh_token`
- `install_nonce`

设备授权轮询请求:

```json
{
  "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
  "device_code": "dev_xxx",
  "client_id": "git-ai-cli"
}
```

刷新请求:

```json
{
  "grant_type": "refresh_token",
  "refresh_token": "refresh_xxx",
  "client_id": "git-ai-cli"
}
```

自动登录请求:

```json
{
  "grant_type": "install_nonce",
  "install_nonce": "nonce_xxx",
  "client_id": "git-ai-cli"
}
```

成功响应:

```json
{
  "access_token": "jwt_or_token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "refresh_xxx",
  "refresh_expires_in": 7776000
}
```

错误响应:

```json
{
  "error": "authorization_pending",
  "error_description": "optional"
}
```

必须兼容的错误码:

- `authorization_pending`
- `slow_down`
- `access_denied`
- `expired_token`

建议状态码:

- `200` 成功返回 token
- `400` OAuth 业务错误，如 `authorization_pending`
- `401` 无效 client 或无效签名
- `500` 服务端错误

建议实现规则:

- `device_code` 未授权时返回 `authorization_pending`
- 轮询过快时返回 `slow_down`
- 用户拒绝时返回 `access_denied`
- code 过期时返回 `expired_token`
- `refresh_token` 失效时返回可读错误，便于 CLI 将其视为未登录
- `install_nonce` 失败时建议服务端记录审计和失败原因

验收:

- 授权前返回 `authorization_pending`
- 授权后返回 access token 和 refresh token
- refresh token 可正常换新 access token

#### P0.5 Token 设计

建议 access token 使用 JWT，并带上 identity claims，供 CLI `whoami` 直接解析:

```json
{
  "sub": "user_123",
  "email": "user@example.com",
  "name": "Alice",
  "personal_org_id": "org_personal_123",
  "orgs": [
    {
      "org_id": "org_1",
      "org_name": "Example Org",
      "org_slug": "example-org",
      "role": "owner"
    }
  ]
}
```

建议时效:

- access token: 1 hour
- refresh token: 90 days

最低建议 claims:

- `sub`
- `email`
- `name`
- `personal_org_id`
- `orgs`

建议额外 claims:

- `iss`
- `aud`
- `iat`
- `exp`

验收:

- `git-ai whoami` 能显示用户、邮箱、组织信息
- access token 过期后自动 refresh 成功

#### P0.6 Dashboard 页面: `GET /me`

用途:

- `git-ai dash` 只会打开这个页面

最小要求:

- 页面存在
- 已登录用户可访问
- 可显示基础用户信息或“dashboard 正在建设中”

源码已确认:

- CLI 只会打开 `/me`
- 当前客户端仓库中没有 `/me` 的 JSON 数据接口定义

建议最小页面契约:

- 方式 A: 直接 SSR 页面，无额外 JSON API
- 方式 B: 页面加载后请求一个内部接口，如 `/api/me/dashboard`

建议最小页面数据:

- user summary
- org summary
- last metrics sync timestamp
- basic counts

建议最小 SSR / JSON 数据结构:

```json
{
  "user": {
    "id": "user_123",
    "email": "user@example.com",
    "name": "Alice"
  },
  "org": {
    "id": "org_1",
    "slug": "example-org",
    "name": "Example Org"
  },
  "stats": {
    "event_count_7d": 42,
    "repo_count_7d": 3,
    "last_sync_at": "2026-03-27T12:34:56Z"
  }
}
```

建议最小内容:

- 当前用户
- 所属组织
- 最近一次 metrics 接收状态

验收:

- 执行 `git-ai dash` 后能打开私有站点个人页

### P1: 让 dashboard 有数据

P0 跑通后，`dash` 只是能打开页面。P1 的目标是让页面不空。

#### P1.1 Metrics 接口: `POST /worker/metrics/upload`

用途:

- 接收 CLI 本地缓存指标
- 为 dashboard 提供基础数据源

请求体:

```json
{
  "v": 1,
  "events": [
    {
      "t": 1700000000,
      "e": 1,
      "v": { "0": 123 },
      "a": { "0": "1.2.3" }
    }
  ]
}
```

响应体:

```json
{
  "errors": []
}
```

实现要求:

- 存储原始 metrics event
- 允许部分校验失败
- 成功时返回 200
- 局部失败通过 `errors[index]` 返回

客户端特性:

- 登录成功后如果本地有积压 metrics，会后台触发上传
- 单批最大 250 条
- 上传失败会重试

状态码矩阵:

- `200` 上传成功，允许 `errors[]` 非空
- `400` 请求体非法
- `401` 未授权
- `500` 服务端错误

`errors[]` 语义:

- index 指向本次 `events` 数组里的事件下标
- 出现在 `errors[]` 中的事件视为校验失败
- 未出现在 `errors[]` 中的事件视为服务端已接收

建议服务端行为:

- 尽量做部分成功，不要因为单条脏数据打回整批
- 返回 200 + `errors[]` 时，客户端会视为整次调用成功，不会重试该批
- 因此 `errors[]` 只应承载“重试无意义”的校验失败

#### P1.1.1 Metrics 字段字典

Metrics wire format:

- 顶层 `v`: metrics schema version
- 顶层 `events`: 事件数组
- 单条事件:
  - `t`: unix timestamp
  - `e`: event id
  - `v`: event-specific sparse values
  - `a`: shared sparse attributes

公共 attributes `a` 字段:

| Position | Name | Type | Required |
|----------|------|------|----------|
| 0 | `git_ai_version` | String | Yes |
| 1 | `repo_url` | String | No |
| 2 | `author` | String | No |
| 3 | `commit_sha` | String | No |
| 4 | `base_commit_sha` | String | No |
| 5 | `branch` | String | No |
| 20 | `tool` | String | No |
| 21 | `model` | String | No |
| 22 | `prompt_id` | String | No |
| 23 | `external_prompt_id` | String | No |
| 30 | `custom_attributes` | String(JSON) | No |

事件类型:

- `e = 1`: committed
- `e = 2`: agent_usage
- `e = 3`: install_hooks
- `e = 4`: checkpoint

`committed` 的 `v` 字段:

| Position | Name | Type |
|----------|------|------|
| 0 | `human_additions` | u32 |
| 1 | `git_diff_deleted_lines` | u32 |
| 2 | `git_diff_added_lines` | u32 |
| 3 | `tool_model_pairs` | `Vec<String>` |
| 4 | `mixed_additions` | `Vec<u32>` |
| 5 | `ai_additions` | `Vec<u32>` |
| 6 | `ai_accepted` | `Vec<u32>` |
| 7 | `total_ai_additions` | `Vec<u32>` |
| 8 | `total_ai_deletions` | `Vec<u32>` |
| 9 | `time_waiting_for_ai` | `Vec<u64>` |
| 10 | `first_checkpoint_ts` | u64 |
| 11 | `commit_subject` | String |
| 12 | `commit_body` | String |

说明:

- position 3-9 是并行数组
- index 0 为总体聚合
- index 1+ 为 tool/model 维度拆分

`agent_usage` 的 `v` 字段:

- 空对象 `{}`
- 信息主要来自公共 attributes，例如 `tool`、`model`、`prompt_id`

`install_hooks` 的 `v` 字段:

| Position | Name | Type |
|----------|------|------|
| 0 | `tool_id` | String |
| 1 | `status` | String |
| 2 | `message` | String nullable |

`checkpoint` 的 `v` 字段:

| Position | Name | Type |
|----------|------|------|
| 0 | `checkpoint_ts` | u64 |
| 1 | `kind` | String |
| 2 | `file_path` | String |
| 3 | `lines_added` | u32 |
| 4 | `lines_deleted` | u32 |
| 5 | `lines_added_sloc` | u32 |
| 6 | `lines_deleted_sloc` | u32 |

`checkpoint.kind` 当前可见值:

- `human`
- `ai_agent`
- `ai_tab`

建议服务端存储字段:

- tenant / org
- user
- distinct_id
- event_id
- timestamp
- values
- attrs
- received_at

验收:

- 登录完成后服务端能收到 metrics
- `/me` 页面能显示基础统计

#### P1.2 `/me` 基础 dashboard

建议先实现个人维度，不要一开始就做组织级复杂报表。

建议首版内容:

- 今日/近 7 天事件量
- 最近使用的 agent / model
- 最近活跃 repo 数
- 最新同步时间

建议后端数据源:

- 若走 SSR，可在服务端直接聚合渲染
- 若走前后端分离，建议提供内部接口 `/api/me/dashboard`

建议 `/api/me/dashboard` 最小响应:

```json
{
  "user": {
    "id": "user_123",
    "email": "user@example.com",
    "name": "Alice"
  },
  "org": {
    "id": "org_1",
    "slug": "example-org",
    "name": "Example Org"
  },
  "stats": {
    "event_count_today": 10,
    "event_count_7d": 42,
    "repo_count_7d": 3,
    "top_tools_7d": [
      { "tool": "codex", "count": 20 },
      { "tool": "cursor", "count": 12 }
    ],
    "top_models_7d": [
      { "model": "gpt-5", "count": 18 }
    ],
    "last_sync_at": "2026-03-27T12:34:56Z"
  }
}
```

说明:

- 这是建议契约，不是当前客户端源码已确认的接口
- 当前 CLI 只关心 `/me` 页面本身，不依赖该 JSON API

验收:

- 新用户登录后 5 分钟内能在 `/me` 看到自己的基础数据

#### P1.3 `whoami` 联调支持

虽然不是新接口，但建议把以下排障能力纳入联调流程:

- 能从 Bearer token 识别用户
- 能从 JWT claims 显示组织
- 能区分 logged out / logged in / refresh expired

验收:

- `git-ai whoami` 输出与服务端用户信息一致

### P2: Transcript / CAS 和完整私有化体验

P2 不是 `login`/`dash` 的最小依赖，但如果你要完整私有化，一般必须做。

#### P2.1 CAS 上传: `POST /worker/cas/upload`

用途:

- 存储 prompt / transcript 等内容寻址对象

请求体:

```json
{
  "objects": [
    {
      "hash": "sha256_xxx",
      "content": { "messages": [] },
      "metadata": {
        "kind": "transcript"
      }
    }
  ]
}
```

响应体:

```json
{
  "results": [
    {
      "hash": "sha256_xxx",
      "status": "ok"
    }
  ],
  "success_count": 1,
  "failure_count": 0
}
```

实现要求:

- 按 hash 幂等写入
- 支持批量写入
- 支持单对象失败回报

状态码矩阵:

- `200` 批量结果返回
- `400` 请求体非法
- `401` 或 `403` 未授权
- `500` 服务端错误

#### P2.2 CAS 读取: `GET /worker/cas/?hashes=...`

用途:

- 按 hash 批量读取 transcript / prompt

响应体:

```json
{
  "results": [
    {
      "hash": "sha256_xxx",
      "status": "ok",
      "content": { "messages": [] }
    }
  ],
  "success_count": 1,
  "failure_count": 0
}
```

实现要求:

- 支持批量读取
- 支持 not found
- 做好访问控制

状态码矩阵:

- `200` 返回部分或全部结果
- `404` 当前客户端会把“全部未找到”当作空结果处理
- `401` 或 `403` 未授权
- `500` 服务端错误

建议读取规则:

- 支持部分存在、部分不存在
- 对不存在对象可在单条 result 中返回 `status=error`
- 若选择 `404`，仅用于“全部 hash 均不存在”

#### P2.3 数据治理

README 提到私有/self-hosted transcript store 应具备:

- access control
- secret redaction
- PII filtering

建议实现顺序:

1. 访问控制
2. 服务端静态加密
3. 上传前/入库前脱敏
4. 审计日志

#### P2.4 Dashboard 扩展

这部分是 README 能力方向，不是当前仓库直接证实的页面实现。

建议后续扩展:

- AI code composition
- 接受率 / 重写率 / durability
- agent / model comparison
- repo / PR / org 聚合视图

## 7. 服务端开发任务拆解

### 后端

P0:

- 实现 `POST /worker/oauth/device/code`
- 实现 `POST /worker/oauth/token`
- 实现 refresh token 流程
- 实现 install nonce 流程
- 生成带 identity claims 的 access token
- 实现 `/me` 页面鉴权

P1:

- 实现 `POST /worker/metrics/upload`
- 建立 metrics 原始表和聚合表
- 为 `/me` 提供个人 dashboard 查询接口或 SSR 数据源

P2:

- 实现 `POST /worker/cas/upload`
- 实现 `GET /worker/cas/?hashes=...`
- 实现 CAS 权限控制和脱敏处理

### 前端

P0:

- 设备授权页
- `/me` 最小页面

P1:

- `/me` 基础个人 dashboard

P2:

- transcript 浏览页
- 组织级 dashboard

### 平台/运维

P0:

- 配置 HTTPS
- 配置私有域名
- 配置 secret 管理
- 配置 session / token signing key

P1:

- metrics 存储和保留策略
- dashboard 基础监控

P2:

- CAS 存储
- 审计日志
- 数据备份和恢复

## 8. 接口状态码总表

### 8.1 `POST /worker/oauth/device/code`

- `200`: 成功返回 device flow 参数
- `400`: 非法请求
- `401` / `403`: 不允许该 client 发起 device flow
- `500`: 服务端错误

### 8.2 `POST /worker/oauth/token`

- `200`: 成功返回 token
- `400`: OAuth 业务错误，返回 `error` / `error_description`
- `401`: 非法 client 或签名错误
- `500`: 服务端错误

### 8.3 `GET /me`

- `200`: 页面可访问
- `302`: 未登录时跳登录页，若你的 Web 采用 session
- `401`: 未登录，若你的 Web 采用纯 API 风格
- `403`: 已登录但无访问权限

### 8.4 `POST /worker/metrics/upload`

- `200`: 成功，允许部分校验错误
- `400`: 请求体非法
- `401`: 未授权
- `500`: 服务端错误

### 8.5 `POST /worker/cas/upload`

- `200`: 返回批量写入结果
- `400`: 请求体非法
- `401` / `403`: 未授权
- `500`: 服务端错误

### 8.6 `GET /worker/cas/?hashes=...`

- `200`: 返回查询结果
- `404`: 全部 hash 均不存在
- `401` / `403`: 未授权
- `500`: 服务端错误

## 9. 推荐验收顺序

### Stage 1: 登录闭环

- `git-ai login` 能拿到授权 URL
- 浏览器授权后 CLI 登录成功
- `git-ai whoami` 能显示身份
- access token 过期后自动 refresh

### Stage 2: Dashboard 可打开

- `git-ai dash` 能打开 `/me`
- `/me` 对已登录用户可访问

### Stage 3: Dashboard 有数据

- 登录后 metrics 自动上传
- `/me` 能显示个人统计

### Stage 4: 完整私有化

- CAS 上传成功
- CAS 读取成功
- transcript 可受控访问

## 10. 不建议一开始就做的内容

以下内容建议推迟到 P2 或之后:

- 组织级复杂 BI 报表
- PR / 仓库 / 组织三层大盘一次性齐做
- 复杂 RBAC 细粒度策略
- transcript 全文搜索
- durability / incident 关联分析

原因:

- 这些能力不影响 `login` / `dash` 跑通
- 它们依赖 metrics、CAS 和权限模型稳定后再做更合理

## 11. 最终建议

如果你的目标是“需要支持这些功能”，建议按下面路线推进:

1. 先做 P0，确保 CLI 登录和 `/me` 打开完全可用
2. 再做 P1，让 dashboard 至少有个人数据
3. 最后做 P2，把 transcript / CAS 和更完整的私有能力补齐

这样能最快交付可验证成果，同时避免一开始就在大而全的 dashboard 上投入过多。
