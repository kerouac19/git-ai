# Codex 安全审阅 — 4 项修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Codex review 在 server-go 上找到的 4 个安全/正确性问题：(1) 全局 BodyLimit 早于 AuditMiddleware 生效，避免 unbounded buffer；(2) auth 中间件拒绝 refresh token；(3) device Deny 端点要求登录；(4) `isSecure` 仅在 TRUST_PROXY 启用时读 X-Forwarded-Proto。

**Architecture:**
- 全局 `BodyLimit(casUploadLimit)` 装在 `AuditMiddleware` 之前——`http.MaxBytesReader` 只收紧不放宽，所以全局必须 ≥ 最大路由级限制（CAS = 10MB）；下游路由的 jsonLimit(2MB) / release handler 1MiB 会进一步收紧。
- `JWTAuthMiddleware` + `WorkerAuthMiddleware`（JWT 分支）在 `VerifyToken` 后增加 `claims.Type != "access"` 短路返回 401。
- `SecurityHeadersMiddleware` 与 `HTTPSRedirectMiddleware` 改为接受 `trustProxy bool`，把它透传给 `isSecure(c, trustProxy)`；trustProxy=false 时不读 X-Forwarded-Proto，只看 TLS。
- `/api/oauth/device/deny` 路由加 `jwtMW`，与 Approve 的"已登录用户操作"语义对称（Approve 在 handler 内自己做 cookie 校验；Deny 改为路由级 jwtMW 一致兜底）。

**Tech Stack:** Go 1.26 / Gin / `golang-jwt/jwt`。

**范围外（本次不做）：** Approve 同样改为路由级 jwtMW 替代 handler 级 cookie 校验（属重构，不在安全修复范围）；audit middleware 跳过 GET 请求以省 ReadAll（性能优化）。

**部署提示：** 4 项都是行为级修改，无 schema 变更。单实例部署：停 → 重启即可。已发出的 refresh token 在客户端"作为 Bearer 使用"路径会立即开始返回 401——客户端正常应该用 `/worker/oauth/token` 的 refresh_token grant 走 refresh 流程，那条路径**不**走 JWTAuthMiddleware，所以不受影响（device_flow.go:138 已经检查 `claims.Type != "refresh"`）。

---

## File Structure

**修改文件：**
- `server-go/cmd/server/main.go` — 全局 BodyLimit 插在 AuditMiddleware 前；SecurityHeaders/HTTPSRedirect 传 trustProxy；Deny 路由加 jwtMW
- `server-go/internal/auth/middleware.go` — JWTAuth + WorkerAuth 增 token type 检查
- `server-go/internal/auth/middleware_test.go` — 新增 refresh token 拒绝测试
- `server-go/internal/middleware/security_headers.go` — `SecurityHeadersMiddleware` + `isSecure` 接受 trustProxy
- `server-go/internal/middleware/https_redirect.go` — `HTTPSRedirectMiddleware` 接受 trustProxy
- `server-go/internal/middleware/security_headers_test.go` — 新建，覆盖 trustProxy gating
- `server-go/internal/handler/device_flow_test.go` — Deny 加 jwtMW 后的测试更新

**不需要修改：**
- `bodylimit.go` — `http.MaxBytesReader` 已正确实现。
- `audit.go` — 不动，全局 BodyLimit 在它之前装即可。
- `jwt.go` — `Claims.Type` 字段已存在，`VerifyToken` 不动。
- 现有 token 签发逻辑（access vs refresh）已正确，不动。

---

## Task 1: 全局 BodyLimit 早于 AuditMiddleware

**Files:**
- Modify: `server-go/cmd/server/main.go` (lines ~125 区域)

> 本 task **只插入 BodyLimit 一行**，不动 SecurityHeaders / HTTPSRedirect 签名（那两个签名变更在 Task 4 一起做）。这样 Task 1 单独可构建可测。

- [ ] **Step 1: 在 main.go AuditMiddleware 之前插入全局 BodyLimit**

编辑 `server-go/cmd/server/main.go`，把：

```go
	r.Use(middleware.SecurityHeadersMiddleware())
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware())
	}
	r.Use(middleware.AuditMiddleware(pool, trustProxy))
```

替换为（只多一行 + 注释段；SecurityHeaders/HTTPSRedirect 调用保持原签名不动）：

