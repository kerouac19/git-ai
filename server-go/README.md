# Git-AI 私有化部署服务器（Go 版）

这是 `server/`（Node.js/NestJS）的 Go 重写版本，功能完全对等，部署更轻量。

## 与 Node.js 版对比

| 维度 | Node.js (`server/`) | Go (`server-go/`) |
|------|--------------------|--------------------|
| 运行时 | Node.js 20+ / pnpm | 单二进制，无依赖 |
| Docker 镜像 | ~300MB (node:20-alpine) | ~20MB (distroless) |
| 内存占用 | ~150-300MB | ~30-50MB |
| 冷启动 | 3-5 秒 | <1 秒 |
| 数据库 | Prisma ORM | pgx + 原生 SQL |
| Metrics 写入 | 逐条 INSERT | pgx.CopyFrom 批量写入 |

## 环境要求

- Go 1.26+
- PostgreSQL 16+（18.x 也可用）

## Ubuntu 裸机快速开始

以下流程适用于 Ubuntu 24.04+ 裸机部署，也可作为其他 Linux 发行版的参考。

### 1. 构建 Linux 二进制

```bash
cd server-go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/server ./cmd/server
```

### 2. 准备数据库

```bash
psql -d postgres -c "CREATE DATABASE git_ai"
```

启动时会自动执行 SQL 迁移，无需手动建表。

### 3. 创建部署目录

```bash
sudo mkdir -p /opt/git-ai/server-go/current
sudo mkdir -p /opt/git-ai/logs
sudo cp bin/server /opt/git-ai/server-go/current/server
```

### 4. 写入环境文件

创建 `/opt/git-ai/.env`：

```env
PORT=3000
APP_ENV=production
JWT_SECRET=<openssl rand -base64 48 的输出>
ENCRYPTION_MASTER_KEY=<openssl rand -hex 32 的输出>
CAS_ENCRYPTION_KEY=<openssl rand -base64 48 的输出>
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=
DB_NAME=git_ai
DB_SSL=false
CORS_ORIGIN=https://git-ai.example.com
TRUST_PROXY=1
HTTPS_REDIRECT=false
```

### 5. 先手工启动验证

```bash
cd /opt/git-ai/server-go/current
set -a
. /opt/git-ai/.env
set +a
./server
```

### 6. 健康检查

```bash
curl http://127.0.0.1:3000/health
curl http://127.0.0.1:3000/api/health/database
```

### 7. 配置 systemd

将以下内容保存为 `/etc/systemd/system/git-ai.service`：

```ini
[Unit]
Description=Git-AI Private Deploy Server
After=network.target postgresql.service

[Service]
Type=simple
User=git-ai
Group=git-ai
WorkingDirectory=/opt/git-ai/server-go/current
ExecStart=/opt/git-ai/server-go/current/server
EnvironmentFile=/opt/git-ai/.env
Restart=always
RestartSec=5
LimitNOFILE=65536
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/git-ai/logs

[Install]
WantedBy=multi-user.target
```

然后执行：

```bash
sudo systemctl daemon-reload
sudo systemctl enable git-ai
sudo systemctl start git-ai
sudo systemctl status git-ai
```

需要更完整的发布说明、反向代理配置和排障建议时，继续看 [docs/production-deployment.md](docs/production-deployment.md)。

## 快速开始

### 1. 构建

```bash
cd server-go
go build -o bin/server ./cmd/server
```

### 2. 准备数据库

```bash
psql -d postgres -c "CREATE DATABASE git_ai"
```

启动时会自动执行 SQL 迁移（7 张表），无需手动建表。

### 3. 配置环境变量

```bash
export PORT=3000
export APP_ENV=development          # development | production
export JWT_SECRET=<长随机字符串>      # 必须设置
export DB_HOST=127.0.0.1
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=
export DB_NAME=git_ai

# 加密密钥（生产环境必须显式设置，开发环境可省略）
export ENCRYPTION_MASTER_KEY=<64位hex字符串>
export CAS_ENCRYPTION_KEY=<长随机字符串>
```

### 4. 启动

```bash
./bin/server
```

输出示例：

```
Application is running on: http://localhost:3000
Environment: development
Database target: postgres://127.0.0.1:5432/git_ai
Trust proxy: false
```

### 5. 验证

```bash
curl http://127.0.0.1:3000/health
# {"service":"git-ai-private-deploy-server","status":"ok"}

curl http://127.0.0.1:3000/api/health/database
# {"database":"connected","status":"ok"}
```

## 完整环境变量

