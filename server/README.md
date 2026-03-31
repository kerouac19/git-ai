# Git-AI 私有化部署服务器

这是一个用于 Git-AI 私有化部署的后端服务器，采用 Node.js/NestJS 技术栈构建。当前代码库中的可用接口以 `server/src` 的实际实现为准，统一挂载在 `/api` 前缀下。

## 架构说明

这个服务器与 Git-AI 客户端是分开管理的，允许企业内部独立部署，同时保证敏感代码不对外暴露。

### 设计决策

我们从最初的 Rust 服务器方案调整为 Node.js/NestJS 方案，主要基于以下几点考虑：
1. 企业 IT 团队更容易理解和运维 Node.js 应用
2. 快速迭代需求
3. 丰富的生态系统和库支持
4. 将后端作为独立项目维护和部署

## 项目结构

```
server/
├── package.json          # Node.js 项目依赖配置
├── tsconfig.json         # TypeScript 配置
├── nest-cli.json         # NestJS CLI 配置
├── src/                  # 源代码目录
│   ├── authorship/      # 作者归属记录
│   ├── cas/             # 内容寻址存储模块
│   ├── database/        # 数据库配置
│   ├── dashboard/       # 仪表板模块
│   ├── config/          # 配置管理模块
│   ├── guards/          # 守卫模块 (权限验证)
│   ├── interceptors/    # 拦截器模块 
│   ├── middleware/      # 中间件模块
│   ├── security/        # 安全模块 (加密/审计)
│   ├── utils/           # 工具函数
│   ├── app.module.ts    # 主应用模块
│   └── main.ts          # 应用入口点
├── dist/                # 编译后输出目录
├── docs/                # 文档
├── tests/               # 测试文件
└── docker/              # Docker 相关配置
```

## 核心功能

1. **Authorship**
   - `POST /api/authorship/record`
   - `POST /api/authorship/commit`
   - `GET /api/authorship/commits/:userId`
   - `GET /api/authorship/commits/:userId/:commitHash`
   - `GET /api/authorship/commit/:commitHash`
   - `PUT /api/authorship/sync/:userId`

2. **内容寻址存储 (CAS)**
   - `POST /api/cas/upload`
   - `GET /api/cas/read/:hash`

3. **仪表板功能**
   - `GET /api/dashboard/public`
   - `GET /api/dashboard/stats?userId=<uuid>`
   - `POST /api/dashboard/generate-report`

4. **配置管理**
   - `GET /api/config`
   - `GET /api/config/:key`
   - `POST /api/config`
   - `PATCH /api/config/:key`
   - `DELETE /api/config/:key`

## 安装和运行

### 环境要求
- Node.js 20+
- `pnpm` 9+
- PostgreSQL 16+

### 安装依赖
```bash
pnpm install
```

### 环境变量配置

本地开发启动方式已单独整理到 [local-startup.md](docs/local-startup.md)。

可从 [`.env.example`](.env.example) 复制一份到 `.env`，最少需要配置：

```env
PORT=3000
CORS_ORIGIN=http://localhost:3000
NODE_ENV=development
JWT_SECRET=<long-random-secret>
ENCRYPTION_MASTER_KEY=<64-char-hex-string>
CAS_ENCRYPTION_KEY=<long-random-secret>
TRUST_PROXY=false
HTTPS_REDIRECT=false
DEV_HTTPS=false
JSON_BODY_LIMIT=2mb
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=
DB_NAME=git_ai
```

说明：

- `JWT_SECRET` 始终必须显式配置。
- `ENCRYPTION_MASTER_KEY` 和 `CAS_ENCRYPTION_KEY` 在生产环境必须显式配置。
- 开发环境未设置加密密钥时，会退回进程内临时密钥；重启后旧数据可能无法解密。
- 当前服务只支持 PostgreSQL。
- 可使用 `DATABASE_URL`，或分别设置 `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME`。
- `TRUST_PROXY` 支持 `false`、`true`、数字 hop 数，供反向代理部署使用。
- `HTTPS_REDIRECT=true` 时，仅在请求真实被识别为 HTTPS 以外时才执行跳转。
- 本地开发可设置 `DEV_HTTPS=true`，让 NestJS 直接监听 HTTPS。
- `DEV_HTTPS_KEY_PATH` 和 `DEV_HTTPS_CERT_PATH` 默认分别读取 `certs/localhost-key.pem` 和 `certs/localhost.pem`。
- `JSON_BODY_LIMIT` 默认为 `2mb`，用于兼容较大的 CAS prompt payload。
- 若要直接跑本地 HTTPS 联调，可从 [`.env.local-https.example`](.env.local-https.example) 开始。

本地 HTTPS 示例：

```bash
cp .env.local-https.example .env.local-https
ENV_FILE=.env.local-https ./scripts/start-local-https.sh
```

之后可把客户端指向：

