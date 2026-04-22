# server-go P1 最小安全补丁 · 设计

> 日期: 2026-04-23 · 范围: `server-go/` · 版本: 1

## 目标

针对 `server-go/` 一轮代码审查中识别的 P1（安全/正确性）问题做最小闭环修复。不改造日志基础设施、不引入新依赖、不做性能优化。落盘后仓库的安全基线应满足：

- 所有涉及个人数据的 `/api/*` 接口有鉴权
- 不再有日志行记录 PII（邮箱、姓名、`user_code`）
- 服务端原始错误不直接回给客户端
- 请求体大小受限，攻击者无法用大 body 打 OOM
- `X-Forwarded-*` header 只在信任代理时生效
- device flow 轮询的 rate limit 不能通过并发绕过
- CAS 读错误不会被吞成 "not found"
- 登录 / device code 发起通过 nginx 限流

不在本轮范围：结构化日志（slog）、OTEL/Prom 指标、索引优化、Dashboard SQL 合并、slog/readyz 分离、分布式限流。这些留给后续 P2/P3 设计。

## 变更清单

### 1. 鉴权挂载（`cmd/server/main.go`）

| 路由 | 中间件 | 策略 |
|---|---|---|
| `POST /api/cas/upload` | `workerMW` | — |
| `GET /api/cas/read/:hash` | `workerMW` | — |
| `POST /api/authorship/record` | `workerMW` | 请求体 `user_id` 必须等于 `claims.sub`（admin 例外） |
| `POST /api/authorship/commit` | `workerMW` | 仅要求已认证。`DTO.Author` 是 git author 标识符（可能是邮箱/名字），与 JWT sub 不可靠对应，只做 authN 不做 authZ |
| `PUT /api/authorship/sync/:userId` | `workerMW` | 路由参数 `userId` 必须等于 `claims.sub`（admin 例外） |
| `GET /api/authorship/commits/:userId` | `jwtMW` | 同上 |
| `GET /api/authorship/commits/:userId/:commitHash` | `jwtMW` | 同上 |
| `GET /api/authorship/commit/:commitHash` | `jwtMW` | 先查归属，再校验 `record.user_id == claims.sub`（admin 例外），否则 404 |
| `GET /api/dashboard/stats` | `jwtMW` | 忽略 `?userId=`，强制用 `claims.sub` |
| `POST /api/bundles` | `jwtMW` | — |
| `GET /api/dashboard/public` | 不变 | 本身就是 public |

新增 helper：`handler.requireSelfOrAdmin(c *gin.Context, targetUserID string) (ok bool)` —— 从 `c.Get("user")` 取出 claims，对比 `sub`/`role`。不通过时返回 403。

注：`workerMW` 通过 `X-API-Key` 通过时，身份解析为 `DEFAULT_USER_ID` + `DEFAULT_USER_ROLE`。运维侧如果想把 API Key 当作 admin 通道，必须把 `DEFAULT_USER_ROLE` 设为 `admin`；否则 API Key 只能操作"默认用户自己"的数据。这个行为在部署文档里标注即可，不在代码里开特例。

路由读取策略（"404 vs 403"）：对详单型读接口（`/api/authorship/commit/:commitHash`）返 404，避免泄露"这个 commit 在系统里存在"的信号；其他场景沿用 403。

### 2. 日志脱敏

删除以下调用：

- `internal/auth/device_flow.go:117` `fmt.Printf("[DEBUG] ExchangeDeviceCode: parsed subject ...")`
- `internal/auth/device_flow.go:283-289` 预查 `currentStatus` 的 extra-SELECT 以及 err 里的 `userCode/currentStatus` 拼接（它本身是为了给下一行 `[DEBUG]` 提供上下文）
- `cmd/server/main.go:425` `log.Printf("[DEBUG] Updating device code subject: ...")`
- `cmd/server/main.go:431` `log.Printf("[DEBUG] Successfully updated device code subject ...")`

保留 `cmd/server/main.go:427` 的 `[ERROR] Failed to update device code subject: %v`（只含 err，无 PII）。

### 3. Authorship 查询精确匹配

`internal/service/authorship.go:77` 和 `:92`：

```sql
-- 之前
WHERE user_id LIKE '%' || $1 || '%'
WHERE git_commit_hash LIKE '%' || $1 || '%'

-- 之后
WHERE user_id = $1
WHERE git_commit_hash = $1
```

对应的 Go 函数签名不变。调用方预期行为已和"精确匹配"一致，目前 LIKE 语义只是残留。

### 4. `X-Forwarded-*` 按 `TRUST_PROXY` 放行

