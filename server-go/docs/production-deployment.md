# Git-AI Go Server — 生产环境部署手册

> 版本: 2.0 · 更新日期: 2026-05-07 · 适用分支: `server-feature`
>
> 本手册以 `server-go/scripts/deploy.sh` 为权威部署路径（裸机 + systemd），其它部署形态（Docker Compose、K8s）见附录 A/B。

---

## 0. 部署素材与权威路径

仓库里描述部署的素材：

| 来源 | 内容 | 现状 |
|---|---|---|
| `server-go/scripts/deploy.sh` | build/install/upgrade/status | **权威**，本手册以它为准 |
| `server-go/deploy/nginx.conf` | nginx 反代 + P1 限流 | 与生产一致，按 §7 替换占位即可 |
| `server-go/deploy/opt-git-ai.env` | `.env` 字段参考 | 与 `deploy.sh` 已对齐（PORT=3000） |
| `server-go/README.md`（裸机段） | 早期手工流程 | 仅作为概念参考；新部署一律走 `deploy.sh` |

`deploy.sh` 默认 `.env` 是开发取向（`CORS_ORIGIN=*`、`DB_PASSWORD=` 空、`DEFAULT_USER_ROLE` 不显式给），上生产前必须按 §4.4 改掉。

---

## 1. 概览

Go 服务是 `server/`（Node.js）的对等重写：

- 单二进制（CGO_ENABLED=0），无运行时依赖
- 内存 ~30–50 MB，冷启动 < 1 秒
- 启动时自动跑 SQL 迁移（嵌入二进制）
- 部署形态：**systemd 守护 + nginx 反代 + 本机 PostgreSQL**

P1 安全基线（2026-04-23 已合入 `server-feature`，commits `c48ea9b7` → `37d767ca`）：

- 所有涉及个人数据的 `/api/*` 接口已上鉴权
- 设备授权流程、登录路由有 nginx 限流
- 5xx 错误体已脱敏，不再回显原始 err
- 请求体大小受 `JSON_BODY_LIMIT` / `CAS_UPLOAD_LIMIT` 限制
- `X-Forwarded-*` 仅在 `TRUST_PROXY` 启用时被读取
- 详见 §11。

### 1.1 运行时拓扑

```
        443 / 80
            │
            ▼
   ┌───────────────────┐
   │ nginx             │  限流: gitai_auth zone (10r/m)
   │ deploy/nginx.conf │   ├─ /api/user/login
   │                   │   └─ /workers?/oauth/device/code
   │ upstream → :3000  │  body: client_max_body_size 10m
   └─────────┬─────────┘  TLS: Let's Encrypt
             │
             ▼
   ┌───────────────────────────────────────┐
   │ git-ai.service (systemd)              │
   │   User=devops, Group=devops           │
   │   EnvironmentFile=/opt/git-ai/.env    │
   │   ExecStart=/opt/git-ai/server-go/    │
   │           current/git-ai-server       │
   │   Sandbox: ProtectSystem=strict,      │
   │            ProtectHome=yes,           │
   │            NoNewPrivileges=yes        │
   │   Restart=always                      │
   └─────────┬─────────────────────────────┘
             │ 127.0.0.1:5432
             ▼
   ┌───────────────────────────────────────┐
   │ PostgreSQL 16+ (本机)                  │
   │ database = git_ai                     │
   │ 启动时自动迁移建表                     │
   └───────────────────────────────────────┘
```

---

## 2. 前置条件

### 2.1 主机

- Ubuntu 24.04 LTS（已验证）；其它 systemd Linux 发行版同理
- 可访问外网（`go build` 拉模块、release 同步访问 GitHub API）
- 反向代理域名 + TLS 证书（Let's Encrypt 推荐）

### 2.2 软件

| 组件 | 版本 | 用途 |
|---|---|---|
| Go | 1.26.x（仅构建机） | 编译二进制，目标机不需要 |
| PostgreSQL | 16+（18.x 可用） | 主数据库 |
| nginx | 任何当前 LTS | 反向代理 + 限流 |
| openssl | 任何 | 生成密钥 |
| jq, curl | 任何 | release 同步脚本依赖 |

### 2.3 系统用户

`deploy.sh` 默认要求名为 `devops` 的用户/组**已存在**：

```bash
# 若不存在
sudo useradd --system --shell /usr/sbin/nologin --home-dir /opt/git-ai devops
sudo groupadd -f devops
sudo usermod -a -G devops devops
```