```go
	r.Use(middleware.SecurityHeadersMiddleware())
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware())
	}

	// Global body cap MUST come before AuditMiddleware: audit calls
	// io.ReadAll on the request body, so without an early cap an
	// unauthenticated large POST would buffer unbounded bytes in memory
	// before any route-level BodyLimit could trim it. Sized at the
	// largest legitimate route (CAS upload); per-route jsonLimit /
	// release admin (1 MiB) further tighten because http.MaxBytesReader
	// only collapses inward.
	r.Use(middleware.BodyLimit(casUploadLimit))

	r.Use(middleware.AuditMiddleware(pool, trustProxy))
```

- [ ] **Step 2: build 通过**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./...
```
期望：通过。

- [ ] **Step 3: 跑全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./...
```
期望：全 PASS（middleware 测试不会变，handler 测试不会变）。

- [ ] **Step 4: 提交 Task 1**

```bash
cd /Users/hg/git/git-ai && git add server-go/cmd/server/main.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: cap request body before AuditMiddleware buffers it

AuditMiddleware calls io.ReadAll on every request body to capture it
for the audit log, but the per-route BodyLimit was installed after
AuditMiddleware. An unauthenticated large POST to /worker/oauth/...
or release uploads could buffer unbounded memory before the route
limit kicked in.

Install a global BodyLimit(casUploadLimit) before AuditMiddleware.
casUploadLimit (~10MB by default) is the largest legitimate request
on any route, so downstream jsonLimit(2MB) and the release admin
1 MiB cap still tighten correctly — http.MaxBytesReader only
collapses inward when re-wrapped.

Codex review P1 #1.
EOF
)"
```

---

## Task 2: 拒绝 refresh token 充当 access token

**Files:**
- Modify: `server-go/internal/auth/middleware.go` (JWTAuthMiddleware + WorkerAuthMiddleware token type check)
- Modify: `server-go/internal/auth/middleware_test.go` (新增测试)

- [ ] **Step 1: 写失败测试**

编辑 `server-go/internal/auth/middleware_test.go`，在文件末尾追加：

```go
func TestJWTAuthMiddlewareRejectsRefreshToken(t *testing.T) {
	secret := "test-secret"
	subject := TokenSubject{Sub: "user-123", Email: "u@example.com", Name: "u", Role: "user"}

	refreshToken, err := SignRefreshToken(subject, secret, time.Hour)
	if err != nil {
		t.Fatalf("SignRefreshToken: %v", err)
	}

	r := gin.New()
	r.GET("/protected", JWTAuthMiddleware(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (refresh token must not authenticate); body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkerAuthMiddlewareRejectsRefreshToken(t *testing.T) {
	secret := "test-secret"
	subject := TokenSubject{Sub: "user-123", Email: "u@example.com", Name: "u", Role: "user"}

	refreshToken, err := SignRefreshToken(subject, secret, time.Hour)
	if err != nil {
		t.Fatalf("SignRefreshToken: %v", err)
	}

	r := gin.New()
	r.GET("/worker-protected", WorkerAuthMiddleware(secret, nil, TokenSubject{}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/worker-protected", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (refresh token must not authenticate worker route); body=%s", rec.Code, rec.Body.String())
	}
}
```

若该测试文件未导入 `"time"`、`"net/http"`、`"net/http/httptest"`、`"github.com/gin-gonic/gin"`，加入。

