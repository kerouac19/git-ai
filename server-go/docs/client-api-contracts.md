# Git-AI 客户端 HTTP 接口与参数

本文整理 Rust 客户端当前定义或发出的 HTTP 接口、参数和响应形态，供 `server-go` 私有化服务端实现、联调和回归检查使用。若 `src/api` 中定义了客户端 wrapper 但当前 `src/` 内未发现直接调用点，本文会单独标注。

核对来源：

- `src/api/client.rs`
- `src/api/types.rs`
- `src/api/bundle.rs`
- `src/api/cas.rs`
- `src/api/metrics.rs`
- `src/api/notes.rs`
- `src/auth/client.rs`
- `src/auth/types.rs`
- `src/commands/upgrade.rs`
- `src/commands/fetch_notes.rs`
- `src/commands/notes_migrate.rs`
- `src/commands/flush_metrics_db.rs`
- `src/daemon/telemetry_worker.rs`
- `src/metrics/types.rs`
- `src/metrics/events.rs`
- `src/metrics/attrs.rs`

核对日期：2026-05-14。

## 范围

本文只把 Rust 客户端源码作为依据，不从 `server-go` 已有实现反推协议。

`server-go` 需要重点覆盖的产品接口包括：

| 功能 | Method | Path | 主要调用方 |
| --- | --- | --- | --- |
| OAuth device code | `POST` | `/worker/oauth/device/code` | `git-ai login` |
| OAuth token exchange | `POST` | `/worker/oauth/token` | `git-ai login`、token refresh、`exchange-nonce` |
| Bundle create | `POST` | `/api/bundles` | `ApiClient::create_bundle` wrapper；当前 `src/` 内未发现直接调用点 |
| CAS upload | `POST` | `/worker/cas/upload` | daemon CAS flush |
| CAS read | `GET` | `/worker/cas/?hashes=...` | `ApiClient::read_ca_prompt_store` wrapper；当前 `src/` 内未发现直接调用点 |
| Metrics upload | `POST` | `/worker/metrics/upload` | daemon metrics flush、`flush-metrics-db` |
| Notes upload | `POST` | `/worker/notes/upload` | HTTP notes backend、`notes migrate`、daemon notes flush |
| Notes read | `GET` | `/worker/notes/?commits=...` | HTTP notes backend warm cache |
| Releases check | `GET` | `/worker/releases` | `git-ai upgrade` |
| Release artifact download | `GET` | `/worker/releases/:channel/download/:name` | `git-ai upgrade` |
| Dashboard URL | browser open | `{api_base_url}/me` | `git-ai dash` / `git-ai dashboard` |

附录列出 Rust 客户端中的外部第三方 HTTP 请求，例如 GitLab、JetBrains Marketplace、PostHog、Sentry；这些不是 `server-go` 产品接口。

## 基址与通用 Header

产品 API 默认基址来自 `Config::api_base_url`，默认值是：

```text
https://usegitai.com
```

`ApiContext` 拼接 endpoint 时会保留 base URL 上的 path prefix。例如：

```text
https://host/api/gitai + /worker/notes/upload
= https://host/api/gitai/worker/notes/upload
```

通过 `ApiContext` 发出的请求会附带：

| Header | 条件 | 值 |
| --- | --- | --- |
| `User-Agent` | 总是 | `git-ai/<CARGO_PKG_VERSION>` |
| `X-Distinct-ID` | 总是 | 本地 distinct id |
| `Authorization` | 本地 OAuth access token 可用时 | `Bearer <access_token>` |
| `X-API-Key` | 配置了 API key 时 | `<api_key>` |
| `X-Author-Identity` | 配置了 API key 且 git identity 可解析时 | 当前 git author identity |
| `Content-Type` | JSON `POST` | `application/json` |

服务端应同时支持 Bearer token 和 `X-API-Key` 两条认证路径。Release 下载接口当前由客户端用公共接口调用，服务端不应强制登录。

## OAuth

### 发起 device flow

```http
POST /worker/oauth/device/code
Content-Type: application/json
```

请求 body：

```json
{}
```