新文件 `internal/middleware/clientip.go`：

```go
package middleware

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
)

// ClientIP returns the best-effort client IP. When trustProxy is true it reads
// X-Forwarded-For / X-Real-Ip; otherwise falls back to the socket peer.
func ClientIP(r *http.Request, trustProxy bool) string {
    if trustProxy {
        if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
            return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
        }
        if realIP := r.Header.Get("X-Real-Ip"); realIP != "" {
            return realIP
        }
    }
    // Fallback: gin's ClientIP respects trusted proxies when configured; for
    // our purposes the raw RemoteAddr is the right default.
    host, _, _ := strings.Cut(r.RemoteAddr, ":")
    if host != "" {
        return host
    }
    return ""
}

// ClientIPFromGin is a thin adapter used by handlers that already hold a *gin.Context.
func ClientIPFromGin(c *gin.Context, trustProxy bool) string {
    return ClientIP(c.Request, trustProxy)
}
```

用点：

- `internal/middleware/audit.go:142-150` `clientIP(c)` 改为调用 `ClientIPFromGin(c, trustProxy)`；`AuditMiddleware` 签名接受 `trustProxy bool`。
- `internal/handler/compatibility.go:98-100` `c.GetHeader("X-Forwarded-Proto")` 同样按 trustProxy 阀门读。

`trustProxy` 由 `main.go` 根据 `Config.TrustProxy()` 计算并下传：`any != false` 视为 true。

### 5. 错误脱敏

新文件 `internal/handler/respond.go`：

```go
package handler

import (
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
)

// Internal logs err server-side and returns a generic 500 payload to the client.
// Use this anywhere you would otherwise write err.Error() into a 5xx response.
func Internal(c *gin.Context, err error) {
    log.Printf("[handler] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
    c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
        "error": "internal server error",
    })
}
```

扫描替换策略：所有 `c.JSON(http.StatusInternalServerError, gin.H{"error": ... err.Error() ...})` → `handler.Internal(c, err)`。**不动 4xx**：那些是用户输入错误，清晰反馈给客户端更有用。

### 6. Body 限流

`internal/config/config.go` 加方法：

```go
// ParsedJSONBodyLimit parses "2mb" / "10mb" / "500kb" / raw bytes.
// Format: decimal number followed by optional unit b/kb/mb/gb (case-insensitive).
// Returns 2*1024*1024 on unparsable or non-positive input.
func (c *Config) ParsedJSONBodyLimit() int64 {
    return parseBytes(c.JSONBodyLimit, 2*1024*1024)
}

// ParsedCASUploadLimit parses CAS_UPLOAD_LIMIT; default 10mb.
func (c *Config) ParsedCASUploadLimit() int64 {
    return parseBytes(c.CASUploadLimit, 10*1024*1024)
}
```

`parseBytes` 单测覆盖：`"2mb"` → 2_097_152、`"500kb"` → 512_000、`""` → default、`"garbage"` → default。

新增 `CAS_UPLOAD_LIMIT` 环境变量（config 结构体加 `CASUploadLimit string` 字段，Viper default `"10mb"`）。

新文件 `internal/middleware/bodylimit.go`：

```go
func BodyLimit(limit int64) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
        c.Next()
    }
}
```

在 bind JSON 失败时，handler 端会收到 `*http.MaxBytesError`（或包装 error）。在错误脱敏 helper 中识别并返回 413：

```go
var maxErr *http.MaxBytesError
if errors.As(err, &maxErr) {
    c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
    return
}
```

挂载：

- `/api/*` 全局：`BodyLimit(cfg.ParsedJSONBodyLimit())`（默认 2MB）
- `/worker/cas/upload` / `/api/cas/upload`：改用 `BodyLimit(cfg.ParsedCASUploadLimit())`（默认 10MB）
- `/worker/metrics/upload`：用 `/api/*` 的 2MB 即可（`maxBatchSize=250` 事件下 2MB 绰绰有余）
- release 上传已有独立 1MiB 硬限制，不改

### 7. CORS credentials 修正

`cmd/server/main.go:251-267`：

```go
allowOrigin := origin
if allowOrigin == "" {
    allowOrigin = "*"
}
c.Header("Access-Control-Allow-Origin", allowOrigin)
// ...
if allowOrigin != "*" {
    c.Header("Access-Control-Allow-Credentials", "true")
}
```

### 8. Device poll TOCTOU

`internal/auth/device_flow.go:125-131` 的"读 + 判 + 写"三步并成一条 SQL：

