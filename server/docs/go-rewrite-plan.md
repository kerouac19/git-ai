# Server Go 重写详细方案

> 制定日期: 2026-03-31

## 1. 重写范围

### 1.1 当前 TypeScript 代码盘点

| 模块 | 文件数 | 核心职责 | 重写优先级 |
|------|--------|---------|-----------|
| `auth/` | 4 | JWT 策略、OAuth Device Flow、Cookie Session、密钥管理 | P0 |
| `compatibility/` | 2 | Worker 兼容端点（oauth、metrics、cas、health） | P0 |
| `cas/` | 3 | 内容寻址存储（压缩 + AES-256-GCM 加密） | P0 |
| `metrics/` | 3 | 遥测事件批量写入、用户摘要查询 | P0 |
| `authorship/` | 4 | 作者归属记录 CRUD | P1 |
| `dashboard/` | 4 | 聚合指标、仪表板统计 | P1 |
| `config/` | 4 | 系统配置 CRUD（含敏感字段加密） | P1 |
| `security/` | 4 | EncryptionService、AuditLogMiddleware、运行时密钥 | P0 |
| `guards/` | 3 | JwtAuthGuard、PermissionGuard、AuditGuard | P0 |
| `interceptors/` | 1 | EncryptionInterceptor（响应脱敏） | P2 |
| `middleware/` | 1 | HTTPS 重定向 + 安全头 | P2 |
| `prisma/` | 2 | PrismaService（DB 连接管理） | P0（替换为 pgx） |
| `database/` | 3 | 数据库配置、Schema 自动迁移 | P0 |
| `utils/` | 1 | AES-256-GCM 加解密、哈希、HMAC | P0 |
| `main.ts` | 1 | 启动入口、Device Flow HTML 页面、全局配置 | P0 |

### 1.2 数据库 Schema（7 张表）

```
authorship_records      — 作者归属记录
commit_attributions     — 提交归属统计
cas_entries            — 内容寻址存储
config                 — 系统配置
oauth_device_codes     — OAuth 设备授权码
metrics_events         — 遥测事件
audit_logs             — 审计日志
```

### 1.3 API 端点清单（共 ~30 个）

**不带 /api 前缀的端点（直接挂载）：**

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/health` | 无 | 基础健康检查 |
| GET | `/oauth/device` | 无 | 设备授权页面（HTML） |
| POST | `/oauth/device/approve` | 无 | 批准设备授权 |
| POST | `/oauth/device/deny` | 无 | 拒绝设备授权 |
| GET | `/me` | Cookie Session | 用户仪表板页面（HTML） |

**Worker 兼容端点（同时支持 /worker/* 和 /workers/*）：**

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/worker/oauth/device/code` | 无 | 发起 Device Flow |
| POST | `/worker/oauth/token` | 无 | 交换 Token |
| POST | `/worker/metrics/upload` | JWT | 上传遥测事件 |
| POST | `/worker/cas/upload` | JWT | 上传 CAS 对象 |
| GET | `/worker/cas` | JWT | 批量读取 CAS 对象 |
| GET | `/worker/cas/checkout` | JWT | 读取单个 CAS 对象 |