成功响应：

```ts
{
  device_code: string,
  user_code: string,
  verification_uri: string,
  verification_uri_complete?: string,
  expires_in: number,
  interval: number
}
```

客户端行为：

- `git-ai login` 打印 `verification_uri_complete`，如果不存在则打印 `verification_uri`。
- 客户端会尝试自动打开浏览器。
- `interval` 和 `expires_in` 控制后续 token polling。

### device code 换 token

```http
POST /worker/oauth/token
Content-Type: application/json
```

请求 body：

```ts
{
  grant_type: "urn:ietf:params:oauth:grant-type:device_code",
  device_code: string,
  client_id: "git-ai-cli"
}
```

成功响应：

```ts
{
  access_token: string,
  token_type: string,
  expires_in: number,
  refresh_token: string,
  refresh_expires_in: number
}
```

错误响应：

```ts
{
  error: string,
  error_description?: string
}
```

客户端显式处理这些 `error`：

| error | 客户端行为 |
| --- | --- |
| `authorization_pending` | 继续轮询 |
| `slow_down` | 轮询间隔增加 5 秒后继续 |
| `access_denied` | 登录失败 |
| `expired_token` | 登录失败，提示 device code 过期 |

### refresh token 换 token

```http
POST /worker/oauth/token
Content-Type: application/json
```

请求 body：

```ts
{
  grant_type: "refresh_token",
  refresh_token: string,
  client_id: "git-ai-cli"
}
```

成功 / 错误响应同上。`whoami`、其他 `ApiContext::new()` 路径在 access token 临近过期时可能触发该请求。

### install nonce 换 token

```http
POST /worker/oauth/token
Content-Type: application/json
```

请求 body：

```ts
{
  grant_type: "install_nonce",
  install_nonce: string,
  client_id: "git-ai-cli"
}
```

该请求由 `git-ai exchange-nonce` 触发。命令从环境变量读取：

- `INSTALL_NONCE`
- `API_BASE`

成功响应同 token response，客户端会把 credential 写入本地 credential store。

## Bundles

### 创建 bundle

```http
POST /api/bundles
Content-Type: application/json
Authorization: Bearer <access_token>
```

也可能通过 `X-API-Key` 认证；取决于 `ApiContext` 当前可用 credential。

当前状态：Rust `src/api/bundle.rs` 定义了 `ApiClient::create_bundle`，但当前 `src/` 内未发现直接调用点。`server-go` 仍应保留该接口以兼容客户端 API wrapper 和后续调用方。

请求 body：

```ts
{
  title: string,
  data: {
    prompts: {
      [prompt_id: string]: {
        agent_id: {
          tool: string,
          id: string,
          model: string
        },
        human_author?: string,
        messages: Message[],
        total_additions: number,
        total_deletions: number,
        accepted_lines: number,
        overriden_lines: number,
        messages_url?: string,
        custom_attributes?: { [key: string]: string }
      }
    },
    files?: {
      [path: string]: {
        annotations: {
          [prompt_hash: string]: Array<number | [number, number]>
        },
        diff: string,
        base_content: string
      }
    }
  }
}
```

`Message` 使用 `type` tagged enum，`type` 为 `snake_case`：

```ts
type Message =
  | { type: "user", text: string, timestamp?: string }
  | { type: "assistant", text: string, timestamp?: string }
  | { type: "thinking", text: string, timestamp?: string }
  | { type: "plan", text: string, timestamp?: string }
  | { type: "tool_use", name: string, input: unknown, timestamp?: string };
```

成功响应：

```ts
{
  success: boolean,
  id: string,
  url: string
}
```

400 / 500 错误响应：

```ts
{
  error: string,
  details?: unknown
}
```

## CAS

### 上传 CAS objects

```http
POST /worker/cas/upload
Content-Type: application/json
```

请求 body：

```ts
{
  objects: Array<{
    content: unknown,
    hash: string,
    metadata?: { [key: string]: string }
  }>
}
```

客户端约束和行为：

- daemon flush CAS 时每批最多上传 `50` 个 object。
- `metadata` 为空时序列化会省略该字段。