| 变量 | 必须 | 默认值 | 说明 |
|------|------|--------|------|
| `PORT` | 否 | `3000` | 监听端口 |
| `APP_ENV` | 否 | `development` | 环境标识 |
| `JWT_SECRET` | **是** | - | JWT 签名密钥 |
| `ENCRYPTION_MASTER_KEY` | 生产必须 | 开发自动生成 | 配置加密主密钥（64 位 hex） |
| `CAS_ENCRYPTION_KEY` | 生产必须 | 开发自动生成 | CAS 内容加密密钥 |
| `DATABASE_URL` | 否 | 从 DB_* 拼接 | 完整数据库连接串 |
| `DB_HOST` | 否 | `127.0.0.1` | 数据库主机 |
| `DB_PORT` | 否 | `5432` | 数据库端口 |
| `DB_USER` | 否 | `postgres` | 数据库用户 |
| `DB_PASSWORD` | 否 | 空 | 数据库密码 |
| `DB_NAME` | 否 | `git_ai` | 数据库名 |
| `DB_SSL` | 否 | `false` | 启用 SSL |
| `CORS_ORIGIN` | 否 | `*` | CORS 允许源 |
| `TRUST_PROXY` | 否 | `false` | 信任代理（`true`/`false`/数字） |
| `HTTPS_REDIRECT` | 否 | `false` | 强制 HTTPS 跳转 |
| `JSON_BODY_LIMIT` | 否 | `2mb` | 请求体大小限制 |
| `DEFAULT_USER_ID` | 否 | `00000000-...` | 默认用户 ID |
| `DEFAULT_USER_EMAIL` | 否 | `git-ai@example.local` | 默认用户邮箱 |
| `DEFAULT_USER_NAME` | 否 | `Git AI User` | 默认用户名 |
| `DEFAULT_USER_ROLE` | 否 | `user` | 默认角色 |

## API 端点

### 健康检查

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/health` | 无 | 基础健康检查 |
| GET | `/api/health` | 无 | API 健康检查 |
| GET | `/api/health/database` | 无 | 数据库连通性 |
| GET | `/api/status` | 无 | 服务状态 + 公开统计 |
| GET | `/api/version` | 无 | 版本号 |

### OAuth Device Flow

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/worker/oauth/device/code` | 发起设备授权流程 |
| POST | `/worker/oauth/token` | Token 交换（device_code / refresh_token / install_nonce） |
| GET | `/oauth/device?user_code=XXX` | 设备授权页面（HTML） |
| POST | `/oauth/device/approve` | 批准授权 |
| POST | `/oauth/device/deny` | 拒绝授权 |

> 同时支持 `/workers/*` 复数路径。

### 用户

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/me` | JWT | 当前用户信息 + 仪表板 |
| GET | `/me` | Cookie | 用户仪表板页面（HTML） |

### Bundles

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/bundles` | 无 | 创建分享 bundle |

### CAS（内容寻址存储）

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/cas/upload` | 无 | 上传内容 |
| GET | `/api/cas/read/:hash` | 无 | 读取内容 |
| POST | `/worker/cas/upload` | JWT 或 `X-API-Key` | Worker 上传（JSON 对象或 multipart 文件） |
| GET | `/worker/cas?hashes=...` | JWT 或 `X-API-Key` | 批量读取 |
| GET | `/worker/cas/?hashes=...` | JWT 或 `X-API-Key` | 批量读取（兼容尾斜杠） |
| GET | `/worker/cas/checkout?hash=...` | JWT 或 `X-API-Key` | 单个对象读取 |

### Metrics（遥测）

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/worker/metrics/upload` | JWT 或 `X-API-Key` | 批量上传事件（pgx.CopyFrom） |

当前已与客户端 `metrics` wire format `v=1` 对齐，接受事件 ID `1/2/3/4`，单批次最多 `250` 条。
`values_json` / `attrs_json` 会保留原始 sparse payload，因此兼容当前客户端新增的 committed 字段 `10/11/12` 与 attributes `2/3/4/5/30`。

### Releases / Upgrade

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/worker/releases` | 无 | 查询 release channels |
| GET | `/worker/releases/:channel/download/SHA256SUMS` | 无 | 下载校验和 |
| GET | `/worker/releases/:channel/download/install.sh` | 无 | 下载 Unix 安装脚本占位 |
| GET | `/worker/releases/:channel/download/install.ps1` | 无 | 下载 Windows 安装脚本占位 |

### Authorship（作者归属）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/authorship/record` | 保存归属记录 |
| POST | `/api/authorship/commit` | 保存提交归属 |
| GET | `/api/authorship/commits/:userId` | 用户提交列表（分页） |
| GET | `/api/authorship/commits/:userId/:commitHash` | 特定用户提交 |
| GET | `/api/authorship/commit/:commitHash` | 按哈希查归属 |
| PUT | `/api/authorship/sync/:userId` | 同步（upsert） |

### Dashboard（仪表板）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dashboard/public` | 公开统计 |
| GET | `/api/dashboard/stats?userId=...` | 用户仪表板统计 |
| POST | `/api/dashboard/generate-report` | 生成报告 |