> 想换名字（例如 `git-ai`），改 `scripts/deploy.sh` 顶部的 `DEPLOY_USER` / `DEPLOY_GROUP` 后再 install。

### 2.4 密钥（投产前生成 + 离线保存）

```bash
openssl rand -base64 48     # → JWT_SECRET
openssl rand -hex 32        # → ENCRYPTION_MASTER_KEY（必须恰好 64 位 hex）
openssl rand -base64 48     # → CAS_ENCRYPTION_KEY
openssl rand -base64 48     # → RELEASE_UPLOAD_TOKEN（启用 release 同步时）
```

> ⚠️ **`ENCRYPTION_MASTER_KEY` 与 `CAS_ENCRYPTION_KEY` 一旦投产后不可更换**，否则已加密的 CAS 数据和系统配置无法解密。请同时存到 Vault / Secrets Manager / 密封信封。

---

## 3. 构建（构建机）

### 3.1 标准构建（Linux amd64）

```bash
cd server-go
bash scripts/deploy.sh build
```

等价命令：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o bin/git-ai-server ./cmd/server
```

产出 `bin/git-ai-server`，约 25–30 MB。

### 3.2 ARM64

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -ldflags="-s -w" -o bin/git-ai-server-arm64 ./cmd/server
```

### 3.3 构建前检查

```bash
cd server-go
go vet ./...
bash scripts/smoke-test.sh   # 需要本机 PostgreSQL，启一个临时 server
```

---

## 4. 首次部署

### 4.1 准备数据库

```bash
# 在目标机
sudo -u postgres psql -c "CREATE USER git_ai WITH PASSWORD '<密码>'"
sudo -u postgres psql -c "CREATE DATABASE git_ai OWNER git_ai"
```

或用脚本辅助：

```bash
sudo bash scripts/deploy.sh init-db   # 仅 createdb，不建用户
```

> 表结构在服务首次启动时自动迁移，不需要手工建表。

### 4.2 把构建产物和 deploy.sh 传到目标机

```bash
# 在构建机
scp bin/git-ai-server user@target:/tmp/
scp scripts/deploy.sh user@target:/tmp/
```

### 4.3 在目标机执行 install

```bash
# 把二进制和脚本放到同目录（脚本会沿目录查找二进制）
sudo bash /tmp/deploy.sh install
```

`install` 干的事：

1. 校验 `devops` 用户存在
2. 创建 `/opt/git-ai/server-go/current/`、`/opt/git-ai/logs/`
3. 优雅停掉旧服务（如有）
4. `cp` 二进制到 `/opt/git-ai/server-go/current/git-ai-server`
5. **首次**：用 `openssl rand` 生成密钥，写入 `/opt/git-ai/.env`（`chmod 600`）
6. `chown -R devops:devops /opt/git-ai`
7. 写 `/etc/systemd/system/git-ai.service`，`daemon-reload`、`enable`
8. `systemctl start git-ai`
9. `curl /health`、`/api/health/database` 验证

### 4.4 修正自动生成的 .env

`deploy.sh` 默认值偏开发，**上生产前必须改**：

```bash
sudo nano /opt/git-ai/.env
```

需要改的字段：

```env
APP_ENV=production
CORS_ORIGIN=https://git-ai.your-company.com  # 不要留 *
DB_USER=git_ai                               # 默认是 postgres
DB_PASSWORD=<上一步设置的密码>
TRUST_PROXY=1                                # 反向代理后保持 1
HTTPS_REDIRECT=false                         # 由 nginx 处理 80→443
JSON_BODY_LIMIT=2mb                          # 默认；放心用
CAS_UPLOAD_LIMIT=10mb                        # CAS 上传限额
DEFAULT_USER_ROLE=admin                      # 若 X-API-Key 想当 admin 通道，否则保持 user

# 启用 release 同步时
RELEASE_STORAGE_PATH=/opt/git-ai/releases
RELEASE_UPLOAD_TOKEN=<另一个 openssl rand>
```

> `PORT` 默认 `3000`，已与 `deploy/nginx.conf` 的 upstream 对齐，无需手动改。

改完重启：

```bash
sudo systemctl restart git-ai
sudo journalctl -u git-ai -n 50 --no-pager
```

确认日志里：

```
Application is running on: http://localhost:3000
Environment: production
Database target: postgres://127.0.0.1:5432/git_ai
Trust proxy: 1
```