成功响应：

```ts
{
  results: Array<{
    hash: string,
    status: string,
    error?: string
  }>,
  success_count: number,
  failure_count: number
}
```

400 / 500 错误响应：

```ts
{
  error: string,
  details?: unknown
}
```

### 读取 CAS objects

```http
GET /worker/cas/?hashes=<comma-joined-hex-hashes>
```

当前状态：Rust `src/api/cas.rs` 定义了 `ApiClient::read_ca_prompt_store`，但当前 `src/` 内未发现直接调用点。该接口仍属于客户端 API wrapper 的契约。

Query 参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `hashes` | `string` | 多个 hash 以英文逗号拼接；客户端不 URL-encode 该列表 |

客户端约束和行为：

- 每个 hash 必须是 ASCII hex；非 hex 输入会在客户端本地失败，不发 HTTP 请求。
- 客户端代码注释标明每次最多读取 `100` 个 hash。
- `404` 被当作“全部未命中”，不会作为硬错误。

成功响应：

```ts
{
  results: Array<{
    hash: string,
    status: string,
    content?: unknown,
    error?: string
  }>,
  success_count: number,
  failure_count: number
}
```

`404` 等价响应：

```ts
{
  results: [],
  success_count: 0,
  failure_count: <requested_hash_count>
}
```

## Metrics

### 上传 metrics batch

```http
POST /worker/metrics/upload
Content-Type: application/json
```

请求 body：

```ts
{
  v: 1,
  events: Array<{
    t: number,
    e: number,
    v: { [position: string]: unknown },
    a: { [position: string]: unknown }
  }>
}
```

字段含义：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `v` | `number` | metrics wire format version，当前为 `1` |
| `events[].t` | `number` | Unix timestamp seconds |
| `events[].e` | `number` | event id |
| `events[].v` | sparse object | event-specific values，key 是 position 的字符串 |
| `events[].a` | sparse object | common attributes，key 是 position 的字符串 |

客户端行为：

- daemon flush metrics 时按 `1000` 条一批上传。
- `flush-metrics-db` 从本地 DB 取最多 `1000` 条事件后上传。
- `200` 且带 partial errors 时，客户端记录错误但不重试这些 validation error。
- `401` 被识别为 Unauthorized。
- 非 200 错误会按客户端 retry 逻辑处理。

成功响应：

```ts
{
  errors: Array<{
    index: number,
    error: string
  }>
}
```

通用 attribute positions：

| Position | Name | Type / 说明 |
| --- | --- | --- |
| `0` | `git_ai_version` | `string`，通常必填 |
| `1` | `repo_url` | `string` / `null` |
| `2` | `author` | `string` / `null` |
| `3` | `commit_sha` | `string` / `null` |
| `4` | `base_commit_sha` | `string` / `null` |
| `5` | `branch` | `string` / `null` |
| `20` | `tool` | `string` / `null` |
| `21` | `model` | `string` / `null` |
| `22` | `prompt_id` | tombstoned；只兼容历史 payload，不能复用 |
| `23` | `external_session_id` | `string` / `null` |
| `24` | `session_id` | `string`，新 payload 通常应有 |
| `25` | `trace_id` | `string` / `null` |
| `26` | `parent_session_id` | `string` / `null` |
| `27` | `external_parent_session_id` | `string` / `null` |
| `30` | `custom_attributes` | JSON string / `null` |

Event IDs：

| Event ID | Name | 说明 |
| --- | --- | --- |
| `1` | `committed` | 提交完成后的 authorship / diff 统计 |
| `2` | `agent_usage` | AI checkpoint / agent 使用记录 |
| `3` | `install_hooks` | `install-hooks` 时每个工具的安装结果 |
| `4` | `checkpoint` | 每个 checkpoint 文件的行数和上下文 |
| `5` | `session_event` | agent session 原始事件 |

`committed` values：