### Config（系统配置）

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/config` | JWT | 配置列表 |
| GET | `/api/config/:key` | JWT | 获取配置 |
| POST | `/api/config` | JWT | 创建配置 |
| PATCH | `/api/config/:key` | JWT | 更新配置 |
| DELETE | `/api/config/:key` | JWT | 删除配置 |

> 敏感配置项存储时 AES-256-GCM 加密，读取时返回 `[REDACTED]`。

## 测试

### 冒烟测试

自动化测试脚本覆盖全部端点（86 个断言）：

```bash
# 先启动服务，然后运行：
bash scripts/smoke-test.sh

# 指定地址：
bash scripts/smoke-test.sh http://127.0.0.1:3000
```

测试覆盖：
- ① 健康检查（5 个端点）
- ② OAuth Device Flow（完整流程：发起→pending→approve→exchange→refresh→install_nonce→deny）
- ③ JWT 认证保护
- ④ CAS 上传 + 读取（压缩→加密→解密→解压 roundtrip 验证）
- ⑤ Metrics 批量写入
- ⑥ Dashboard 聚合查询
- ⑦ Authorship CRUD + upsert
- ⑧ Config CRUD（敏感字段加密 + 脱敏）
- ⑨ HTML 页面渲染
- ⑩ 错误处理边界

### 客户端联调

```bash
export GIT_AI_API_BASE_URL=http://127.0.0.1:3000
git-ai login          # Device Flow 登录
git-ai checkpoint ... # 触发 CAS + Metrics 上传
```

## Docker 部署

```bash
# 构建镜像（~20MB）
docker build -t git-ai-server .

# 运行
docker run -d -p 3000:3000 \
  -e JWT_SECRET=<your-secret> \
  -e DB_HOST=<db-host> \
  -e DB_PORT=5432 \
  -e DB_USER=postgres \
  -e DB_NAME=git_ai \
  -e ENCRYPTION_MASTER_KEY=<64-char-hex> \
  -e CAS_ENCRYPTION_KEY=<your-key> \
  git-ai-server
```

## 项目结构

```
server-go/
├── cmd/server/
│   ├── main.go                     # 入口：配置、路由注册、HTML 页面处理
│   └── templates/                  # HTML 模板（embed 嵌入到二进制）
│       ├── device_flow.html        # 设备授权页
│       ├── device_result.html      # 授权结果页
│       ├── dashboard.html          # 用户仪表板
│       └── login_required.html     # 登录提示页
├── internal/
│   ├── auth/                       # 认证层
│   │   ├── jwt.go                  # JWT 签发 / 验证（HS256）
│   │   ├── middleware.go           # JWT 中间件（Bearer + Cookie 双通道）
│   │   ├── cookie.go               # Session Cookie 工具
│   │   └── device_flow.go          # OAuth Device Flow 全流程
│   ├── config/
│   │   └── config.go               # Viper 环境变量配置
│   ├── crypto/
│   │   ├── aes.go                  # AES-256-GCM 双模式加密
│   │   ├── hash.go                 # SHA256 / HMAC
│   │   └── secrets.go              # 运行时密钥管理
│   ├── database/
│   │   ├── postgres.go             # pgx 连接池
│   │   ├── migrate.go              # golang-migrate 迁移
│   │   └── migrations/             # SQL 迁移文件（7 张表）
│   ├── handler/                    # HTTP 处理器（gin）
│   │   ├── health.go
│   │   ├── compatibility.go        # /worker/* 兼容端点
│   │   ├── authorship.go
│   │   ├── cas.go
│   │   ├── dashboard.go
│   │   └── sysconfig.go
│   ├── middleware/
│   │   ├── audit.go                # 审计日志（异步写入）
│   │   ├── security_headers.go     # HSTS / CSP / XSS 防护
│   │   └── https_redirect.go       # HTTPS 强制跳转
│   ├── model/                      # 数据模型 + DTO
│   └── service/                    # 业务逻辑
│       ├── cas.go                  # zlib 压缩 + AES 加密存储
│       ├── metrics.go              # pgx.CopyFrom 批量写入
│       ├── authorship.go
│       ├── dashboard.go            # errgroup 并行聚合查询
│       ├── sysconfig.go            # 敏感字段加密管理
│       └── audit.go
├── scripts/
│   └── smoke-test.sh               # 冒烟测试脚本
├── Dockerfile                      # 多阶段构建（distroless）
├── go.mod
└── go.sum
```

## 安全性

- JWT HS256 认证，支持 Bearer Header + Cookie 双通道
- CAS 数据 zlib 压缩后 AES-256-GCM 加密存储（Scrypt 密钥派生）
- 系统配置敏感字段 AES-256-GCM 加密，读取时脱敏
- 审计日志异步写入 PostgreSQL
- 安全头：HSTS、CSP、X-Frame-Options、XSS Protection
- 请求体敏感字段自动脱敏（password、token、secret、key）