### 4.5 配置 nginx

```bash
sudo cp server-go/deploy/nginx.conf /etc/nginx/sites-available/git-ai
sudo ln -sf /etc/nginx/sites-available/git-ai /etc/nginx/sites-enabled/
# 改掉 server_name + ssl_certificate 路径
sudo nano /etc/nginx/sites-available/git-ai
sudo nginx -t && sudo systemctl reload nginx
```

详见 §7。

### 4.6 端到端验证

```bash
curl https://git-ai.your-company.com/health
curl https://git-ai.your-company.com/api/health/database

# 限流验证（连续打 6 次以上，应看到 429）
for i in 1 2 3 4 5 6 7 8; do
  curl -s -o /dev/null -w "%{http_code}\n" \
       https://git-ai.your-company.com/api/user/login \
       -H 'content-type: application/json' \
       -d '{"username":"x","password":"y"}'
done
```

---

## 5. 升级

### 5.1 deploy.sh upgrade（推荐）

```bash
# 构建机
cd server-go
bash scripts/deploy.sh build
scp bin/git-ai-server user@target:/tmp/
scp scripts/deploy.sh user@target:/tmp/

# 目标机
sudo bash /tmp/deploy.sh upgrade
```

`upgrade` 流程：

1. 找新二进制
2. 备份当前为 `/opt/git-ai/server-go/current/git-ai-server.bak`
3. `graceful_stop`：SIGTERM，最多等 15 秒，超时 SIGKILL
4. `cp` 新二进制 + `chown devops:devops`
5. `systemctl start git-ai`
6. `curl /health`：成功即完成
7. **失败自动回滚**：`cp .bak` 回原位 + restart

### 5.2 手工回滚

如果新版本上线后才发现行为问题（健康检查通过但接口异常），手工回滚：

```bash
sudo systemctl stop git-ai
sudo cp /opt/git-ai/server-go/current/git-ai-server.bak \
        /opt/git-ai/server-go/current/git-ai-server
sudo systemctl start git-ai
```

> `upgrade` 每次都会用当前版覆盖 `.bak`，所以**只能回滚一次**。需要更长的版本历史就用 git tag + 自己留 archive。

### 5.3 多版本灰度

`deploy.sh` 不直接支持。需要：

- 起第二个 systemd unit 在另一端口
- 用 nginx `split_clients` 或 weight 灰度

这超出 `deploy.sh` 范围，按需自己写。

---

## 6. 运行时配置（.env 完整字段）

### 6.1 必填

| 变量 | 说明 |
|---|---|
| `JWT_SECRET` | JWT 签名密钥；≥32 字符；多实例必须一致 |
| `ENCRYPTION_MASTER_KEY` | **64 位 hex**（32 字节）；config 表加密；不可更换 |
| `CAS_ENCRYPTION_KEY` | CAS 内容加密；不可更换 |

### 6.2 网络

| 变量 | 默认 | 说明 |
|---|---|---|
| `PORT` | `3000` | 监听端口；与 `deploy/nginx.conf` upstream 对齐 |
| `APP_ENV` | `development` | 生产必须 `production` |
| `CORS_ORIGIN` | `*` | 收紧到具体域名；P1 §7 后 `*` 时不发 `Allow-Credentials` |
| `TRUST_PROXY` | `false` | nginx 反代后必须 `1` 或 `true`；否则 P1 §4 会忽略 `X-Forwarded-*` |
| `HTTPS_REDIRECT` | `false` | 由 nginx 处理 80→443 时保持 false |
| `JSON_BODY_LIMIT` | `2mb` | P1 §6 全局 body 限额；支持 `b/kb/mb/gb` |
| `CAS_UPLOAD_LIMIT` | `10mb` | CAS 上传专用限额 |

### 6.3 数据库

| 变量 | 默认 | 说明 |
|---|---|---|
| `DATABASE_URL` | — | 如设置则覆盖下列字段 |
| `DB_HOST` | `127.0.0.1` | |
| `DB_PORT` | `5432` | |
| `DB_USER` | `postgres` | 推荐独立账号 `git_ai` |
| `DB_PASSWORD` | 空 | 生产必须设 |
| `DB_NAME` | `git_ai` | |
| `DB_SSL` | `false` | 跨主机连接建议 `true` |
| `DB_SSL_REJECT_UNAUTHORIZED` | `false` | `DB_SSL=true` 时建议 `true` |

### 6.4 默认用户（私有部署兼容）