```go
// claimPollSlot atomically updates last_polled_at if the previous attempt is
// older than deviceCodePollIntervalSecs. Returns true if the slot was claimed.
func (s *DeviceFlowService) claimPollSlot(ctx context.Context, deviceCode string) (bool, error) {
    tag, err := s.Pool.Exec(ctx, `
        UPDATE public.oauth_device_codes
        SET last_polled_at = now()
        WHERE device_code = $1
          AND (last_polled_at IS NULL
               OR last_polled_at < now() - make_interval(secs => $2))
    `, deviceCode, deviceCodePollIntervalSecs)
    if err != nil {
        return false, fmt.Errorf("claiming poll slot: %w", err)
    }
    return tag.RowsAffected() == 1, nil
}
```

`ExchangeDeviceCode` 的 pending 分支改为：

```go
ok, err := s.claimPollSlot(ctx, deviceCode)
if err != nil {
    return nil, err
}
if !ok {
    return oauthError("slow_down", "Polling too frequently"), nil
}
return oauthError("authorization_pending", "..."), nil
```

删除原 `touchDeviceCodePoll` 调用。

### 9. CAS ReadContent 错误语义

`internal/service/cas.go:200-204`：

```go
err := s.Pool.QueryRow(ctx, `SELECT encrypted_content, content_type FROM cas_entries WHERE hash = $1`, hash).Scan(&encrypted, &ct)
if errors.Is(err, pgx.ErrNoRows) {
    return nil, nil
}
if err != nil {
    return nil, fmt.Errorf("querying cas entry: %w", err)
}
```

handler 侧（`internal/handler/cas.go:51-77` 和 `internal/handler/compatibility.go:302-329`）对应路径已有 `result == nil → 404`，保留即可；错误路径改走 `handler.Internal`。

### 10. smoke-test.sh 配套更新

`server-go/scripts/smoke-test.sh`：

- 登录测试（脚本现有）成功后，把 access_token 存到 `ADMIN_TOKEN`
- 给所有 `/api/cas/*`、`/api/authorship/*`、`/api/dashboard/stats`、`/api/bundles` 调用追加 `-H "Authorization: Bearer $ADMIN_TOKEN"`
- 若脚本现在用的 `TEST_USER_ID` 和 token `sub` 不一致，改为登录响应里的用户 ID（admin 账号也能查自己的 authorship）
- 新增两条用例：未带 token 请求 `/api/cas/upload` 应返回 401，非 admin token 请求 `/api/authorship/commits/:otherUserId` 应返回 403

### 11. nginx 限流

`server-go/deploy/nginx.conf` 顶部加：

```nginx
limit_req_zone $binary_remote_addr zone=gitai_auth:10m rate=10r/m;
```

`server { ... }` 内部替换目前的 `location /`（或并列新增）：

```nginx
location = /api/user/login {
    limit_req zone=gitai_auth burst=5 nodelay;
    limit_req_status 429;
    include /etc/nginx/conf.d/git-ai-proxy.include;  # 见下
}
location ~ ^/workers?/oauth/device/code$ {
    limit_req zone=gitai_auth burst=5 nodelay;
    limit_req_status 429;
    include /etc/nginx/conf.d/git-ai-proxy.include;
}
```

为保持初版简单，不抽 include 文件，直接在两个 `location` 块内复制粘贴 `proxy_pass` + `proxy_set_header ...` 的 6 行（与现有 `location /` 一致）。后续嫌重复再重构。`limit_req_status 429` 让被限流的请求返回 429 而非默认 503，语义更准确。

## 测试

- `cd server-go && go build ./...` 必须绿
- `go vet ./...` 必须绿
- 新增 `internal/middleware/clientip_test.go`：table-driven 覆盖 `trustProxy=true/false × {有XFF, 有XRI, 都无}`
- 新增 `internal/middleware/bodylimit_test.go`：写超过 limit 应拿 413，且 err 是 `*http.MaxBytesError`
- 新增 `internal/service/authorship_userid_test.go`：验证精确匹配（模拟两条 user_id 有前缀包含关系，查询时不越界）
- `bash server-go/scripts/smoke-test.sh`：改脚本后应整体通过

## 提交策略

- commit 1：§2（日志）+ §3（LIKE→=）+ §8（device poll SQL）+ §9（CAS 读错误） —— 低风险、无对外协议变化
- commit 2：§4（clientip）+ §5（错误脱敏）+ §6（body 限流）+ §7（CORS）
- commit 3：§1（鉴权挂载）+ §10（smoke-test 同步）
- commit 4：§11（nginx）

拆分理由：commit 1、2 即使临时不合并后面也有独立价值；3 是对外协议变化，需要客户端/脚本同步；4 是配置文件，可由 ops 单独推。
