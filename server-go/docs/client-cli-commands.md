# Git-AI 客户端命令与服务端关系

本文记录当前 Rust 客户端 `git-ai` 命令入口，以及这些命令与 `server-go` / Git AI API / 本机后台服务之间的关系。

核对来源：

- `src/commands/git_ai_handlers.rs`
- `src/commands/login.rs`
- `src/commands/logout.rs`
- `src/commands/whoami.rs`
- `src/commands/personal_dashboard.rs`
- `src/auth/client.rs`
- `src/api/*`
- `src/commands/upgrade.rs`
- `src/commands/fetch_notes.rs`
- `src/commands/notes_migrate.rs`
- `src/commands/flush_metrics_db.rs`

核对日期：2026-05-14。

## 术语

本文把“服务端相关”分成两类：

- **Git AI API / server-go**：远端或私有化部署的 HTTP 服务，例如 OAuth、Dashboard、Metrics、CAS、Releases、HTTP notes backend。
- **本机后台服务**：客户端本机 daemon，由 `git-ai bg` 管理；很多 `git-ai` 命令执行前需要它可连接。

## 公开命令

`git-ai help` 当前公开列出的命令如下：

| 命令 | 用途 | 服务端关系 |
| --- | --- | --- |
| `checkpoint` | 对工作区变更做 checkpoint，并记录作者归因 | 本机 daemon 必需；运行后可能经 daemon 异步上传 Metrics / CAS / HTTP notes |
| `log` | 显示带 AI authorship notes 的提交日志 | 本机 daemon 必需；主要读本地 git notes / authorship 数据 |
| `blame` | 在 `git blame` 结果上叠加 AI authorship 信息 | 本机 daemon 必需；主要读本地 git notes / authorship 数据 |
| `diff` | 显示带 AI authorship 标注的 diff | 本机 daemon 必需；主要读本地 git notes / authorship 数据 |
| `stats` | 显示指定提交的 AI authorship 统计 | 本机 daemon 必需；主要读本地 git notes / authorship 数据 |
| `status` | 显示未提交变更的 AI authorship 状态 | 本机 daemon 必需；读本地 working log / attribution 状态 |
| `show` | 显示指定 revision 或 range 的 authorship log | 本机 daemon 必需；当 `notes_backend.kind=http` 时，底层读取可能命中 HTTP notes cache / backend |
| `show-prompt` | 按 ID 显示 prompt 记录 | 本机 daemon 必需；可能读取本地 DB / notes / CAS-backed prompt 内容 |
| `config` | 查看和管理 git-ai 配置 | 不要求本机 daemon；本地配置读写 |
| `debug` | 输出支持 / 排障用诊断信息 | 不要求本机 daemon；诊断信息为主 |
| `bg` / `d` / `daemon` | 运行和控制 git-ai 本机后台服务 | 管理本机后台服务 |
| `install-hooks` / `install` | 安装 git hooks / agent hooks | 不要求本机 daemon；安装后会尝试重启本机 daemon |
| `uninstall-hooks` | 移除 git-ai hooks | 不要求本机 daemon |
| `ci` | CI 辅助工具 | 本机 daemon 必需；主要用于 CI 内重写 / 保留 authorship 数据 |
| `squash-authorship` | 为 squash 后的提交生成 authorship log | 本机 daemon 必需；主要读写本地 git notes |
| `git-path` | 输出底层 git 可执行文件路径 | 本机 daemon 必需；本地配置查询 |
| `upgrade` | 检查并安装可用更新 | 直接访问 Git AI API release endpoints |
| `fetch-notes` | 同步拉取 AI authorship notes | 本机 daemon 必需；默认走 git remote notes，HTTP notes backend 开启时访问 server-go |
| `login` | 登录 Git AI | 直接访问 Git AI API OAuth endpoints |
| `logout` | 清除本地已保存的登录凭据 | 本机 credential 清理；不调用远端 revoke/logout |
| `whoami` | 显示当前认证状态和登录身份 | 主要读本地 credential；access token 过期时可能刷新 token |
| `version` / `-v` / `--version` | 输出 git-ai 版本 | 不要求本机 daemon；不访问远端 |
| `help` / `-h` / `--help` | 显示帮助信息 | 不要求本机 daemon；不访问远端 |

## 隐藏或内部命令

这些命令在 dispatcher 中存在，但不是全部出现在 `git-ai help` 中：

| 命令 | 用途 | 服务端关系 |
| --- | --- | --- |
| `git-hooks` | 清理已废弃的 git core hook | 本机仓库 hooks 管理 |
| `flush-metrics-db` | 手动上传本地 metrics DB 队列 | 访问 `/worker/metrics/upload` |
| `exchange-nonce` | 安装脚本自动登录，用 install nonce 换 credential | 访问 `/worker/oauth/token`，`grant_type=install_nonce` |
| `dash` / `dashboard` | 打开个人 dashboard 页面 | 打开 `{api_base_url}/me` |
| `effective-ignore-patterns` | 机器调用 / 内部命令，输出 effective ignore patterns | 本机 daemon 必需；本地仓库分析 |
| `blame-analysis` | 机器调用 / 内部命令，输出 blame analysis JSON | 本机 daemon 必需；本地仓库分析 |
| `fetch-authorship-notes` / `fetch_authorship_notes` | 机器调用 / 内部命令，拉取 authorship notes | 本机 daemon 必需；git remote notes |
| `push-authorship-notes` / `push_authorship_notes` | 机器调用 / 内部命令，推送 authorship notes | 本机 daemon 必需；git remote notes |
| `notes migrate` | 批量上传已有 `refs/notes/ai` 到 HTTP notes backend | 仅 `notes_backend.kind=http` 时有意义；访问 `/worker/notes/upload` |
| `notes serve` | 本地内存版 notes backend reference server | 开发 / 测试用途，不是生产服务端 |