| 变量 | 默认 |
|---|---|
| `DEFAULT_USER_ID` | `00000000-0000-0000-0000-000000000001` |
| `DEFAULT_USER_EMAIL` | `git-ai@example.local` |
| `DEFAULT_USER_NAME` | `Git AI User` |
| `DEFAULT_USER_ROLE` | `user` |
| `DEFAULT_PERSONAL_ORG_ID` | — |
| `DEFAULT_ORG_NAME` | — |
| `DEFAULT_ORG_SLUG` | — |
| `INITIAL_ADMIN_USERNAME` | `admin` |
| `INITIAL_ADMIN_PASSWORD` | 空（不创建） |

> **注意**（P1 §1）：worker 路由用 `X-API-Key` 通过时，身份解析为 `DEFAULT_USER_ID + DEFAULT_USER_ROLE`。想让 API Key 当 admin 通道，必须把 `DEFAULT_USER_ROLE` 设为 `admin`。

### 6.5 API Key（worker 路由）

| 变量 | 说明 |
|---|---|
| `GIT_AI_API_KEY` | 主 key |
| `GIT_AI_API_KEYS` | 可选：逗号分隔的额外 key（轮换期间用） |

### 6.6 Release 同步

| 变量 | 说明 |
|---|---|
| `RELEASE_STORAGE_PATH` | 默认 `/opt/git-ai/releases`；改路径需同步 systemd `ReadWritePaths` |
| `RELEASE_UPLOAD_TOKEN` | 不设则 `PUT /api/releases/...` 返回 503 |
| `PRIVATE_SERVER` | sync-releases.sh 用，私服公网域名 |
| `GITHUB_TOKEN` | 可选；不设走匿名 GitHub API（60/小时/IP） |
| `GITHUB_REPO` | 默认 `git-ai-project/git-ai`；fork 时覆盖 |

---

## 7. nginx 反向代理

### 7.1 部署文件

仓库里 `server-go/deploy/nginx.conf` 是当前权威配置（P1 §11 限流已合入）。结构：

```
upstream git_ai_backend → 127.0.0.1:3000
limit_req_zone gitai_auth: 10r/m, burst=5  ← 全局共享限流桶
80 → 443 重定向
443:
  TLS + HSTS + 安全头
  client_max_body_size 10m
  location = /api/user/login        → 限流 + proxy
  location ~ ^/workers?/oauth/device/code$ → 限流 + proxy
  location /                         → proxy
  location = /health                 → proxy + access_log off
```

### 7.2 替换占位

```bash
sudo cp server-go/deploy/nginx.conf /etc/nginx/sites-available/git-ai
sudo sed -i 's/git-ai\.example\.com/git-ai.your-company.com/g' \
    /etc/nginx/sites-available/git-ai
```

`ssl_certificate` 路径按 Let's Encrypt 实际位置改。

### 7.3 release 制品 PUT 放大 body（按需）

启用 release 同步时，PUT 制品可能超过 10m。在 443 server 块内加：

```nginx
location /api/releases/ {
    client_max_body_size 200m;
    proxy_request_buffering off;
    proxy_buffering         off;
    proxy_read_timeout      300s;
    proxy_send_timeout      300s;
    proxy_pass              http://git_ai_backend;

    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header Authorization     $http_authorization;
}
```

### 7.4 启用 + 校验

```bash
sudo nginx -t
sudo systemctl reload nginx
```

---

## 8. systemd 单元与文件布局

### 8.1 deploy.sh install 写出的单元（权威）

```ini
[Unit]
Description=Git-AI Go Server
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=devops
Group=devops
WorkingDirectory=/opt/git-ai/server-go/current
EnvironmentFile=/opt/git-ai/.env
ExecStart=/opt/git-ai/server-go/current/git-ai-server
ExecStartPre=/usr/bin/test -x /opt/git-ai/server-go/current/git-ai-server
ExecStartPre=/usr/bin/test -f /opt/git-ai/.env
Restart=always
RestartSec=5
TimeoutStartSec=30
TimeoutStopSec=15
KillSignal=SIGTERM
LimitNOFILE=65536
UMask=0077
SyslogIdentifier=git-ai
StandardOutput=journal
StandardError=journal
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
ReadWritePaths=/opt/git-ai/logs

[Install]
WantedBy=multi-user.target
```