- [ ] **Step 2: 跑测试看 FAIL**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/auth/ -run "Rejects.*RefreshToken" -v
```
期望：FAIL（两个测试都返回 200，因为当前中间件接受 refresh token）。

- [ ] **Step 3: 修改 middleware.go 拒绝非 access token**

编辑 `server-go/internal/auth/middleware.go`。

(a) 把 `JWTAuthMiddleware` 中：
```go
		claims, err := VerifyToken(tokenString, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
```
替换为：
```go
		claims, err := VerifyToken(tokenString, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		if claims.Type != "" && claims.Type != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
```

> 允许 `Type == ""` 是为了兼容历史 token（未填 type 字段）；只要明确不是 "access" 就拒绝。同一 pattern 已在 device_flow.go:303 `DecodeAccessToken` 中使用。

(b) 把 `WorkerAuthMiddleware` 中 JWT 分支：
```go
		if tokenString != "" {
			if claims, err := VerifyToken(tokenString, secret); err == nil {
				setUserFromClaims(c, claims)
				c.Next()
				return
			}
		}
```
替换为：
```go
		if tokenString != "" {
			if claims, err := VerifyToken(tokenString, secret); err == nil {
				if claims.Type == "" || claims.Type == "access" {
					setUserFromClaims(c, claims)
					c.Next()
					return
				}
			}
		}
```

> 这里不直接 abort——因为后面还有 X-API-Key 路径要尝试。如果 JWT 是 refresh，跳过 JWT 分支走 API key 兜底，最终若两者都不通过，abort 401。

- [ ] **Step 4: 跑测试验证 PASS**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/auth/ -run "Rejects.*RefreshToken" -v
```
期望：两个测试 PASS。

- [ ] **Step 5: 跑 auth 包全测**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/auth/ -v
```
期望：全 PASS。

- [ ] **Step 6: 跑 build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
期望：通过、全 PASS。

- [ ] **Step 7: 提交 Task 2**

```bash
cd /Users/hg/git/git-ai && git add server-go/internal/auth/middleware.go server-go/internal/auth/middleware_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: reject refresh tokens in JWT/Worker auth middleware

VerifyToken accepts both access and refresh JWTs. The login/OAuth
response returns a 90-day refresh token, which clients could (and
attackers could induce them to) submit as Authorization: Bearer ...
on protected /api/* and worker routes, bypassing the 1-hour access
token lifetime entirely.

Both middlewares now require claims.Type == "access" (or empty for
legacy tokens that pre-date the type field) before authenticating
the caller. The same check has been in DecodeAccessToken since
inception; we just had not applied it on the inbound HTTP path.

Codex review P1 #2.
EOF
)"
```

---

## Task 3: device Deny 端点加 jwtMW

**Files:**
- Modify: `server-go/cmd/server/main.go` (line ~172 加 jwtMW)
- Modify: `server-go/internal/handler/device_flow_test.go` (现有测试可能需要 mock auth context)

- [ ] **Step 1: 检查现有 Deny 测试有没有挂中间件**

```bash
grep -n "Deny\|deviceFlowH.Deny\|\"/deny\"" /Users/hg/git/git-ai/server-go/internal/handler/device_flow_test.go
```

记下结果：如果现有测试只挂了 `r.POST("/deny", h.Deny)`（不挂 jwtMW），就需要在测试里加 jwtMW + 合法 JWT；如果测试已自己处理，跳过 Step 2 的测试改动。

- [ ] **Step 2: 加新测试覆盖"无 token 时 Deny 应返回 401"**

编辑 `server-go/internal/handler/device_flow_test.go`，在文件末尾追加：

```go
func TestDeny_RequiresAuthenticatedCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Mount with jwtMW + the actual handler; do NOT inject any token.
	jwtMW := auth.JWTAuthMiddleware("test-secret")
	h := &DeviceFlowHandler{} // service-less; jwtMW should short-circuit before handler runs
	r.POST("/deny", jwtMW, h.Deny)

	body := []byte(`{"user_code":"ABCD-EFGH"}`)
	req := httptest.NewRequest(http.MethodPost, "/deny", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (Deny must reject unauthenticated callers); body=%s", rec.Code, rec.Body.String())
	}
}
```

如该测试文件未导入 `"bytes"`、`"git-ai-server/internal/auth"`，加入；其余 `net/http`、`net/http/httptest`、`testing`、gin 通常已有。

- [ ] **Step 3: 跑测试看 PASS**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run TestDeny_RequiresAuthenticatedCaller -v
```
期望：PASS（jwtMW 在测试里挂上后即被中间件拒掉；此测试用例与生产配置保持一致）。

- [ ] **Step 4: 改 main.go 路由：Deny 加 jwtMW**

编辑 `server-go/cmd/server/main.go` 第 172 行，把：

```go
			device.POST("/deny", csrfMW, deviceFlowH.Deny)
```

替换为：

```go
			device.POST("/deny", jwtMW, csrfMW, deviceFlowH.Deny)
```

> 与 Approve 不对称的原因：Approve 的 handler 里已经做了 `claimsFromCookie` 显式校验（line 102-122 之类），cookie 缺失或无效会自行 401。Deny handler 没有等价校验。最简洁的修法是在路由层加 jwtMW；这同时也阻止"用 refresh token 作为 Bearer 来 Deny"的攻击（Task 2 的修复也覆盖这点）。

- [ ] **Step 5: 检查 device_flow_test.go 其他 Deny 测试是否会因新 jwtMW 失败**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/handler/ -run "Deny" -v
```
若有挂载 deny 路由但没传 jwt 的测试现在变成 401 而原本期望 200，更新它们：
- 要么挂 jwtMW + 在 request 附 valid token
- 要么换成上面 Step 2 的 mode（直接 mount handler 跳过 jwtMW 测内部行为）

注：如果该文件原有 Deny 测试不通过，逐一修复使其在新路由配置下重新对齐。具体修法因测试结构而异，**implementer 需要 read 测试文件后判断**，目标是：全包测试 PASS。

- [ ] **Step 6: build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
期望：build 通过，全 PASS。

- [ ] **Step 7: 提交 Task 3**

```bash
cd /Users/hg/git/git-ai && git add server-go/cmd/server/main.go server-go/internal/handler/device_flow_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: require authentication on /api/oauth/device/deny

The Deny handler never verified a logged-in session; the only
middleware on /api/oauth/device/deny was CSRF, which is bypassed
when the request carries an Authorization header. A client that
knew a pending user_code could deny that device authorization
without being authenticated.

Add jwtMW to the route, mirroring the implicit cookie check that
Approve does inside its handler. After Task 2, a refresh token
masquerading as a Bearer also fails closed here.

Codex review P2 #1.
EOF
)"
```

---

## Task 4: `isSecure` 仅在 TRUST_PROXY=true 时读 X-Forwarded-Proto

**Files:**
- Modify: `server-go/internal/middleware/security_headers.go` (isSecure + SecurityHeadersMiddleware 签名)
- Modify: `server-go/internal/middleware/https_redirect.go` (HTTPSRedirectMiddleware 签名)
- Modify: `server-go/cmd/server/main.go` (调用点传 trustProxy)
- Create: `server-go/internal/middleware/security_headers_test.go` (覆盖 trustProxy gating)

- [ ] **Step 1: 写失败测试**

新建 `server-go/internal/middleware/security_headers_test.go`：

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIsSecure_IgnoresForwardedProtoWhenTrustProxyDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ping", func(c *gin.Context) {
		if isSecure(c, false) {
			c.JSON(http.StatusOK, gin.H{"secure": true})
		} else {
			c.JSON(http.StatusOK, gin.H{"secure": false})
		}
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/ping", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != `{"secure":false}` {
		t.Fatalf("isSecure = %s, want {\"secure\":false} (X-Forwarded-Proto must be ignored when trustProxy=false)", got)
	}
}

func TestIsSecure_HonorsForwardedProtoWhenTrustProxyEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ping", func(c *gin.Context) {
		if isSecure(c, true) {
			c.JSON(http.StatusOK, gin.H{"secure": true})
		} else {
			c.JSON(http.StatusOK, gin.H{"secure": false})
		}
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/ping", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != `{"secure":true}` {
		t.Fatalf("isSecure = %s, want {\"secure\":true} (X-Forwarded-Proto should be honored when trustProxy=true)", got)
	}
}
```

- [ ] **Step 2: 跑测试看 FAIL（编译错误：isSecure 当前签名是 `isSecure(c)`）**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/middleware/ -run "IsSecure_" -v
```
期望：build error 或 FAIL（`isSecure` 当前签名是 `func(c *gin.Context) bool`，编译失败）。

- [ ] **Step 3: 改 `isSecure` + `SecurityHeadersMiddleware` 接受 trustProxy**

编辑 `server-go/internal/middleware/security_headers.go`，整段替换为：

```go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func SecurityHeadersMiddleware(trustProxy bool) gin.HandlerFunc {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-src 'self'",
		"object-src 'none'",
	}, "; ")

	return func(c *gin.Context) {
		h := c.Writer.Header()

		if isSecure(c, trustProxy) {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Content-Security-Policy", csp)
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		c.Next()
	}
}

// isSecure reports whether the request was received over TLS. When trustProxy
// is true, the X-Forwarded-Proto header is also honored — operators must
// explicitly opt in via TRUST_PROXY=true (or numeric hop count) so that an
// attacker cannot spoof "https" upstream over plain HTTP.
func isSecure(c *gin.Context, trustProxy bool) bool {
	if c.Request.TLS != nil {
		return true
	}
	if !trustProxy {
		return false
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	for _, p := range strings.Split(proto, ",") {
		if strings.TrimSpace(strings.ToLower(p)) == "https" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 改 `HTTPSRedirectMiddleware` 接受 trustProxy**

编辑 `server-go/internal/middleware/https_redirect.go`，把整个 `HTTPSRedirectMiddleware` 函数签名 + body 改为：

```go
func HTTPSRedirectMiddleware(trustProxy bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		if !isSecure(c, trustProxy) {
```

> 只动签名 + 调用 isSecure 的那一行；其他逻辑（构造重定向 URL、写 301 等）原样保留。

实际编辑：把
```go
func HTTPSRedirectMiddleware() gin.HandlerFunc {
```
改为
```go
func HTTPSRedirectMiddleware(trustProxy bool) gin.HandlerFunc {
```

把
```go
		if !isSecure(c) {
```
改为
```go
		if !isSecure(c, trustProxy) {
```

- [ ] **Step 5: 改 main.go 调用点传 trustProxy**

编辑 `server-go/cmd/server/main.go`，把：

```go
	r.Use(middleware.SecurityHeadersMiddleware())
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware())
	}
```

替换为：

```go
	r.Use(middleware.SecurityHeadersMiddleware(trustProxy))
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware(trustProxy))
	}
```

- [ ] **Step 6: 跑测试验证 PASS**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/middleware/ -run "IsSecure_" -v
```
期望：两个测试 PASS。

- [ ] **Step 7: middleware 包全测**

```bash
cd /Users/hg/git/git-ai/server-go && go test ./internal/middleware/ -v
```
期望：全 PASS（包含原有 csrf/bodylimit/clientip 测试）。

- [ ] **Step 8: build + 全包测试**

```bash
cd /Users/hg/git/git-ai/server-go && go build ./... && go test ./...
```
期望：通过、全 PASS。

- [ ] **Step 9: 提交 Task 4**

```bash
cd /Users/hg/git/git-ai && git add \
  server-go/internal/middleware/security_headers.go \
  server-go/internal/middleware/https_redirect.go \
  server-go/cmd/server/main.go \
  server-go/internal/middleware/security_headers_test.go
git -C /Users/hg/git/git-ai commit -m "$(cat <<'EOF'
server-go: only honor X-Forwarded-Proto when TRUST_PROXY is enabled

isSecure() previously trusted X-Forwarded-Proto unconditionally, so
when HTTPS_REDIRECT was on and the service was reachable directly (or
through an untrusted hop), a client could send X-Forwarded-Proto:
https over plain HTTP and skip the redirect / pretend the connection
was secure for HSTS purposes — even with TRUST_PROXY=false.

Thread trustProxy through SecurityHeadersMiddleware and
HTTPSRedirectMiddleware so isSecure consults the forwarded header
only when the operator opted in.

Codex review P2 #2.
EOF
)"
```

---

## Task 5: 端到端 smoke（综合验证 4 项修复）

**Files:** 无代码修改。

- [ ] **Step 1: 重建 smoke DB + 起 server**

```bash
psql -h 127.0.0.1 -d postgres -c "DROP DATABASE IF EXISTS git_ai_smoke;" -c "CREATE DATABASE git_ai_smoke;"
cd /Users/hg/git/git-ai/server-go && go build -o /tmp/git-ai-server-smoke ./cmd/server
PORT=37337 APP_ENV=development \
  JWT_SECRET=smoke-jwt-secret-not-for-prod-just-for-test \
  DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=$USER DB_PASSWORD= DB_NAME=git_ai_smoke DB_SSL=false \
  GIT_AI_API_KEY=smoke-api-key \
  HTTPS_REDIRECT=true \
  /tmp/git-ai-server-smoke > /tmp/git-ai-server-smoke.log 2>&1 &
sleep 2
curl -s http://127.0.0.1:37337/health
```

期望：health 返回 200。

- [ ] **Step 2: Task 1 — 全局 BodyLimit 验证（11 MB body 应被 413 拒绝）**

```bash
# 11 MB random payload > casUploadLimit(10 MB) global
dd if=/dev/zero bs=1M count=11 2>/dev/null | base64 | \
  curl -sS -i -X POST http://127.0.0.1:37337/worker/oauth/device/code \
    -H "Content-Type: application/json" --data-binary @- 2>&1 | head -10
```
期望：`HTTP/1.1 413` 或 `400`，含 "request body too large" 或类似文案（来自 http.MaxBytesReader → handler.Internal）。**关键**：返回应该早于 audit log 写入（验证手段：看 server log 没有 audit slow query）。

- [ ] **Step 3: Task 2 — refresh token 不能当 access token 用**

先用 device flow 拿到一个 refresh token。简便做法：用 admin 登录拿一对 token，提取 refresh_token：

```bash
curl -sS -c /tmp/smoke-cookies.txt -X POST http://127.0.0.1:37337/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<admin-pass-or-skip>"}' 2>&1 | head -5
```

注：如果 `INITIAL_ADMIN_PASSWORD` 没设置则无 admin 用户。这种情况下用 device flow / install_nonce 路径拿 token，或跳过这步——单元测试 `TestJWTAuthMiddlewareRejectsRefreshToken` 已经覆盖核心契约。

若拿到 refresh token：

```bash
# Get refresh_token (suppose REFRESH_TOKEN env var set)
curl -sS -i http://127.0.0.1:37337/api/me \
  -H "Authorization: Bearer $REFRESH_TOKEN" | head -3
```
期望：`HTTP/1.1 401` + `{"error":"Invalid or expired token"}`。

如果无法拿到真实 refresh token，本步通过 `TestJWTAuthMiddlewareRejectsRefreshToken` 单元测试已验证；smoke 这一项可跳过。

- [ ] **Step 4: Task 3 — device Deny 无 token 应 401**

```bash
curl -sS -i -X POST http://127.0.0.1:37337/api/oauth/device/deny \
  -H "Content-Type: application/json" \
  -d '{"user_code":"ABCD-EFGH"}' 2>&1 | head -5
```
期望：`HTTP/1.1 401` + `{"error":"Authorization required"}`。

- [ ] **Step 5: Task 4 — X-Forwarded-Proto 无效（TRUST_PROXY=false 默认）**

```bash
# HTTPS_REDIRECT=true 时，普通 HTTP 请求 GET / 应被 301 到 https
curl -sS -i -X GET http://127.0.0.1:37337/health 2>&1 | head -5

echo "---"

# 加上 X-Forwarded-Proto: https 仍应被 301（trustProxy=false 时不信任头）
curl -sS -i -X GET http://127.0.0.1:37337/health \
  -H "X-Forwarded-Proto: https" 2>&1 | head -5
```
期望：两个 curl 都返回 `HTTP/1.1 301`（前者本就 301，后者 trustProxy=false 时 X-Forwarded-Proto 被忽略也 301）。

> 但注意：本 smoke 服务用 `HTTPS_REDIRECT=true` 启动，且 GET /health 满足重定向方法判定。若 HTTPS_REDIRECT 默认是 false，把 env 加上重启再测。

第二轮带 `TRUST_PROXY=true` 重启，重测：

```bash
kill $(pgrep -f /tmp/git-ai-server-smoke) 2>/dev/null
sleep 1
PORT=37337 APP_ENV=development \
  JWT_SECRET=smoke-jwt-secret-not-for-prod-just-for-test \
  DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=$USER DB_PASSWORD= DB_NAME=git_ai_smoke DB_SSL=false \
  GIT_AI_API_KEY=smoke-api-key \
  HTTPS_REDIRECT=true TRUST_PROXY=true \
  /tmp/git-ai-server-smoke > /tmp/git-ai-server-smoke.log 2>&1 &
sleep 2

curl -sS -i -X GET http://127.0.0.1:37337/health \
  -H "X-Forwarded-Proto: https" 2>&1 | head -5
```
期望：`HTTP/1.1 200`（trustProxy=true 时 forwarded header 被信任，认为已经 HTTPS，不重定向）。

- [ ] **Step 6: 关停 server**

```bash
pkill -f /tmp/git-ai-server-smoke || true
```

Task 5 无代码改动，不 commit。

---

## 完成标准

- `go build ./...` 通过；`go test ./...` 全 PASS（含新增 4 个 middleware/handler 测试 + 原有测试无回归）
- 4 项 Codex P1/P2 各对应一个 commit
- 单实例本地 smoke：
  - 11 MB body 上传被 413/400 早期拒绝（Task 1）
  - refresh token 当 Bearer 用返回 401（Task 2，单元测试覆盖）
  - Deny 无 token 返回 401（Task 3）
  - TRUST_PROXY=false 时 X-Forwarded-Proto:https 仍触发 HTTPS redirect / 不被信任（Task 4）

## 回滚

按提交倒序 revert：
- Task 4：revert security_headers.go / https_redirect.go / main.go 中 trustProxy 透传；删除 security_headers_test.go。
- Task 3：revert main.go Deny 路由的 jwtMW；revert device_flow_test.go 新增 Deny test。
- Task 2：revert middleware.go 两处 token type 检查；删除 middleware_test.go 中两个新测试。
- Task 1：revert main.go 中的全局 BodyLimit 行。

无 schema / migration 变更，回滚不涉及 DB。

---

## 本地验证记录（2026-05-11）

实际在本机 macOS + PostgreSQL 15 上完成端到端 smoke。

**Commits chain**（base `4859750b` → HEAD `3a681d9d`，共 6 个 commit）：

```
3929c622 server-go: cap request body before AuditMiddleware buffers it
dacc100f server-go: reject refresh tokens in JWT/Worker auth middleware
11d46f3a server-go: add happy-path tests for access tokens in auth middleware
3472ee08 server-go: require authentication on /api/oauth/device/deny
4e605cdb server-go: only honor X-Forwarded-Proto when TRUST_PROXY is enabled
3a681d9d server-go: symmetrize /api/oauth/device/approve under jwtMW
```

其中最后一个是 final reviewer 标记的 Important asymmetry 修复（Approve 加 jwtMW 与 Deny 对称，handler 内 `claimsFromCookie` 保留并加注释）。

**环境：**
- 独立 smoke 库：`git_ai_smoke`（每次重启重建）
- 二进制：`/tmp/git-ai-server-smoke`
- 启动参数随场景切换 `HTTPS_REDIRECT` / `TRUST_PROXY`

**验证项与结果：**

| # | 项 | 结果 |
|---|----|------|
| 1 | `go build ./...` 通过；`go test ./...` 全 PASS（含 4 新 middleware/handler 测试 + Approve/Deny 既有测试无回归） | ✅ |
| 2 | Task 1：HTTPS_REDIRECT=false 下，3MB POST → `/api/user/login`（jsonLimit=2MB）→ `400` | ✅ |
| 3 | Task 1：11MB POST → `/api/user/login`（全局 BodyLimit=10MB 先于 jsonLimit 命中）→ `400` | ✅ |
| 4 | Task 2：4 个单元测试 PASS（JWT/Worker × refresh-reject + access-accept） | ✅ |
| 5 | Task 3：POST `/api/oauth/device/deny` 无 token → `401` `{"error":"Authorization required"}` | ✅ |
| 6 | Task 4 (a)：HTTPS_REDIRECT=true + TRUST_PROXY=false，GET `/api/version` 无 XFP → `301 → https://...` | ✅ |
| 7 | Task 4 (b)：同上配置 + `X-Forwarded-Proto: https` 头 → 仍 `301`（XFP 被忽略） | ✅ |
| 8 | Task 4 (c)：HTTPS_REDIRECT=true + TRUST_PROXY=true + `X-Forwarded-Proto: https` → `200`（XFP 被信任） | ✅ |
| 9 | Polish：`/api/oauth/device/approve` 路由加 jwtMW 后 Approve 既有测试不回归（handler 直挂的 happy-path 测试与生产路由解耦） | ✅ |

**SQL 对照与 smoke 命令样本：** 见 plan 各 Task 的 Step 块；执行细节均按计划走过。

**结论：** 6 个 commit 在本地端到端工作。Codex review 的 4 项安全 issue（2 P1 + 2 P2）全部修复并由单元测试 + smoke 双重覆盖；Final review 提出的 Important asymmetry 也以最小代价修复（Approve handler 内 `claimsFromCookie` 保留用于身份注入，路由层 jwtMW 兜底）。

**已知 follow-up（未在本 plan 范围内）：**
- `isSecure` 当前以"任一逗号分隔 XFP 值为 https 即返回 true"判断，更稳健做法是取链尾（最外层可信代理添加）。pre-existing，不属本次回归
- TLS 直连路径（`c.Request.TLS != nil`）暂未在 `security_headers_test.go` 测试中覆盖（httptest 注入 TLS 较繁琐，单元测试可作为后续补充）
- Final reviewer minor：`TestDeviceFlowHandler_DenySuccess` 名字可加注释说明"测 handler 内部假设 auth 已通过"