## 明确和 server-go / Git AI API 相关的命令

### `git-ai login`

直接走 OAuth device flow：

1. `POST /worker/oauth/device/code`
2. 浏览器打开服务端返回的 verification URL
3. 轮询 `POST /worker/oauth/token`

Token request 使用：

```json
{
  "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
  "device_code": "<device_code>",
  "client_id": "git-ai-cli"
}
```

客户端识别的 OAuth pending / error code：

- `authorization_pending`
- `slow_down`
- `access_denied`
- `expired_token`

### `git-ai whoami`

主要读取本地 credential store，并从 access token claims 中解析：

- `sub`
- `email`
- `name`
- `personal_org_id`
- `orgs[]`

如果本地 access token 已过期但 refresh token 未过期，`ApiContext::new()` 可能调用 `POST /worker/oauth/token` 刷新：

```json
{
  "grant_type": "refresh_token",
  "refresh_token": "<refresh_token>",
  "client_id": "git-ai-cli"
}
```

没有 credential 且没有 API key 时，命令会输出 logged out 并返回非零状态。

### `git-ai logout`

只清理本地 credential store。当前客户端没有调用服务端 logout / revoke endpoint。

### `git-ai dash` / `git-ai dashboard`

读取 `api_base_url`，打开：

```text
{api_base_url}/me
```

这个路径通常由 SPA / nginx static hosting 提供，Go 服务端负责其背后的 `/api/me`、dashboard 和 metrics API。

### `git-ai exchange-nonce`

安装脚本内部命令，从环境变量读取：

- `INSTALL_NONCE`
- `API_BASE`

然后调用 `POST /worker/oauth/token`：

```json
{
  "grant_type": "install_nonce",
  "install_nonce": "<nonce>",
  "client_id": "git-ai-cli"
}
```

成功后把 credential 写入本地 credential store。

### `git-ai upgrade`

访问 release endpoints：

- `GET /worker/releases`
- `GET /worker/releases/:channel/download/SHA256SUMS`
- `GET /worker/releases/:channel/download/install.sh`
- `GET /worker/releases/:channel/download/install.ps1`

客户端 channel 当前包括：

- `latest`
- `next`
- `enterprise-latest`
- `enterprise-next`

### `git-ai flush-metrics-db`

从本地 metrics SQLite 队列取出事件，上传到：

```text
POST /worker/metrics/upload
```

当使用默认 API 且未登录、也未配置 API key 时，会跳过上传。

### `git-ai fetch-notes`

默认行为是从 git remote 拉取 `refs/notes/ai`。

当配置为 HTTP notes backend：

```bash
git-ai config set notes_backend.kind http
git-ai config set notes_backend.backend_url <base-url>
```

命令会改为通过 HTTP notes backend warm 本地 notes cache，读取：

```text
GET /worker/notes/?commits=<sha1>,<sha2>,...
```

### `git-ai notes migrate`

只在 `notes_backend.kind=http` 时运行。它读取本地 `refs/notes/ai`，分批上传：

```text
POST /worker/notes/upload
```

要求已登录或配置 API key。

## daemon 相关注意事项

`git-ai` 主入口在执行多数子命令前，会先初始化本机 daemon telemetry handle。以下命令不要求本机 daemon 可连接：

- `help`
- `--help`
- `-h`
- `version`
- `--version`
- `-v`
- `config`
- `bg`
- `d`
- `daemon`
- `debug`
- `upgrade`
- `install-hooks`
- `install`
- `uninstall-hooks`

其余 `git-ai` 子命令如果无法连接本机后台服务，会报错并退出：

```text
error: failed to connect to git-ai background service: ...
```

服务端排障时需要区分：

- OAuth、release、metrics、CAS、notes HTTP backend 失败，通常是 `server-go` / API / 网络 / 认证问题。
- `failed to connect to git-ai background service`，通常是客户端本机 daemon 问题，不是远端 `server-go` 进程问题。

## HTTP 认证头

客户端 `ApiContext` 对 HTTP API 自动附带：

- `User-Agent: git-ai/<version>`
- `X-Distinct-ID: <distinct-id>`
- `Authorization: Bearer <access_token>`，当本地 OAuth credential 可用时
- `X-API-Key: <api-key>`，当配置了 API key 时
- `X-Author-Identity: <git identity>`，仅当配置了 API key 且能解析 git identity 时

server-go 对 worker/API endpoint 做兼容时，应同时考虑 Bearer token 和 API key 两条认证路径。
