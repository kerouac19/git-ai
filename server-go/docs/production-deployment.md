# Git-AI Go Server — 生产环境发布指南

> 版本: 1.0.0 | 更新日期: 2026-04-09

## 1. 发布概述

Go 版本是 `server/`（Node.js/NestJS）的完整重写，功能对等，部署更轻量：

- **单二进制**，无运行时依赖
- Docker 镜像 ~20MB（vs Node.js ~300MB）
- 内存占用 ~30-50MB（vs Node.js ~150-300MB）
- 冷启动 <1 秒
- 启动时自动执行数据库迁移

---

## 2. 发布前检查清单

### 2.1 必须确认

- [ ] 冒烟测试全部通过：`bash scripts/smoke-test.sh`
- [ ] PostgreSQL 16+ 已部署并可连接
- [ ] `JWT_SECRET` 已生成（≥32 字符随机字符串）
- [ ] `ENCRYPTION_MASTER_KEY` 已生成（64 位 hex 字符串 = 32 字节密钥）
- [ ] `CAS_ENCRYPTION_KEY` 已生成（≥32 字符随机字符串）
- [ ] 以上三个密钥已安全存储（Vault / AWS Secrets Manager / 密封信封）
- [ ] DNS / 负载均衡 已就绪
- [ ] TLS 证书已准备（如需 HTTPS 直连或反向代理终止）

### 2.2 密钥生成

```bash
# JWT_SECRET
openssl rand -base64 48

# ENCRYPTION_MASTER_KEY（64 位 hex）
openssl rand -hex 32

# CAS_ENCRYPTION_KEY
openssl rand -base64 48
```

> ⚠️ **密钥一旦投产后不可更换**，否则已加密的 CAS 数据和系统配置将无法解密。

---

## 3. 构建

### 3.1 本地构建

```bash
cd server-go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/server ./cmd/server
```

交叉编译（ARM64 / Apple Silicon）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/server-arm64 ./cmd/server
```

### 3.2 Ubuntu 裸机快速路径

适用于首次在 Ubuntu 24.04+ 上做二进制直部署。若构建机和目标机都是 `linux/amd64`，可直接沿用上面的产物。

已验证可用的版本边界：

- Go 1.26.x
- PostgreSQL 16+（18.x 也可用）

推荐步骤：

```bash
# 1. 构建
cd server-go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/server ./cmd/server

# 2. 准备数据库
psql -d postgres -c "CREATE DATABASE git_ai"

# 3. 准备目录
sudo mkdir -p /opt/git-ai/server-go/current
sudo mkdir -p /opt/git-ai/logs
sudo cp bin/server /opt/git-ai/server-go/current/server

# 4. 生成密钥（分别写入 /opt/git-ai/.env）
openssl rand -base64 48
openssl rand -hex 32
openssl rand -base64 48
```

建议的 `/opt/git-ai/.env` 最小生产配置：

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

首次启动前可先手工验证：

```bash
cd /opt/git-ai/server-go/current
set -a
. /opt/git-ai/.env
set +a
./server

curl http://127.0.0.1:3000/health
curl http://127.0.0.1:3000/api/health/database
```

### 3.3 Docker 构建

```bash
docker build -t git-ai-server:1.0.0 .
docker tag git-ai-server:1.0.0 your-registry.com/git-ai-server:1.0.0
docker push your-registry.com/git-ai-server:1.0.0
```

产出镜像基于 `gcr.io/distroless/static-debian12`，约 20MB，无 shell、无包管理器。

---

## 4. 生产环境配置

### 4.1 环境变量（完整）

```bash
# ───── 必须设置 ─────
PORT=3000
APP_ENV=production
JWT_SECRET=<openssl rand -base64 48 的输出>
ENCRYPTION_MASTER_KEY=<openssl rand -hex 32 的输出>
CAS_ENCRYPTION_KEY=<openssl rand -base64 48 的输出>

# ───── 数据库 ─────
DB_HOST=db.internal.example.com
DB_PORT=5432
DB_USER=git_ai
DB_PASSWORD=<数据库密码>
DB_NAME=git_ai
DB_SSL=true
DB_SSL_REJECT_UNAUTHORIZED=true
# 或直接使用 DATABASE_URL：
# DATABASE_URL=postgresql://git_ai:<password>@db.internal:5432/git_ai?sslmode=verify-full

# ───── 网络 ─────
CORS_ORIGIN=https://git-ai.your-company.com
TRUST_PROXY=1                    # 反向代理后设为 1 或 true
HTTPS_REDIRECT=true              # 强制 HTTPS

# ───── 默认用户（私有部署兼容） ─────
DEFAULT_USER_ID=00000000-0000-0000-0000-000000000001
DEFAULT_USER_EMAIL=admin@your-company.com
DEFAULT_USER_NAME=Admin
DEFAULT_USER_ROLE=admin
DEFAULT_PERSONAL_ORG_ID=your-org
DEFAULT_ORG_NAME=Your Company
DEFAULT_ORG_SLUG=your-company
```

### 4.2 配置验证

启动后检查日志：

```
Application is running on: http://localhost:3000
Environment: production
Database target: postgres://db.internal:5432/git_ai
Trust proxy: 1
```

确认 `Environment: production` 和 `Database target` 正确。

---

## 5. 部署方式

### 5.1 Docker Compose（推荐）

创建 `docker-compose.yml`：

```yaml
version: "3.8"