> 启用 release 同步时，把 `ReadWritePaths` 改为 `ReadWritePaths=/opt/git-ai/logs /opt/git-ai/releases`，否则 `ProtectSystem=strict` 会拦截写入。

### 8.2 文件布局

```
/opt/git-ai/
├── .env                              # 600，devops:devops
├── logs/                             # ReadWritePaths
├── releases/                         # 启用 release 同步时
│   ├── latest/
│   │   ├── current.json
│   │   ├── SHA256SUMS
│   │   └── git-ai-vX.Y.Z-*.tar.gz
│   └── next/
├── scripts/
│   └── sync-releases.sh              # install -m 0755
└── server-go/
    └── current/
        ├── git-ai-server             # 当前二进制
        └── git-ai-server.bak         # 上一次升级前备份（仅一份）
```

### 8.3 常用命令

```bash
sudo systemctl status git-ai
sudo systemctl restart git-ai
sudo journalctl -u git-ai -f
sudo journalctl -u git-ai --since "1 hour ago" -p warning

# 查看实际生效的单元（确认 deploy.sh 改没改）
systemctl cat git-ai
```

---

## 9. Release 同步（sync-releases.sh）

`scripts/sync-releases.sh` 把 GitHub Releases 上的 `SHA256SUMS`、`install.sh`、`install.ps1` 拉到本地，再通过 `PUT /api/releases/:channel/...` 推送到私服 `RELEASE_STORAGE_PATH`，并写 `current.json` 作为版本指针。脚本**幂等**（`current.json.tag` 已等于最新 tag 时 no-op），**不会**被 server-go 自动触发，必须外部调度。

### 9.1 落盘

```bash
sudo apt-get install -y jq curl

sudo install -o devops -g devops -m 0755 \
    server-go/scripts/sync-releases.sh \
    /opt/git-ai/scripts/sync-releases.sh

sudo mkdir -p /opt/git-ai/releases
sudo chown devops:devops /opt/git-ai/releases
sudo chmod 750 /opt/git-ai/releases
```

如果 `RELEASE_STORAGE_PATH` 不是默认值，把实际路径加入 systemd `ReadWritePaths`。

### 9.2 首次手工冒烟

```bash
# 用服务用户身份跑，确认权限一致
sudo -u devops env $(grep -v '^#' /opt/git-ai/.env | xargs) \
     /opt/git-ai/scripts/sync-releases.sh

ls /opt/git-ai/releases/latest/
curl -s https://git-ai.your-company.com/api/releases/latest/current.json
```

预期：两个 channel（`latest` / `next`）各打印 `synced <none> -> vX.Y.Z` 或 `already at ...`。

失败排查顺序：

1. `Authorization: Bearer <token>` 前缀漏掉 → 401
2. `RELEASE_UPLOAD_TOKEN` 未设 → 503
3. nginx `client_max_body_size` < 制品大小 → 413
4. `/opt/git-ai/releases` owner 不是 `devops` 或未 `ReadWritePaths` → systemd 拒写

### 9.3 调度（推荐 systemd timer）

```ini
# /etc/systemd/system/git-ai-sync.service
[Unit]
Description=Sync git-ai releases from GitHub
After=network-online.target git-ai.service
Wants=network-online.target

[Service]
Type=oneshot
User=devops
Group=devops
EnvironmentFile=/opt/git-ai/.env
ExecStart=/opt/git-ai/scripts/sync-releases.sh
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/git-ai/releases
```

```ini
# /etc/systemd/system/git-ai-sync.timer
[Unit]
Description=Run git-ai release sync every 15 minutes

[Timer]
OnBootSec=2min
OnUnitActiveSec=15min
AccuracySec=30s
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now git-ai-sync.timer
systemctl list-timers git-ai-sync.timer
journalctl -u git-ai-sync.service --since "30 min ago"
```

频率 5–15 分钟。匿名 GitHub API 60 次/小时/IP，多节点或高频时**必须**设 `GITHUB_TOKEN`。

### 9.4 回滚

- **暂停同步**：`systemctl disable --now git-ai-sync.timer`
- **关闭上传端点**：`.env` 移除 `RELEASE_UPLOAD_TOKEN` + `systemctl restart git-ai`，所有 PUT 立即返回 503
- **回退到旧版本**：手工编辑 `/opt/git-ai/releases/:channel/current.json` 指回旧 tag（旧 artifacts 目录需仍存在）

---

## 10. 数据库管理

### 10.1 自动迁移

服务启动时执行 `golang-migrate`，迁移文件嵌入二进制。