**API 前缀端点（/api/*）：**

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/health` | 无 | API 健康检查 |
| GET | `/api/health/database` | 无 | 数据库连通性检查 |
| GET | `/api/status` | 无 | 服务状态 + 公开统计 |
| GET | `/api/version` | 无 | 版本号 |
| GET | `/api/me` | JWT | 当前用户信息 + 仪表板 |
| POST | `/api/authorship/record` | 无 | 保存归属记录 |
| POST | `/api/authorship/commit` | 无 | 保存提交归属 |
| GET | `/api/authorship/commits/:userId` | 无 | 用户提交列表（分页） |
| GET | `/api/authorship/commits/:userId/:commitHash` | 无 | 特定提交归属 |
| GET | `/api/authorship/commit/:commitHash` | 无 | 按哈希查归属 |
| PUT | `/api/authorship/sync/:userId` | 无 | 同步归属数据 |
| POST | `/api/cas/upload` | 无 | 上传内容 |
| GET | `/api/cas/read/:hash` | 无 | 读取内容 |
| GET | `/api/dashboard/public` | 无 | 公开统计 |
| GET | `/api/dashboard/stats` | 无 | 用户仪表板统计 |
| POST | `/api/dashboard/generate-report` | 无 | 生成报告 |
| GET | `/api/config` | JWT | 配置列表 |
| GET | `/api/config/:key` | JWT | 获取配置 |
| POST | `/api/config` | JWT | 创建配置 |
| PATCH | `/api/config/:key` | JWT | 更新配置 |
| DELETE | `/api/config/:key` | JWT | 删除配置 |

---

## 2. Go 项目结构

```
server-go/
├── cmd/
│   └── server/
│       └── main.go                    # 入口：配置加载、路由注册、启动
├── internal/
│   ├── config/
│   │   └── config.go                  # 环境变量 / 配置文件解析（viper）
│   ├── database/
│   │   ├── postgres.go                # pgx 连接池初始化
│   │   └── migrations/
│   │       ├── embed.go               # embed.FS 嵌入 SQL 文件
│   │       ├── 001_create_tables.up.sql
│   │       ├── 001_create_tables.down.sql
│   │       └── ...
│   ├── auth/
│   │   ├── jwt.go                     # JWT 签发 / 验证（golang-jwt）
│   │   ├── middleware.go              # JWT 认证中间件
│   │   ├── cookie.go                  # Session Cookie 工具函数
│   │   └── device_flow.go             # OAuth Device Flow 逻辑
│   ├── crypto/
│   │   ├── aes.go                     # AES-256-GCM 加解密（标准库）
│   │   ├── hash.go                    # SHA256、HMAC
│   │   └── secrets.go                 # 运行时密钥管理（master key、CAS key）
│   ├── handler/
│   │   ├── health.go                  # /health、/api/health、/api/health/database
│   │   ├── compatibility.go           # /worker/* 兼容端点
│   │   ├── authorship.go              # /api/authorship/*
│   │   ├── cas.go                     # /api/cas/*
│   │   ├── dashboard.go               # /api/dashboard/*
│   │   ├── sysconfig.go               # /api/config/*
│   │   ├── device_page.go             # /oauth/device（HTML 渲染）
│   │   └── me_page.go                 # /me（HTML 仪表板页面）
│   ├── middleware/
│   │   ├── audit.go                   # 审计日志中间件
│   │   ├── security_headers.go        # HSTS、CSP、XSS 等安全头
│   │   └── https_redirect.go          # HTTPS 重定向
│   ├── model/
│   │   ├── authorship.go              # 数据模型
│   │   ├── cas.go
│   │   ├── config.go
│   │   ├── metrics.go
│   │   ├── audit.go
│   │   └── oauth.go
│   ├── repository/                    # 数据访问层（sqlc 生成 + 手写复杂查询）
│   │   ├── queries/
│   │   │   ├── authorship.sql         # sqlc 查询定义
│   │   │   ├── cas.sql
│   │   │   ├── config.sql
│   │   │   ├── metrics.sql
│   │   │   ├── audit.sql
│   │   │   └── oauth.sql
│   │   ├── db.go                      # sqlc 生成
│   │   ├── models.go                  # sqlc 生成
│   │   ├── authorship.sql.go          # sqlc 生成
│   │   ├── cas.sql.go                 # sqlc 生成
│   │   └── ...
│   ├── service/
│   │   ├── authorship.go              # 业务逻辑
│   │   ├── cas.go                     # 含 zlib 压缩 + 加密
│   │   ├── sysconfig.go               # 含敏感字段加密
│   │   ├── metrics.go                 # 含批量写入（pgx.CopyFrom）
│   │   ├── dashboard.go               # 聚合查询
│   │   └── audit.go                   # 审计日志
│   └── templates/
│       ├── device_flow.html           # 设备授权页面模板
│       ├── device_result.html         # 授权结果页面模板
│       ├── dashboard.html             # 用户仪表板模板
│       └── login_required.html        # 登录提示页面模板
├── sqlc.yaml                          # sqlc 配置
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

---

## 3. 技术栈与依赖

```go
// go.mod 核心依赖
module git-ai-server

go 1.23

require (
    github.com/gin-gonic/gin        v1.10.x   // HTTP 框架
    github.com/jackc/pgx/v5         v5.7.x    // PostgreSQL 驱动
    github.com/golang-jwt/jwt/v5    v5.2.x    // JWT
    github.com/golang-migrate/migrate/v4 v4.18.x // 数据库迁移
    github.com/spf13/viper          v1.19.x   // 配置管理
    github.com/google/uuid          v1.6.x    // UUID 生成
)
```

**标准库直接覆盖（零外部依赖）：**
- `crypto/aes` + `crypto/cipher` — AES-256-GCM
- `crypto/sha256` + `crypto/hmac` — 哈希 / HMAC
- `crypto/rand` — 随机数
- `compress/zlib` — 内容压缩
- `html/template` — HTML 页面渲染
- `encoding/json` — JSON 序列化
- `encoding/hex` — 十六进制编解码

---

## 4. 模块映射与重写策略

### 4.1 数据库层

**当前**：Prisma ORM + `pg` 驱动 + `postgres-schema-compat.ts` 自动建表

**Go 方案**：

```
pgx 连接池 + sqlc 代码生成 + golang-migrate 迁移管理
```

**迁移文件生成策略**：

全新部署，不需要兼容 Prisma 遗留的 camelCase 列名，全部统一为 snake_case。从 `schema.prisma` 提取全部 7 张表的 DDL，生成第一个迁移文件 `001_create_tables.up.sql`：

```sql
-- 001_create_tables.up.sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS authorship_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    git_commit_hash TEXT NOT NULL,
    file_attributions TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ai_attribution_percentage DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS commit_attributions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commit_hash TEXT NOT NULL,
    author TEXT NOT NULL,
    file_changes TEXT NOT NULL,
    ai_contribution_metrics TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cas_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hash TEXT NOT NULL UNIQUE,
    encrypted_content TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text/plain',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    value TEXT,
    description TEXT,
    category VARCHAR(100) NOT NULL DEFAULT 'general',
    is_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_config_key ON config (key);
CREATE INDEX IF NOT EXISTS idx_config_category ON config (category);

CREATE TABLE IF NOT EXISTS oauth_device_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_code TEXT NOT NULL UNIQUE,
    user_code TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL DEFAULT 'git-ai-cli',
    verification_uri TEXT NOT NULL,
    status TEXT NOT NULL,
    user_id TEXT NOT NULL,
    subject_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    denied_at TIMESTAMPTZ,
    last_polled_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_user_id ON oauth_device_codes (user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_status ON oauth_device_codes (status);
CREATE INDEX IF NOT EXISTS idx_oauth_device_codes_expires_at ON oauth_device_codes (expires_at);

CREATE TABLE IF NOT EXISTS metrics_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    distinct_id TEXT,
    schema_version INTEGER NOT NULL,
    event_timestamp TIMESTAMPTZ NOT NULL,
    event_id INTEGER NOT NULL,
    values_json JSONB NOT NULL,
    attrs_json JSONB NOT NULL,
    git_ai_version TEXT,
    repo_url TEXT,
    tool TEXT,
    model TEXT,
    prompt_id TEXT,
    external_prompt_id TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_metrics_events_user_id ON metrics_events (user_id);
CREATE INDEX IF NOT EXISTS idx_metrics_events_event_timestamp ON metrics_events (event_timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_events_repo_url ON metrics_events (repo_url);
CREATE INDEX IF NOT EXISTS idx_metrics_events_distinct_id ON metrics_events (distinct_id);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    params_json JSONB NOT NULL,
    ip TEXT NOT NULL,
    user_agent TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    success BOOLEAN NOT NULL,
    details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs (occurred_at);
```

### 4.2 加密层

**当前**：两套加密实现并存
1. `crypto.util.ts` — EncryptionService 用的通用 AES-256-GCM（12 字节 IV）
2. `cas.service.ts` 私有方法 — CAS 专用 AES-256-GCM（Scrypt 派生密钥，16 字节 IV，`iv:authTag:data` 格式）

**Go 方案**：

```go
// internal/crypto/aes.go

// 通用加密（对应 crypto.util.ts）
// 输入：明文 + 32 字节 masterKey
// 输出：{ encryptedData, iv, authTag, algorithm } JSON
func EncryptGeneric(plaintext string, key []byte) (EncryptedPayload, error)
func DecryptGeneric(payload EncryptedPayload, key []byte) (string, error)

// CAS 加密（对应 cas.service.ts 私有方法）
// 输入：明文 + secretKey 字符串
// 输出："iv_hex:authTag_hex:ciphertext_hex" 字符串
// 密钥派生：scrypt(secretKey, "GitAISalt", 32)
func EncryptCAS(plaintext string, secretKey string) (string, error)
func DecryptCAS(encrypted string, secretKey string) (string, error)
```

> **数据兼容性关键点**：Go 实现必须能解密现有 Node.js 加密的数据。两套加密使用不同的 IV 长度（12 vs 16 字节）和不同的密钥派生方式，必须分别实现。

### 4.3 认证层

**当前**：NestJS Passport + JWT + Cookie Session

**Go 方案**：

```go
// internal/auth/jwt.go
type Claims struct {
    jwt.RegisteredClaims
    Email          string   `json:"email"`
    Name           string   `json:"name"`
    PersonalOrgID  string   `json:"personal_org_id"`
    Orgs           []Org    `json:"orgs"`
    Role           string   `json:"role"`
    Type           string   `json:"type"` // "access" | "refresh"
}

func SignAccessToken(subject TokenSubject, secret string, ttl time.Duration) (string, error)
func SignRefreshToken(subject TokenSubject, secret string, ttl time.Duration) (string, error)
func VerifyToken(tokenString string, secret string) (*Claims, error)

// internal/auth/middleware.go — gin 中间件
func JWTAuthMiddleware(secret string) gin.HandlerFunc
// 从 Authorization: Bearer <token> 或 Cookie: git_ai_session=<token> 提取并验证

// internal/auth/device_flow.go — 对应 compatibility-auth.service.ts
type DeviceFlowService struct { pool *pgxpool.Pool; jwtSecret string }
func (s *DeviceFlowService) StartDeviceFlow(baseURL string) (*DeviceCodeResponse, error)
func (s *DeviceFlowService) ExchangeDeviceCode(deviceCode string) (interface{}, error)
func (s *DeviceFlowService) ExchangeRefreshToken(refreshToken string) (interface{}, error)
func (s *DeviceFlowService) ApproveDeviceCode(userCode string) (*DeviceCodeInfo, error)
func (s *DeviceFlowService) DenyDeviceCode(userCode string) (*DeviceCodeInfo, error)
```

**JWT 兼容性**：当前使用 HS256 算法，`JWT_SECRET` 作为对称密钥。Go 端使用相同的 secret 和算法即可。Go 签发的 token 和 Node.js 签发的 token 互相可验证。

### 4.4 Metrics 批量写入优化

**当前**：逐条 `INSERT`（循环中每个 event 一次 `$executeRaw`）

**Go 方案**：使用 `pgx.CopyFrom` 批量写入

```go
// internal/service/metrics.go
func (s *MetricsService) UploadBatch(ctx context.Context, userID string, distinctID *string, batch MetricsBatch) error {
    rows := make([][]interface{}, 0, len(batch.Events))
    for _, event := range batch.Events {
        rows = append(rows, []interface{}{
            userID, distinctID, batch.Version,
            time.Unix(int64(event.T), 0),
            event.E,
            event.V, // jsonb
            event.A, // jsonb
            extractNullableString(event.A, "0"),  // git_ai_version
            extractNullableString(event.A, "1"),  // repo_url
            extractNullableString(event.A, "20"), // tool
            extractNullableString(event.A, "21"), // model
            extractNullableString(event.A, "22"), // prompt_id
            extractNullableString(event.A, "23"), // external_prompt_id
        })
    }

    _, err := s.pool.CopyFrom(ctx,
        pgx.Identifier{"metrics_events"},
        []string{"user_id", "distinct_id", "schema_version", "event_timestamp", "event_id",
                 "values_json", "attrs_json", "git_ai_version", "repo_url",
                 "tool", "model", "prompt_id", "external_prompt_id"},
        pgx.CopyFromRows(rows),
    )
    return err
}
```

### 4.5 CAS 服务

**当前**：zlib 压缩 → AES-256-GCM 加密 → 存入 DB

**Go 方案**：完全对应实现

```go
// internal/service/cas.go
func (s *CasService) UploadObject(ctx context.Context, hash string, content interface{}) (string, error) {
    // 1. JSON 序列化
    // 2. zlib 压缩（compress/zlib）
    // 3. base64 编码
    // 4. AES-256-GCM 加密（CAS 专用加密）
    // 5. INSERT INTO cas_entries (hash, encrypted_content, content_type)
}

func (s *CasService) ReadObject(ctx context.Context, hash string) (interface{}, error) {
    // 1. SELECT FROM cas_entries WHERE hash = $1
    // 2. AES-256-GCM 解密
    // 3. base64 解码
    // 4. zlib 解压
    // 5. JSON 反序列化
}
```

### 4.6 HTML 页面渲染

**当前**：`main.ts` 中内联的 HTML 模板字符串（设备授权页、仪表板页、登录提示页）

**Go 方案**：使用 `html/template` + `embed.FS`

```go
//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
```

将当前 `main.ts` 中 ~230 行 CSS + HTML 提取为独立的 `.html` 模板文件，嵌入到二进制中。

### 4.7 中间件映射

| NestJS 组件 | Go 实现 |
|-------------|---------|
| `JwtAuthGuard` | `gin.HandlerFunc` — JWT 中间件 |
| `PermissionGuard` | `gin.HandlerFunc` — 角色检查中间件 |
| `AuditGuard` + `AuditLogMiddleware` | `gin.HandlerFunc` — 合并为一个审计中间件 |
| `EncryptionInterceptor` | `gin.HandlerFunc` — 响应后处理中间件 |
| `HttpsRedirectMiddleware` | `gin.HandlerFunc` — HTTPS 重定向 + 安全头 |
| `ValidationPipe` | gin 内置 `binding` + `validator` |

---

## 5. 分阶段实施计划

### Phase 0：项目骨架（0.5 天）

- [ ] 初始化 `server-go/` 目录，`go mod init`
- [ ] 配置 gin 路由骨架
- [ ] 配置 viper 环境变量读取（完整对应 `.env.example`）
- [ ] 配置 pgx 连接池
- [ ] 配置 golang-migrate，生成初始迁移文件
- [ ] 配置 sqlc.yaml
- [ ] 基础 Dockerfile（多阶段构建，distroless 运行时）

### Phase 1：核心基础设施（1.5 天）

- [ ] `internal/crypto/` — AES-256-GCM 双模式加密（通用 + CAS）
- [ ] `internal/crypto/` — SHA256 哈希、HMAC、Scrypt 密钥派生
- [ ] `internal/crypto/secrets.go` — 运行时密钥管理（master key、CAS key 的解析和回退逻辑）
- [ ] `internal/auth/jwt.go` — JWT 签发 / 验证
- [ ] `internal/auth/middleware.go` — JWT 中间件（Bearer + Cookie 双通道提取）
- [ ] `internal/auth/cookie.go` — Session Cookie 工具
- [ ] `internal/auth/device_flow.go` — OAuth Device Flow 全流程
- [ ] `internal/middleware/` — 审计日志、安全头、HTTPS 重定向
- [ ] 编写加密兼容性测试：用 Node.js 加密数据 → Go 解密

### Phase 2：业务模块（2 天）

- [ ] `internal/service/cas.go` + `internal/handler/cas.go` — CAS 存取（含压缩 + 加密）
- [ ] `internal/service/metrics.go` + `internal/handler/compatibility.go` — Metrics 批量写入（pgx.CopyFrom）
- [ ] `internal/service/authorship.go` + `internal/handler/authorship.go` — 归属记录 CRUD
- [ ] `internal/service/dashboard.go` + `internal/handler/dashboard.go` — 聚合查询
- [ ] `internal/service/sysconfig.go` + `internal/handler/sysconfig.go` — 配置管理（含加密）
- [ ] `internal/handler/compatibility.go` — Worker 兼容端点（oauth、metrics、cas）
- [ ] `internal/handler/health.go` — 所有健康检查端点

### Phase 3：HTML 页面 + 完整路由（0.5 天）

- [ ] 提取 HTML 模板到 `internal/templates/`
- [ ] `internal/handler/device_page.go` — 设备授权页面（GET /oauth/device、POST /oauth/device/approve|deny）
- [ ] `internal/handler/me_page.go` — 用户仪表板页面（GET /me）
- [ ] 路由注册：确保 /worker/* 和 /workers/* 双路径兼容
- [ ] 全局中间件链组装

### Phase 4：测试 + 联调（1.5 天）

- [ ] 单元测试：加密、JWT、配置解析
- [ ] 集成测试：每个 API 端点的请求/响应验证
- [ ] **加密兼容性验证**：用 Node.js 加密 → Go 解密（确保迁移后旧数据可读）
- [ ] **API 兼容性验证**：对比 Node.js 和 Go 的响应格式
- [ ] Docker 构建测试（目标镜像 < 25MB）
- [ ] 用 `git-ai` 客户端进行端到端联调

### Phase 5：部署切换（0.5 天）

- [ ] 更新 `docker-compose.yml`
- [ ] 更新部署文档
- [ ] 更新 README.md
- [ ] 灰度切换策略（双跑验证 → 切换 → 退役 Node.js 服务）

---

## 6. 关键风险与缓解措施

### 6.1 数据加密兼容性（最高风险）

**风险**：Go 无法正确解密 Node.js 已加密的数据

**缓解**：
- Phase 1 中优先实现加密模块并编写交叉验证测试
- 测试用例：Node.js 生成加密数据 → Go 解密 → 验证明文一致
- 两套加密格式分别测试（通用 JSON 格式 + CAS `iv:authTag:data` 格式）
- CAS 的 Scrypt 参数必须完全对齐：`salt="GitAISalt"`, `keyLen=32`, `N=32768(default)`, `r=8`, `p=1`

### 6.2 JWT Token 兼容性

**风险**：Go 签发的 token 旧客户端无法解析，或 Node.js 签发的 token Go 无法验证

**缓解**：
- 使用相同的 HS256 算法和 JWT_SECRET
- Claims 结构体的 JSON 标签必须与当前 TypeScript payload 完全一致
- 测试：Node.js 签发 → Go 验证；Go 签发 → Node.js 验证

### 6.3 API 响应格式差异

**风险**：Go 的 JSON 序列化行为与 TypeScript 不同（如空值处理、字段顺序）

**缓解**：
- 编写 API 对比测试脚本，同时请求 Node.js 和 Go 服务，比较响应
- 注意 `omitempty` 标签的使用，确保空值字段的行为一致
- 特别注意 `null` vs 缺失字段 vs `""` 的区别

### 6.4 数据库 Schema

全新部署，所有列名统一为 snake_case，无 Prisma camelCase 兼容负担。sqlc 生成的 Go 结构体字段名自然对应。

---

## 7. Dockerfile（目标 < 25MB）

```dockerfile
# 构建阶段
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server

# 运行阶段
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /app/server .
EXPOSE 3000
ENTRYPOINT ["/app/server"]
```

---

## 8. 配置对照

| 环境变量 | 当前用途 | Go 对应 |
|---------|---------|---------|
| `PORT` | 监听端口 | viper 直读 |
| `NODE_ENV` | 环境标识 | 改为 `APP_ENV`（production/development） |
| `CORS_ORIGIN` | CORS 允许源 | gin CORS 中间件 |
| `JWT_SECRET` | JWT 签名密钥 | golang-jwt 使用 |
| `ENCRYPTION_MASTER_KEY` | 主加密密钥（64 位 hex） | 标准库 crypto |
| `CAS_ENCRYPTION_KEY` | CAS 加密密钥 | Scrypt 派生 |
| `DATABASE_URL` | 数据库连接串 | pgx 直接使用 |
| `DB_HOST/PORT/USER/PASSWORD/NAME` | 分拆数据库配置 | viper 读取后拼接 |
| `DB_SSL` | SSL 开关 | pgx TLS 配置 |
| `TRUST_PROXY` | 信任代理层级 | gin TrustedProxies |
| `HTTPS_REDIRECT` | HTTPS 强制跳转 | 自定义中间件 |
| `DEV_HTTPS` | 本地 HTTPS | Go `tls.Config` |
| `JSON_BODY_LIMIT` | 请求体大小限制 | gin `MaxMultipartMemory` |
| `DEFAULT_USER_*` | 默认用户信息 | viper 读取 |

---

## 9. 验收标准

1. **功能完整性**：所有 ~30 个 API 端点行为与当前 Node.js 实现一致
2. **数据兼容性**：能正确读取 Node.js 已写入的加密数据（CAS、Config）
3. **Token 兼容性**：Node.js 签发的 JWT 在 Go 服务上可正常使用
4. **性能指标**：
   - 冷启动时间 < 1 秒（Node.js 约 3-5 秒）
   - 内存占用 < 50MB（Node.js 约 150-300MB）
   - Docker 镜像 < 25MB（Node.js 约 300MB）
5. **metrics 写入吞吐**：250 条/批的 CopyFrom 写入延迟 < 50ms（当前逐条 INSERT 约 500ms）
6. **部署简化**：单二进制 + 环境变量 即可运行，无需 Node.js 运行时
7. **测试覆盖**：核心路径（加密、认证、CAS 存取）测试覆盖率 > 80%

---

## 10. 时间线总结

| 阶段 | 耗时 | 产出 |
|------|------|------|
| Phase 0: 项目骨架 | 0.5 天 | 可编译运行的空壳 |
| Phase 1: 核心基础设施 | 1.5 天 | 加密、认证、中间件 |
| Phase 2: 业务模块 | 2 天 | 全部 API 端点 |
| Phase 3: HTML + 路由 | 0.5 天 | 完整路由 + 页面 |
| Phase 4: 测试联调 | 1.5 天 | 验证通过 |
| Phase 5: 部署切换 | 0.5 天 | 文档 + 部署 |
| **总计** | **~6.5 天** | |