services:
  server:
    image: your-registry.com/git-ai-server:1.0.0
    ports:
      - "3000:3000"
    environment:
      PORT: "3000"
      APP_ENV: production
      JWT_SECRET: "${JWT_SECRET}"
      ENCRYPTION_MASTER_KEY: "${ENCRYPTION_MASTER_KEY}"
      CAS_ENCRYPTION_KEY: "${CAS_ENCRYPTION_KEY}"
      DB_HOST: db
      DB_PORT: "5432"
      DB_USER: git_ai
      DB_PASSWORD: "${DB_PASSWORD}"
      DB_NAME: git_ai
      DB_SSL: "false"               # 同 Docker 网络内无需 SSL
      CORS_ORIGIN: "https://git-ai.your-company.com"
      TRUST_PROXY: "1"
      HTTPS_REDIRECT: "false"       # 由反向代理处理
    depends_on:
      db:
        condition: service_healthy
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: 256M
          cpus: "1.0"
        reservations:
          memory: 64M
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:3000/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: git_ai
      POSTGRES_USER: git_ai
      POSTGRES_PASSWORD: "${DB_PASSWORD}"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U git_ai"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: 512M

volumes:
  pgdata:
```

启动：

```bash
# 创建 .env 文件
cat > .env <<EOF
JWT_SECRET=$(openssl rand -base64 48)
ENCRYPTION_MASTER_KEY=$(openssl rand -hex 32)
CAS_ENCRYPTION_KEY=$(openssl rand -base64 48)
DB_PASSWORD=$(openssl rand -base64 24)
EOF

docker compose up -d
docker compose logs -f server
```

### 5.2 二进制直部署

适用于不使用 Docker 的环境：

```bash
# 1. 复制二进制到服务器
scp bin/server user@prod-server:/opt/git-ai/

# 2. 创建 systemd service
cat > /etc/systemd/system/git-ai.service <<EOF
[Unit]
Description=Git-AI Private Deploy Server
After=network.target postgresql.service

[Service]
Type=simple
User=git-ai
Group=git-ai
WorkingDirectory=/opt/git-ai
ExecStart=/opt/git-ai/server
EnvironmentFile=/opt/git-ai/.env
Restart=always
RestartSec=5
LimitNOFILE=65536

# 安全加固
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/git-ai/logs

[Install]
WantedBy=multi-user.target
EOF

# 3. 启动
systemctl daemon-reload
systemctl enable git-ai
systemctl start git-ai
systemctl status git-ai
```

### 5.3 Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: git-ai-server
spec:
  replicas: 2
  selector:
    matchLabels:
      app: git-ai-server
  template:
    metadata:
      labels:
        app: git-ai-server
    spec:
      containers:
        - name: server
          image: your-registry.com/git-ai-server:1.0.0
          ports:
            - containerPort: 3000
          envFrom:
            - secretRef:
                name: git-ai-secrets
            - configMapRef:
                name: git-ai-config
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          livenessProbe:
            httpGet:
              path: /health
              port: 3000
            initialDelaySeconds: 5
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /api/health/database
              port: 3000
            initialDelaySeconds: 10
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: git-ai-server
spec:
  selector:
    app: git-ai-server
  ports:
    - port: 80
      targetPort: 3000
  type: ClusterIP
```

---

## 6. 反向代理配置

### 6.1 Nginx

```nginx
upstream git-ai {
    server 127.0.0.1:3000;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name git-ai.your-company.com;

    ssl_certificate     /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    client_max_body_size 10m;

    location / {
        proxy_pass http://git-ai;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_connect_timeout 10s;
        proxy_read_timeout    60s;
        proxy_send_timeout    60s;
    }
}

server {
    listen 80;
    server_name git-ai.your-company.com;
    return 301 https://$host$request_uri;
}
```

---

## 7. 数据库管理

### 7.1 自动迁移

服务启动时自动执行 `golang-migrate`，无需手动建表。迁移文件嵌入在二进制中。

### 7.2 数据库备份

```bash
# 每日备份
pg_dump -U git_ai -h db.internal -d git_ai -Fc > backup_$(date +%Y%m%d).dump

# 恢复
pg_restore -U git_ai -h db.internal -d git_ai -c backup_20260409.dump
```

### 7.3 表结构（7 张表）

| 表 | 说明 | 数据量预估 |
|----|------|-----------|
| `authorship_records` | 作者归属记录 | 随提交增长 |
| `commit_attributions` | 提交归属统计 | 随提交增长 |
| `cas_entries` | CAS 加密存储 | 最大，按 prompt 增长 |
| `config` | 系统配置 | <100 行 |
| `oauth_device_codes` | 设备授权码（自动清理） | <10 行 |
| `metrics_events` | 遥测事件 | 增长最快，考虑定期清理 |
| `audit_logs` | 审计日志 | 持续增长，建议 90 天保留 |