### 10.2 备份

```bash
# 每日
pg_dump -U git_ai -h 127.0.0.1 -d git_ai -Fc \
    > /var/backups/git-ai/backup_$(date +%Y%m%d).dump

# 恢复
pg_restore -U git_ai -h 127.0.0.1 -d git_ai -c \
    /var/backups/git-ai/backup_20260507.dump
```

### 10.3 表（7 张）

| 表 | 说明 | 增长 |
|---|---|---|
| `authorship_records` | 作者归属记录 | 随 commit |
| `commit_attributions` | 提交归属统计 | 随 commit |
| `cas_entries` | CAS 加密存储 | 最大，按 prompt 增长 |
| `config` | 系统配置 | <100 行 |
| `oauth_device_codes` | 设备授权码 | <10 行（自动清理） |
| `metrics_events` | 遥测事件 | 增长最快 |
| `audit_logs` | 审计日志 | 持续增长 |

### 10.4 维护

```sql
DELETE FROM audit_logs      WHERE occurred_at    < now() - interval '90 days';
DELETE FROM metrics_events  WHERE event_timestamp < now() - interval '180 days';
VACUUM ANALYZE metrics_events;
VACUUM ANALYZE audit_logs;
```

P1 §3 后 `authorship.user_id` / `git_commit_hash` 已从 `LIKE` 改为 `=`，命中索引；如果有自建索引检查一下是否还需要保留 trigram。

---

## 11. P1 安全基线（2026-04-23 后）

设计文档：`docs/superpowers/specs/2026-04-23-server-go-p1-security-patch-design.md`

| 章节 | 提交 | 落地内容 |
|---|---|---|
| §1 鉴权挂载 | `831c73df` | `/api/cas/*`、`/api/authorship/*`、`/api/dashboard/stats`、`/api/bundles` 全部要求 JWT 或 API Key；`/api/dashboard/public` 仍开放 |
| §2 PII 日志 | `c48ea9b7` | device flow 不再打印 user_code/sub/email/name |
| §3 LIKE→= | `c48ea9b7` | authorship 查询精确匹配，避免 user_id 跨边界 |
| §4 trustProxy 网关 | `683cdcd3` | `X-Forwarded-For/X-Real-Ip/X-Forwarded-Proto` 仅在 `TRUST_PROXY=1` 时生效 |
| §5 错误脱敏 | `683cdcd3` | 5xx 一律返回 `{"error":"internal server error"}`，原始 err 只进 server log；4xx 不变 |
| §6 body 限流 | `683cdcd3` | 全局 `JSON_BODY_LIMIT`（2mb 默认）；`/api/cas` + `/worker/cas/upload` 用 `CAS_UPLOAD_LIMIT`（10mb 默认）；超限 413 |
| §7 CORS | `683cdcd3` | `CORS_ORIGIN=*` 时不发 `Access-Control-Allow-Credentials`（浏览器拒绝该组合） |
| §8 device poll TOCTOU | `c48ea9b7` | 单条 `UPDATE ... WHERE last_polled_at < now() - interval` 原子化轮询 rate limit |
| §9 CAS 错误语义 | `c48ea9b7` | `pgx.ErrNoRows → 404`；其它 DB 错 → 500，不再被吞为 not found |
| §10 smoke-test | `831c73df` | 脚本带 token 调所有受保护路由 + 增加 401/403 负向断言 |
| §11 nginx 限流 | `37d767ca` | `gitai_auth` zone：登录 + 设备授权初始化 10r/m，burst 5，超限 429 |

### 11.1 已知偏离

- `GET /api/authorship/commit/:commitHash`：设计是 "owner or admin"，但 `commit_attributions` 表无 `user_id` 字段，**当前降级为 admin-only**（`831c73df` commit message 里有说明）。

### 11.2 上线前自查

- [ ] `.env` 的 `CORS_ORIGIN` 是确切域名，不是 `*`
- [ ] `TRUST_PROXY=1`（nginx 反代后）
- [ ] `RELEASE_UPLOAD_TOKEN` 已设（启用 release 同步）或刻意不设（关闭上传端点）
- [ ] nginx limit_req_zone 在 conf 顶部，被 reload 加载（`nginx -t` 通过）
- [ ] smoke-test.sh 全绿

---

## 12. 健康检查与日志

### 12.1 端点