| Position | Name | Type / 说明 |
| --- | --- | --- |
| `0` | `human_additions` | `u32` |
| `1` | `git_diff_deleted_lines` | `u32` |
| `2` | `git_diff_added_lines` | `u32` |
| `3` | `tool_model_pairs` | `string[]` |
| `4` | removed | 不再使用 |
| `5` | `ai_additions` | `u32[]` |
| `6` | `ai_accepted` | `u32[]` |
| `7` | removed | 不再使用 |
| `8` | removed | 不再使用 |
| `9` | removed | 不再使用 |
| `10` | `first_checkpoint_ts` | `u64` / `null` |
| `11` | `commit_subject` | `string` |
| `12` | `commit_body` | `string` / `null` |
| `13` | `authorship_note` | `string`，完整 serialized authorship note |
| `14` | `hunks` | `string`，JSON array string |

`agent_usage` values 当前为空 sparse map：

```json
{}
```

`install_hooks` values：

| Position | Name | Type / 说明 |
| --- | --- | --- |
| `0` | `tool_id` | `string` |
| `1` | `status` | `string`，例如 `not_found` / `installed` / `already_installed` / `failed` |
| `2` | `message` | `string` / `null` |

`checkpoint` values：

| Position | Name | Type / 说明 |
| --- | --- | --- |
| `0` | `checkpoint_ts` | `u64` |
| `1` | `kind` | `string`，例如 `human` / `ai_agent` / `ai_tab` |
| `2` | `file_path` | `string` |
| `3` | `lines_added` | `u32` |
| `4` | `lines_deleted` | `u32` |
| `5` | `lines_added_sloc` | `u32` |
| `6` | `lines_deleted_sloc` | `u32` |
| `7` | `external_tool_use_id` | `string` / `null` |
| `8` | `edit_kind` | `string` / `null`，例如 `file_edit` / `bash` |

`session_event` values：

| Position | Name | Type / 说明 |
| --- | --- | --- |
| `0` | `raw_json` | JSON value |
| `1` | `external_event_id` | `string` / omitted |
| `2` | `external_parent_event_id` | `string` / omitted |
| `3` | `external_tool_use_id` | `string` / omitted |

## HTTP Authorship Notes Backend

这些接口只有在配置 HTTP notes backend 时才会调用：

```bash
git-ai config set notes_backend.kind http
git-ai config set notes_backend.backend_url <base-url>
```

`notes_backend.backend_url` 可以和主 API 相同，也可以是单独服务。认证仍使用 `ApiContext` 的标准 headers。

### 上传 notes

```http
POST /worker/notes/upload
Content-Type: application/json
```

请求 body：

```ts
{
  entries: Array<{
    commit_sha: string,
    content: string
  }>
}
```

客户端行为：

- daemon notes flush 每次 dequeue 最多 `50` 条 pending notes。
- `git-ai notes migrate` 也按 `50` 条一批上传。
- 上传前要求已登录或配置 API key；否则跳过或报错。

成功响应：

```ts
{
  success_count: number,
  failure_count: number
}
```

400 错误响应：

```ts
{
  error: string,
  details?: unknown
}
```

### 读取 notes

```http
GET /worker/notes/?commits=<comma-joined-hex-shas>
```

Query 参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `commits` | `string` | 多个 commit SHA 以英文逗号拼接 |

客户端约束和行为：

- 每个 commit SHA 必须是 hex；非 hex 输入会在客户端本地失败。
- `warm_cache_for_remote` 按 `100` 个 SHA 一批读取。
- `404` 被视为成功但无 notes。
- 其他非 200 状态会被视为读取错误；warm-cache 路径把它当作 cache miss 处理。

成功响应：

```ts
{
  notes: { [commit_sha: string]: string }
}
```

`404` 等价响应：

```ts
{
  notes: {}
}
```

## Releases / Upgrade

### 查询 release channels

```http
GET /worker/releases
```

请求 body / query：无。

成功响应：

```ts
{
  channels: {
    [channel: string]: {
      version: string,
      checksum: string
    }
  }
}
```

客户端当前已知 channel：

| Channel | 说明 |
| --- | --- |
| `latest` | 稳定版 |
| `next` | 下一版 / 预览版 |
| `enterprise-latest` | 企业稳定版 |
| `enterprise-next` | 企业下一版 / 预览版 |