### 7.4 维护建议

```sql
-- 清理 90 天前的审计日志
DELETE FROM audit_logs WHERE occurred_at < now() - interval '90 days';

-- 清理 180 天前的 metrics
DELETE FROM metrics_events WHERE event_timestamp < now() - interval '180 days';

-- 定期 VACUUM
VACUUM ANALYZE metrics_events;
VACUUM ANALYZE audit_logs;
```

---

## 8. 健康检查与监控

### 8.1 健康检查端点

| 端点 | 用途 | 建议间隔 |
|------|------|---------|
| `GET /health` | 进程存活检查（liveness） | 30s |
| `GET /api/health/database` | 数据库连通性（readiness） | 10s |

### 8.2 监控指标

建议通过日志收集或 `/api/dashboard/public` 监控：

- **CAS 存储量**：`totalEntries` / `recentEntries`
- **审计日志量**：定期查询 `audit_logs` 表
- **数据库连接池**：观察 `pg_stat_activity`

### 8.3 日志

Go 服务日志输出到 stdout/stderr，GIN 框架自动记录每个请求的状态码和延迟。

生产环境建议：

```bash
# 重定向到文件
./server >> /var/log/git-ai/server.log 2>&1

# 或配合 journald（systemd）
journalctl -u git-ai -f

# 或配合 Docker
docker logs -f git-ai-server
```

---

## 9. 从 Node.js 版迁移

### 9.1 全新部署（推荐）

Go 版本使用 snake_case 列名，与 Node.js 版的 Prisma camelCase 列名不同。**全新部署不共享数据库**。

```
Node.js (server/)  → PostgreSQL A (现有)
Go (server-go/)    → PostgreSQL B (全新)
```

### 9.2 迁移步骤

1. **部署 Go 版本**到新数据库，启动后自动建表
2. **配置 DNS/负载均衡**灰度切换（5% → 20% → 50% → 100%）
3. **客户端指向新地址**：`export GIT_AI_API_BASE_URL=https://git-ai-go.your-company.com`
4. **观察 1-2 周**，确认稳定后退役 Node.js 版本
5. **清理**：关闭旧服务，归档旧数据库

### 9.3 回滚方案

- DNS/负载均衡切回 Node.js 版本
- 客户端无需修改（Token 格式兼容，相同 `JWT_SECRET` 签发的 token 两个版本互通）

---

## 10. 安全加固

### 10.1 网络

- [ ] 服务仅监听内网 IP，通过反向代理暴露
- [ ] 数据库仅允许服务 IP 连接
- [ ] 启用 `DB_SSL=true` + `DB_SSL_REJECT_UNAUTHORIZED=true`
- [ ] 设置 `TRUST_PROXY=1`（反向代理后）
- [ ] 设置 `CORS_ORIGIN` 为确切域名（不用 `*`）

### 10.2 密钥

- [ ] `JWT_SECRET` ≥ 32 字符
- [ ] `ENCRYPTION_MASTER_KEY` 恰好 64 位 hex
- [ ] 密钥通过 Vault / Secrets Manager 注入，不写入磁盘
- [ ] `.env` 文件权限 `chmod 600`

### 10.3 运行时

- [ ] 非 root 用户运行
- [ ] systemd `NoNewPrivileges=yes` + `ProtectSystem=strict`
- [ ] Docker distroless 镜像（无 shell）
- [ ] 资源限制（内存 256MB / CPU 1 核足够）

---

## 11. 容量规划

| 规模 | 用户数 | 服务器配置 | 数据库配置 | 存储 |
|------|--------|-----------|-----------|------|
| 小型 | ≤20 | 2 核 4GB | 2 核 4GB | 50GB |
| 中型 | 20-100 | 4 核 8GB | 4 核 8GB | 200GB |
| 大型 | 100+ | 8 核 16GB × 2 | 8 核 16GB（主从） | 500GB+ |

Go 版本内存占用极低（30-50MB），小型部署可与数据库共享同一台机器。

---

## 12. 故障排查

| 现象 | 可能原因 | 排查 |
|------|---------|------|
| 启动失败 `JWT_SECRET must be set` | 环境变量未设置 | 检查 `.env` 或 systemd EnvironmentFile |
| 启动失败 `ENCRYPTION_MASTER_KEY must be set in production` | 生产环境未设密钥 | `openssl rand -hex 32` 生成并设置 |
| 启动失败 `failed to open database` | 数据库连接失败 | 检查 DB_HOST/PORT/USER/PASSWORD，pg_isready |
| 启动失败 `bind: address already in use` | 端口被占用 | `lsof -i :3000` 查看占用进程 |
| `/api/health/database` 返回 503 | 数据库断连 | 检查 PostgreSQL 服务状态 |
| CAS 读取失败 | 密钥不匹配 | 确认 `CAS_ENCRYPTION_KEY` 与写入时一致 |
| Config 值全是 `[REDACTED]` | 正常行为 | 敏感配置读取时自动脱敏 |
| Token 验证失败 | JWT_SECRET 不一致 | 多实例必须用相同 JWT_SECRET |