| 端点 | 用途 | nginx 透传 | 建议 |
|---|---|---|---|
| `GET /health` | liveness（不查 DB） | 是，access_log off | 30s |
| `GET /api/health/database` | readiness | 是 | 10s |
| `GET /api/version` | 返回 `version` + `commit`（构建时注入） | 是 | 部署对账 |

### 12.2 日志

```bash
# 实时
sudo journalctl -u git-ai -f

# 错误聚焦
sudo journalctl -u git-ai -p warning --since today

# nginx
sudo tail -f /var/log/nginx/git-ai.access.log /var/log/nginx/git-ai.error.log
```

GIN 框架自动记录每个请求的状态码和延迟。P1 §5 之后所有 5xx 的原始 err 都进 server log（`[handler] METHOD PATH: <err>`），不再回客户端。

### 12.3 监控（最小可用）

- `git-ai-server` 进程内存：`systemctl status git-ai`
- DB 连接：`SELECT count(*) FROM pg_stat_activity WHERE datname = 'git_ai'`
- audit 增量：`SELECT date_trunc('hour', occurred_at), count(*) FROM audit_logs ...`
- nginx 429 速率：`grep ' 429 ' /var/log/nginx/git-ai.access.log | wc -l`

P2 计划接入 OTEL/Prom，本轮不在范围。

---

## 13. 版本可追溯

`scripts/deploy.sh build` 在编译时通过 `-ldflags="-X main.commitHash=$(git rev-parse --short HEAD)"` 注入构建时的 git commit。运行后：

```bash
curl -s https://git-ai.your-company.com/api/version
# {"version":"1.0.0","commit":"37d767ca","service":"git-ai-private-deploy-server"}
```

`commit` 字段是判定线上版本的唯一可靠依据。`version` 字段保留原值是为了兼容现有客户端 / 冒烟脚本断言。

构建时若不在 git 仓库内（例如 tarball 构建）会回退为 `commit: "unknown"`；本机 `go run ./cmd/server` 启动则是 `commit: "dev"`。

---

## 14. 故障排查

| 现象 | 可能原因 | 排查 |
|---|---|---|
| 启动失败 `JWT_SECRET must be set` | `.env` 没生效 | `systemctl cat git-ai` 看 `EnvironmentFile` 路径；`stat /opt/git-ai/.env`（应 600 / devops） |
| 启动失败 `ENCRYPTION_MASTER_KEY must be set in production` | 生产没设密钥 | 重新 `openssl rand -hex 32` 写入 |
| 启动失败 `bind: address already in use` | 端口冲突 | `lsof -i :3000` |
| nginx 502 | upstream PORT 与 `.env` `PORT` 不一致 | 默认两边都是 3000；如手动改过务必同步 |
| `/api/health/database` 503 | DB 断连 | `pg_isready`；查 `DB_*` 配置 |
| 请求 401 | P1 §1 鉴权挂上后客户端没带 token | 走 `/api/user/login` 拿 access_token，或带 `X-API-Key` |
| 请求 413 | body 超 `JSON_BODY_LIMIT` / `CAS_UPLOAD_LIMIT` | 调大 `.env` 对应值 + 重启 |
| 请求 429 | nginx `gitai_auth` 限流 | 是登录/设备授权？降低调用频率，或临时调 `rate=30r/m` |
| 客户端拿到 `internal server error` 没细节 | P1 §5 错误脱敏 | `journalctl -u git-ai -p err --since "5 min ago"` 拿原始 err |
| device 轮询永远 `slow_down` | P1 §8 后 1 秒内重复请求会被拒 | 客户端按返回的 `interval` 退避 |
| `X-Forwarded-For` 没生效 | `TRUST_PROXY` 没开 | `.env` 设 `TRUST_PROXY=1` 重启 |
| Token 跨实例验证失败 | `JWT_SECRET` 不一致 | 多实例必须共享同一 `JWT_SECRET` |
| CAS 读取错误 | 密钥不一致 | `CAS_ENCRYPTION_KEY` 必须与写入时相同；不可更换 |
| Config 值 `[REDACTED]` | 正常 | 敏感配置读取自动脱敏 |

---

## 15. 容量规划

| 规模 | 用户 | 服务器 | 数据库 | 存储 |
|---|---|---|---|---|
| 小型 | ≤20 | 2c4g | 2c4g（合机） | 50GB |
| 中型 | 20–100 | 4c8g | 4c8g（独立） | 200GB |
| 大型 | 100+ | 8c16g × 2 | 8c16g 主从 | 500GB+ |

