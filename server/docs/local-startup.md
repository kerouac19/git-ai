# Server 本地启动说明

这份文档只描述 `server/` 在本地开发环境中的启动方法，以及如何让本机已安装的 `git-ai` 直接接入本地服务验证 `login` / `dash` / metrics / CAS。

## 1. 前提

- Node.js 20+
- `pnpm` 9+
- PostgreSQL 16+
- `mkcert`

建议先确认本地数据库已准备好：

```bash
createdb git_ai
psql git_ai -c "select 1;"
```

## 2. 推荐方式：本地 HTTPS 一键启动

这是最适合和已安装 release 版 `git-ai` 联调的方式。

### 2.1 准备环境文件

```bash
cd /Users/hg/git/git-ai/server
cp .env.local-https.example .env.local-https
```

如果你已经有可用的本地配置，也可以直接编辑 [`.env.local-https`](/Users/hg/git/git-ai/server/.env.local-https)。

关键变量：

- `PORT=3443`
- `DEV_HTTPS=true`
- `JWT_SECRET`
- `ENCRYPTION_MASTER_KEY`
- `CAS_ENCRYPTION_KEY`
- `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME`

### 2.2 启动

```bash
cd /Users/hg/git/git-ai/server
ENV_FILE=/Users/hg/git/git-ai/server/.env.local-https ./scripts/start-local-https.sh
```

这个脚本会自动：

- 读取 `ENV_FILE`
- 检查并生成本地证书
- 默认执行 `pnpm build`
- 启动 `https://git-ai.localhost:3443`

脚本在 [start-local-https.sh](/Users/hg/git/git-ai/server/scripts/start-local-https.sh)。

### 2.3 可选参数

热重载：

```bash
WATCH_MODE=1 ENV_FILE=/Users/hg/git/git-ai/server/.env.local-https ./scripts/start-local-https.sh
```

跳过构建：

```bash
SKIP_BUILD=1 ENV_FILE=/Users/hg/git/git-ai/server/.env.local-https ./scripts/start-local-https.sh
```

## 3. 手动启动 HTTPS

如果你不想用脚本，也可以手动启动。

### 3.1 生成证书

```bash
cd /Users/hg/git/git-ai/server
mkcert -install
mkdir -p certs
mkcert -key-file certs/localhost-key.pem -cert-file certs/localhost.pem git-ai.localhost localhost 127.0.0.1 ::1
```

### 3.2 启动服务

```bash
cd /Users/hg/git/git-ai/server
export PORT=3443
export DEV_HTTPS=true
export DEV_HTTPS_KEY_PATH=certs/localhost-key.pem
export DEV_HTTPS_CERT_PATH=certs/localhost.pem
export HTTPS_REDIRECT=false
export JWT_SECRET=dev-jwt-secret-change-me
export ENCRYPTION_MASTER_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
export CAS_ENCRYPTION_KEY=dev-cas-secret-change-me
export DB_HOST=127.0.0.1
export DB_PORT=5432
export DB_USER=hg
export DB_NAME=git_ai

pnpm build
pnpm start
```

## 4. HTTP 启动

如果你只是调 `server` 本身，不需要让 release 版 `git-ai login` 直连，也可以直接 HTTP 启动：

```bash
cd /Users/hg/git/git-ai/server
cp .env.example .env
pnpm build
pnpm start
```

默认地址通常是：

```text
http://localhost:3000
```

注意：

- 已安装的 release 版 `git-ai login` 要求 OAuth base URL 是 `https://`
- 所以 HTTP 模式更适合服务端自身开发，不适合直接验证 release `git-ai login`

## 5. 已安装 `git-ai` 如何接本地 HTTPS

推荐先改客户端配置：

`~/.git-ai/config.json`

```json
{
  "git_path": "/opt/homebrew/bin/git",
  "api_base_url": "https://localhost:3443"
}
```

然后验证健康检查：

```bash
curl -fsS https://localhost:3443/health
```

如果通过，再跑：

```bash
git-ai login
git-ai dash
```

## 6. 本机证书环境冲突的处理

如果 `git-ai login` 报：

```text
invalid peer certificate: UnknownIssuer
```

优先检查当前 shell 有没有这类全局 CA 覆盖：

- `SSL_CERT_FILE`
- `SSL_CERT_DIR`
- `CURL_CA_BUNDLE`

可直接先看：

```bash
env | grep -E '^(SSL_CERT_FILE|SSL_CERT_DIR|CURL_CA_BUNDLE)='
```

如果这三项里有值，`rustls` 可能优先使用这套 CA 配置，而不是系统信任链里的 `mkcert` 根证书。你这台机器之前出现的 `UnknownIssuer`，就是这个原因。

建议先执行：

```bash
mkcert -install
```

然后再用“局部移除证书环境变量”的方式跑 `git-ai`，不要直接改全局 shell 配置。

### 6.1 推荐跑法

```bash
env -u SSL_CERT_FILE -u SSL_CERT_DIR -u CURL_CA_BUNDLE \
  GIT_AI_API_BASE_URL=https://localhost:3443 \
  git-ai login
```

### 6.2 推荐做成本地函数

```bash
git-ai-local() {
  env -u SSL_CERT_FILE -u SSL_CERT_DIR -u CURL_CA_BUNDLE \
    GIT_AI_API_BASE_URL=https://localhost:3443 \
    git-ai "$@"
}
```

之后使用：

```bash
git-ai-local login
git-ai-local dash
git-ai-local whoami
```

### 6.3 不建议的做法

- 不建议直接删除系统证书
- 不建议立刻改动全局 zerobrew CA bundle
- 不建议为了本地联调去关闭 TLS 校验

当前最稳的处理方式，就是只在 `git-ai` 这条命令前局部 `unset` 上述环境变量。

## 7. 最小验证命令

### 7.1 只看服务是否起来

```bash
curl -fsS https://localhost:3443/health
```

### 7.2 服务 + 登录联调

```bash
curl -fsS https://localhost:3443/health && echo && \
env -u SSL_CERT_FILE -u SSL_CERT_DIR -u CURL_CA_BUNDLE \
  GIT_AI_API_BASE_URL=https://localhost:3443 \
  git-ai login
```

### 7.3 跑私有化 smoke

```bash
cd /Users/hg/git/git-ai
./scripts/test-private-flow.sh
```

## 8. 当前本地联调的真实闭环

当前已实际验证通过的是：

- `login`
- `approve`
- `token`
- `/api/me`
- `metrics upload`
- `/me`
- `CAS upload`
- `CAS read`

注意：

- `authorship_records` / `commit_attributions` 目前不是普通 `git commit` 的自动上报结果
- 所以本地验证时，优先看 `oauth_device_codes`、`metrics_events`、`audit_logs`、`cas_entries`