```bash
export GIT_AI_API_BASE_URL=https://git-ai.localhost:3443
```

说明：

- 脚本默认读取 [`.env.local-https.example`](.env.local-https.example)
- 可通过 `ENV_FILE=/abs/path/to/env` 指定自定义环境文件
- 首次启动会自动调用 `mkcert` 生成本地受信任证书
- `WATCH_MODE=1 ./scripts/start-local-https.sh` 会改用 `pnpm start:dev`
- `SKIP_BUILD=1 ./scripts/start-local-https.sh` 会跳过构建

### 构建和启动
```bash
# 安装依赖
pnpm install

# 构建项目
pnpm build

# 开发模式启动
pnpm start:dev

# 生产模式启动
pnpm start
```

可选的 PostgreSQL 初始化示例：

```bash
createdb git_ai
DB_TYPE=postgres DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=postgres DB_NAME=git_ai pnpm start
```

## Docker 部署

```bash
# 构建镜像
docker build -t git-ai-private-server .

# 运行容器
docker run -d -p 3000:3000 --env-file .env git-ai-private-server
```

也可以使用 `docker-compose.yml` 作为起点，再按 `.env.example` 补齐变量。

## 当前 API 合约

当前仓库中统一采用以下 API 合约：

- 业务接口统一挂载在 `/api`
- 已实现模块：`authorship`、`cas`、`dashboard`、`config`
- 已实现兼容端点：
  - `GET /health`
  - `GET /api/health`
  - `GET /api/health/database`
  - `GET /api/status`
  - `GET /api/version`
  - `GET /api/me`
  - `GET /me`（受浏览器 session/cookie 保护的 HTML 页面）
  - `GET /oauth/device`
  - `POST /oauth/device/approve`
  - `POST /oauth/device/deny`
  - `POST /worker/oauth/device/code`
  - `POST /worker/oauth/token`
  - `POST /worker/metrics/upload`
  - `POST /worker/cas/upload`
  - `GET /worker/cas?hashes=...`
  - `GET /worker/cas/checkout`
  - 同时兼容 `/workers/*` 复数路径

兼容认证层用于本地私有部署和旧客户端接入，验证脚本已按这套路由更新。

注意:

- 当前 `git-ai` 客户端已经会把 prompt transcript 通过 git note/CAS 链路同步到私有化 server
- 但普通 `git commit` 后还不会自动调用 `/api/authorship/record` 或 `/api/authorship/commit`
- 因此 `authorship_records` / `commit_attributions` 目前不能作为 smoke 联调是否成功的首要判断依据
- 现阶段更可靠的联调判断信号是 `oauth_device_codes`、`metrics_events`、`audit_logs`、`cas_entries`

## 安全性

- `config` 相关端点通过 JWT 守卫保护，可接受兼容设备流签发的 Bearer token
- 当前实现的是本地兼容 OAuth/JWT 层，不包含企业外部 IdP、SAML 或 OIDC 联邦登录集成
- 数据传输使用 TLS/SSL 加密
- 本地数据存储使用 AES-256-GCM 加密
- 审计日志写入 PostgreSQL `audit_logs` 表
- 敏感配置项只返回 mask，不返回存储中的密文

## 当前认证能力

当前 `server/` 中已经可用的用户认证相关能力：

- 兼容 OAuth Device Flow
  - `POST /worker/oauth/device/code`
  - `POST /worker/oauth/token`
  - 同时兼容 `/workers/*`
- JWT Bearer 鉴权
  - 受保护接口可通过 `Authorization: Bearer <access_token>` 访问
- `refresh_token` 刷新
- `install_nonce` 兼容登录分支
- 当前用户信息接口
  - `GET /api/me`
  - `GET /me`
- 本地默认用户身份
  - 由兼容认证层生成 JWT，用于本地私有部署和旧客户端接入验证
  - 设备授权页批准后会为浏览器写入 session cookie，供 `/me` 页面访问

当前已验证通过的认证链路：

- `POST /worker/oauth/device/code`
- `POST /worker/oauth/token`
- `GET /api/me`
- `GET /me`
- 带 Bearer token 访问受保护的 `config` 端点

## 当前未覆盖的认证能力

以下能力当前没有实现，后续若要作为完整用户系统使用，需要单独补齐：

- 传统 `POST /login`
- 用户名 / 密码认证
- 注册、登出、找回密码
- 真实用户表和用户持久化
- 会话管理
- MFA / 2FA
- 企业 IdP / SSO 集成
  - OIDC
  - SAML
  - LDAP
  - Azure AD / Okta / Google Workspace
- 持久化的角色 / 权限模型
- token 撤销 / 黑名单
- 审计级认证事件持久化

结论：当前认证层适合本地开发、自托管兼容接入和 CLI 联调，不应直接视为完整企业认证中心。

## 维护

后续若接入真实企业身份系统或调整兼容端点，应同步更新本 README 和 `scripts/` 下的验证脚本。