客户端会：

- 从 `channels[update_channel]` 读取 `version` 作为 release tag。
- 从 `channels[update_channel]` 读取 `checksum`，用于校验 `SHA256SUMS` 原始 bytes。

### 下载 SHA256SUMS

```http
GET /worker/releases/:channel/download/SHA256SUMS
```

Path 参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `channel` | `string` | 当前 update channel |

成功响应：raw bytes。客户端要求内容是 UTF-8 文本，格式为：

```text
<sha256 hash>  <filename>
```

注意两个空格是解析分隔符。客户端会先用 release response 中的 `checksum` 校验整份 `SHA256SUMS` raw bytes，再解析每个 filename 的 hash。

### 下载安装脚本

```http
GET /worker/releases/:channel/download/:script_name
```

Path 参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `channel` | `string` | 当前 update channel |
| `script_name` | `"install.sh"` / `"install.ps1"` | Unix 使用 `install.sh`；Windows 使用 `install.ps1` |

成功响应：raw bytes，内容必须是 UTF-8 script text。

客户端会用 `SHA256SUMS` 中同名文件的 hash 校验脚本 raw bytes，校验通过后才执行。

## Dashboard URL

`git-ai dash` / `git-ai dashboard` 不调用 JSON API，而是打开浏览器：

```text
{api_base_url}/me
```

该页面通常由 SPA / nginx static hosting 提供。Go API 侧需要支撑页面背后的 `/api/me`、dashboard、metrics 等接口，但这些接口不是 Rust CLI 直接调用的 HTTP API。

## 外部第三方 HTTP 请求

以下请求存在于 Rust 客户端源码中，但目标不是 Git AI product API，也不应由 `server-go` 实现。

### GitLab Merge Requests

```http
GET ${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/merge_requests
```

Query 参数：

```ts
{
  state: "merged",
  updated_after: string,
  order_by: "updated_at",
  sort: "desc",
  per_page: 100
}
```

认证 header：

```ts
{ "PRIVATE-TOKEN": string }
```

或：

```ts
{ "JOB-TOKEN": string }
```

取决于 `GITLAB_TOKEN` / `CI_JOB_TOKEN` 环境变量。

### JetBrains Marketplace plugin download

```http
GET https://plugins.jetbrains.com/pluginManager
```

Query 参数：

```ts
{
  action: "download",
  id: string,
  build: `${product_code}-${build_number}`
}
```

成功响应是 raw ZIP bytes；`404` 被视为 plugin not found。

### PostHog Capture

```http
POST ${POSTHOG_HOST}/capture/
Content-Type: application/json
```

默认 host：

```text
https://us.i.posthog.com
```

请求 body：

```ts
{
  api_key: string,
  event: string,
  properties: {
    os: string,
    arch: string,
    version: string,
    message: string,
    level: string,
    [context_key: string]: unknown
  },
  distinct_id: string,
  timestamp: string
}
```

客户端忽略响应内容。

### Sentry Store

```http
POST {scheme}://{host}/api/{project_id}/store/
Content-Type: application/json
X-Sentry-Auth: <auth>
```

请求 body：

```ts
{
  message: string,
  level: string,
  timestamp: string,
  platform: "other",
  tags: { [key: string]: unknown },
  extra: { [key: string]: unknown },
  release: string
}
```

任意 `2xx` 被视为成功。

## 维护建议

每次同步 Rust 客户端后，用以下命令复核接口是否变化：

```bash
rg '"/worker|"/api|worker/|api/' src/api src/auth src/commands src/daemon src/git src/ci src/mdm
rg 'post_json|get\(|http_get|http_post|send_with_body|send\(' src
rg 'grant_type|device_code|install_nonce|refresh_token' src/auth
rg 'MetricEventId|attr_pos|committed_pos|checkpoint_pos|session_event_pos' src/metrics
```

如果 endpoint、request body、response body、headers 或 metrics sparse position 发生变化，应同步更新本文档和 `server-go` 对应 handler/service/test。