Go 服务进程内存 30–50MB，小型部署可与 PostgreSQL 共享同机。

---

## 16. 卸载

```bash
sudo systemctl stop git-ai
sudo systemctl disable git-ai
sudo systemctl stop git-ai-sync.timer 2>/dev/null
sudo systemctl disable git-ai-sync.timer 2>/dev/null

sudo rm /etc/systemd/system/git-ai.service
sudo rm -f /etc/systemd/system/git-ai-sync.{service,timer}
sudo systemctl daemon-reload

# 仅删二进制 / 配置（保留 logs/releases）
sudo rm -rf /opt/git-ai/server-go
sudo rm /opt/git-ai/.env

# 完全清理
# sudo rm -rf /opt/git-ai

# 数据库（不可逆）
# sudo -u postgres psql -c "DROP DATABASE git_ai"
# sudo -u postgres psql -c "DROP USER git_ai"

# nginx
sudo rm /etc/nginx/sites-enabled/git-ai
sudo rm /etc/nginx/sites-available/git-ai
sudo nginx -t && sudo systemctl reload nginx
```

---

## 附录 A · Docker Compose 参考

非主要部署路径，仅作备用。

```yaml
services:
  server:
    image: your-registry.com/git-ai-server:latest
    ports: ["3000:3000"]
    environment:
      PORT: "3000"
      APP_ENV: production
      JWT_SECRET: "${JWT_SECRET}"
      ENCRYPTION_MASTER_KEY: "${ENCRYPTION_MASTER_KEY}"
      CAS_ENCRYPTION_KEY: "${CAS_ENCRYPTION_KEY}"
      DB_HOST: db
      DB_USER: git_ai
      DB_PASSWORD: "${DB_PASSWORD}"
      DB_NAME: git_ai
      DB_SSL: "false"
      CORS_ORIGIN: "https://git-ai.your-company.com"
      TRUST_PROXY: "1"
      JSON_BODY_LIMIT: "2mb"
      CAS_UPLOAD_LIMIT: "10mb"
    depends_on:
      db: { condition: service_healthy }
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:3000/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: git_ai
      POSTGRES_USER: git_ai
      POSTGRES_PASSWORD: "${DB_PASSWORD}"
    volumes: [pgdata:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U git_ai"]
      interval: 10s
      retries: 5
    restart: unless-stopped

volumes:
  pgdata:
```

构建镜像：`docker build -t git-ai-server:<tag> .`，镜像基于 `gcr.io/distroless/static-debian12`，~20MB。

---

## 附录 B · Kubernetes 参考

非主要部署路径。Deployment 关键字段：

```yaml
livenessProbe:
  httpGet: { path: /health, port: 3000 }
  initialDelaySeconds: 5
  periodSeconds: 30
readinessProbe:
  httpGet: { path: /api/health/database, port: 3000 }
  initialDelaySeconds: 10
  periodSeconds: 10
resources:
  requests: { memory: "64Mi", cpu: "100m" }
  limits:   { memory: "256Mi", cpu: "500m" }
```

密钥用 `Secret`，普通配置用 `ConfigMap`，通过 `envFrom` 注入。

---

## 附录 C · 从 Node.js 版迁移

Go 版用 snake_case 列名，与 Node.js 版 Prisma camelCase 不同，**不共享数据库**。

迁移流程：

1. 部署 Go 版到新数据库，启动后自动建表
2. DNS / 负载均衡灰度切换（5% → 20% → 50% → 100%）
3. 客户端指向新地址（`GIT_AI_API_BASE_URL`）
4. 观察 1–2 周后退役 Node.js 版

回滚：DNS 切回旧版，相同 `JWT_SECRET` 签发的 token 两个版本互通。

---

## 修订记录

| 版本 | 日期 | 内容 |
|---|---|---|
| 2.1 | 2026-05-07 | 修掉 PORT 双值（脚本默认改 3000）、删除废弃 systemd 模板 `deploy/git-ai.service`、`/api/version` 注入构建时 commit hash（§13） |
| 2.0 | 2026-05-07 | 重写：以 `scripts/deploy.sh` 为权威路径，加入 P1 安全基线（§11），补 `JSON_BODY_LIMIT` / `CAS_UPLOAD_LIMIT` / `GIT_AI_API_KEY*` 字段，nginx 限流配置同步实际部署文件 |
| 1.0 | 2026-04-09 | 初版 |
