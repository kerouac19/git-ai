# server-go 前后端分离 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `server-go` 的 HTML 模板渲染替换成 React + Vite SPA，由 Nginx 直发静态产物，后端只暴露 JSON API。

**Architecture:** 同源部署：Nginx 在前，`/api/*`、`/health`、`/workers?/*`、`/releases` 反代到 Go (`:3000`)，其他路径走 `/var/www/git-ai/dist/` 静态目录。会话沿用 HttpOnly cookie + JWT；CSRF 用双提交 cookie：登录时另发 `csrf_token` 非 HttpOnly cookie，前端把值放进 `X-CSRF-Token` 头，后端中间件做常量时间比较。

**Tech Stack:** Go 1.26 + gin (后端)，React 18 + Vite + TypeScript + react-router-dom (前端)，Nginx (静态托管 + 反代)。

**Spec:** `docs/superpowers/specs/2026-05-08-server-go-frontend-backend-separation-design.md`

## File Map

后端新建：
- `server-go/internal/auth/csrf.go` — CSRF cookie 序列化、token 生成、提取
- `server-go/internal/auth/csrf_test.go`
- `server-go/internal/middleware/csrf.go` — 双提交校验中间件
- `server-go/internal/middleware/csrf_test.go`
- `server-go/internal/handler/device_flow.go` — 设备授权 JSON 接口
- `server-go/internal/handler/device_flow_test.go`

后端修改：
- `server-go/internal/handler/login.go` — 登录响应额外下发 csrf_token cookie；登出清 csrf_token
- `server-go/cmd/server/main.go` — 删 templates 与 HTML handler；新增 `/api/oauth/device/*`；挂 CSRF 中间件
- `server-go/internal/templates/` — **整个目录删除**

前端新建（`server-go/web/` 全部新文件）：
- `package.json`、`pnpm-lock.yaml`、`tsconfig.json`、`vite.config.ts`、`index.html`、`.gitignore`
- `src/main.tsx`、`src/App.tsx`
- `src/api/client.ts`、`src/api/auth.ts`、`src/api/dashboard.ts`、`src/api/device.ts`
- `src/types/api.ts`
- `src/routes/Login.tsx`、`src/routes/Me.tsx`、`src/routes/DeviceFlow.tsx`、`src/routes/DeviceResult.tsx`
- `src/components/ProtectedRoute.tsx`、`src/components/Stat.tsx`
- `src/styles/globals.css`

部署修改：
- `server-go/deploy/nginx.conf` — 静态托管 + SPA fallback + 精确路由分流
- `server-go/scripts/deploy.sh` — 新增 web 构建、静态目录同步

---

## Task 1: CSRF token utilities

**Files:**
- Create: `server-go/internal/auth/csrf.go`
- Test: `server-go/internal/auth/csrf_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// server-go/internal/auth/csrf_test.go
package auth

import (
	"strings"
	"testing"
)

func TestGenerateCSRFTokenIsRandom(t *testing.T) {
	a, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken err: %v", err)
	}
	b, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken err: %v", err)
	}
	if a == "" || b == "" {
		t.Fatalf("token should not be empty")
	}
	if a == b {
		t.Fatalf("two tokens should differ; got %q twice", a)
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}

func TestSerializeCSRFCookieAttributes(t *testing.T) {
	got := SerializeCSRFCookie("abc123", 3600, false)
	for _, must := range []string{"csrf_token=abc123", "Path=/", "SameSite=Lax", "Max-Age=3600"} {
		if !strings.Contains(got, must) {
			t.Fatalf("missing %q in cookie %q", must, got)
		}
	}
	if strings.Contains(got, "HttpOnly") {
		t.Fatalf("CSRF cookie must NOT be HttpOnly: %q", got)
	}
	if strings.Contains(got, "Secure") {
		t.Fatalf("non-production cookie must not have Secure: %q", got)
	}

	prod := SerializeCSRFCookie("abc123", 3600, true)
	if !strings.Contains(prod, "Secure") {
		t.Fatalf("production cookie must have Secure: %q", prod)
	}
}

func TestClearCSRFCookie(t *testing.T) {
	got := ClearCSRFCookie(false)
	if !strings.Contains(got, "csrf_token=") {
		t.Fatalf("clear cookie missing name: %q", got)
	}
	if !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("clear cookie must have Max-Age=0: %q", got)
	}
}

func TestExtractCSRFTokenFromCookie(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"csrf_token=abc", "abc"},
		{"git_ai_session=xx; csrf_token=abc; other=y", "abc"},
		{"", ""},
		{"git_ai_session=xx", ""},
	}
	for _, tc := range cases {
		if got := ExtractCSRFTokenFromCookie(tc.header); got != tc.want {
			t.Errorf("Extract(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server-go && go test ./internal/auth/ -run CSRF -v
```

Expected: build errors / undefined symbols (`GenerateCSRFToken`, `SerializeCSRFCookie`, `ClearCSRFCookie`, `ExtractCSRFTokenFromCookie`).

- [ ] **Step 3: Write the implementation**

```go
// server-go/internal/auth/csrf.go
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const CSRFCookieName = "csrf_token"

func GenerateCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func SerializeCSRFCookie(token string, maxAgeSeconds int, isProduction bool) string {
	attrs := []string{
		fmt.Sprintf("%s=%s", CSRFCookieName, token),
		"Path=/",
		"SameSite=Lax",
		fmt.Sprintf("Max-Age=%d", maxAgeSeconds),
	}
	if isProduction {
		attrs = append(attrs, "Secure")
	}
	return strings.Join(attrs, "; ")
}

func ClearCSRFCookie(isProduction bool) string {
	attrs := []string{
		CSRFCookieName + "=",
		"Path=/",
		"SameSite=Lax",
		"Max-Age=0",
	}
	if isProduction {
		attrs = append(attrs, "Secure")
	}
	return strings.Join(attrs, "; ")
}

func ExtractCSRFTokenFromCookie(cookieHeader string) string {
	for _, segment := range strings.Split(cookieHeader, ";") {
		segment = strings.TrimSpace(segment)
		parts := strings.SplitN(segment, "=", 2)
		if len(parts) != 2 || parts[0] != CSRFCookieName {
			continue
		}
		return parts[1]
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd server-go && go test ./internal/auth/ -run CSRF -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/internal/auth/csrf.go server-go/internal/auth/csrf_test.go
git commit -m "server-go: add CSRF cookie helpers"
```

---

## Task 2: CSRF middleware

**Files:**
- Create: `server-go/internal/middleware/csrf.go`
- Test: `server-go/internal/middleware/csrf_test.go`

The middleware skips when the request carries a non-cookie credential (`Authorization: Bearer ...` or `X-API-Key`). For other unsafe-method requests it requires `X-CSRF-Token` header to equal the `csrf_token` cookie under constant-time comparison. Safe methods (GET/HEAD/OPTIONS) always pass through.

- [ ] **Step 1: Write the failing tests**

```go
// server-go/internal/middleware/csrf_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCSRFRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRFProtect())
	r.GET("/get", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/post", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestCSRFAllowsSafeMethods(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
}

func TestCSRFBlocksUnsafeWithoutToken(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Cookie", "csrf_token=abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (no header)", w.Code)
	}
}

func TestCSRFBlocksUnsafeWithMismatch(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Cookie", "csrf_token=abc")
	req.Header.Set("X-CSRF-Token", "xyz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (mismatch)", w.Code)
	}
}

func TestCSRFAllowsUnsafeWithMatchingToken(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Cookie", "csrf_token=abc")
	req.Header.Set("X-CSRF-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
}

func TestCSRFSkipsBearerAuth(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer xxx")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (bearer should bypass CSRF)", w.Code)
	}
}

func TestCSRFSkipsAPIKeyAuth(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("X-API-Key", "xxx")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (api key should bypass CSRF)", w.Code)
	}
}

func TestCSRFRejectsEmptyTokenEvenIfMatching(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Cookie", "csrf_token=")
	req.Header.Set("X-CSRF-Token", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (empty token must not pass)", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server-go && go test ./internal/middleware/ -run CSRF -v
```

Expected: undefined `CSRFProtect`.

- [ ] **Step 3: Write the implementation**

```go
// server-go/internal/middleware/csrf.go
package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"git-ai-server/internal/auth"

	"github.com/gin-gonic/gin"
)

// CSRFProtect enforces double-submit cookie CSRF for cookie-authenticated
// requests. It is a no-op when the caller presents a non-cookie credential
// (Authorization Bearer or X-API-Key) so worker / CLI clients are unaffected.
func CSRFProtect() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		if hasNonCookieCredential(c) {
			c.Next()
			return
		}

		cookieToken := auth.ExtractCSRFTokenFromCookie(c.GetHeader("Cookie"))
		headerToken := c.GetHeader("X-CSRF-Token")
		if cookieToken == "" || headerToken == "" ||
			subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf token missing or mismatched"})
			return
		}
		c.Next()
	}
}

func hasNonCookieCredential(c *gin.Context) bool {
	if v := strings.TrimSpace(c.GetHeader("Authorization")); v != "" {
		if strings.HasPrefix(strings.ToLower(v), "bearer ") {
			return true
		}
	}
	if strings.TrimSpace(c.GetHeader("X-API-Key")) != "" {
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd server-go && go test ./internal/middleware/ -run CSRF -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/internal/middleware/csrf.go server-go/internal/middleware/csrf_test.go
git commit -m "server-go: add double-submit CSRF middleware"
```

---

## Task 3: Wire CSRF cookie into login / logout

**Files:**
- Modify: `server-go/internal/handler/login.go:38-103` (Login) and `:105-112` (Logout)

The login response already returns `access_token` in the body. Add a freshly generated `csrf_token` to both the response body and a non-HttpOnly cookie. Logout must clear it.

Note: `c.Header("Set-Cookie", ...)` overwrites. To set two cookies use `c.Writer.Header().Add("Set-Cookie", ...)`.

- [ ] **Step 1: Replace the Login handler body (after `service.ValidatePassword`)**

Replace lines 70-102 in `internal/handler/login.go` with:

```go
	subject := userToSubject(user)

	accessToken, err := auth.SignAccessToken(subject, h.JWTSecret, loginAccessTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue token"})
		return
	}

	refreshToken, err := auth.SignRefreshToken(subject, h.JWTSecret, loginRefreshTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue token"})
		return
	}

	csrfToken, err := auth.GenerateCSRFToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue token"})
		return
	}

	c.Writer.Header().Add("Set-Cookie", auth.SerializeSessionCookie(accessToken, int(loginAccessTokenTTL.Seconds()), h.IsProduction))
	c.Writer.Header().Add("Set-Cookie", auth.SerializeCSRFCookie(csrfToken, int(loginAccessTokenTTL.Seconds()), h.IsProduction))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"role":         user.Role,
			"status":       user.Status,
		},
		"access_token":       accessToken,
		"token_type":         "Bearer",
		"expires_in":         int(loginAccessTokenTTL.Seconds()),
		"refresh_token":      refreshToken,
		"refresh_expires_in": int(loginRefreshTokenTTL.Seconds()),
		"csrf_token":         csrfToken,
	})
}
```

- [ ] **Step 2: Replace the Logout handler**

Replace `Logout` (current lines 105-112) with:

```go
func (h *LoginHandler) Logout(c *gin.Context) {
	c.Writer.Header().Add("Set-Cookie", auth.ClearSessionCookie(h.IsProduction))
	c.Writer.Header().Add("Set-Cookie", auth.ClearCSRFCookie(h.IsProduction))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out"})
}
```

(The `text/html` redirect branch is intentionally dropped — the SPA owns the post-logout navigation.)

- [ ] **Step 3: Verify build**

```bash
cd server-go && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Smoke-test login response shape**

```bash
cd server-go && go test ./internal/handler/ -run TestLogin -v 2>&1 | tail -5
```

(There is no existing TestLogin; this just confirms the package still tests.) Run all package tests as a regression guard:

```bash
cd server-go && go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/internal/handler/login.go
git commit -m "server-go: emit csrf_token cookie on login, clear on logout"
```

---

## Task 4: Device flow JSON handler

**Files:**
- Create: `server-go/internal/handler/device_flow.go`
- Test: `server-go/internal/handler/device_flow_test.go`

Three endpoints replace the old HTML-rendering forms. All payloads are JSON.

| Method | Path | Auth | Body / Query |
|---|---|---|---|
| GET | `/api/oauth/device/info` | optional cookie | `?user_code=` |
| POST | `/api/oauth/device/approve` | cookie required | `{"user_code": "..."}` |
| POST | `/api/oauth/device/deny` | cookie optional | `{"user_code": "..."}` |

For `info`, when the caller is logged in we surface the caller's identity as `subject` (overriding any pending value on the device entry). When not logged in we surface the device entry's existing subject (often a placeholder) and an `authenticated: false` flag so the SPA can route to login.

For `approve`, we require an active cookie session; we update the device code's subject to the cookie's claims before approving.

- [ ] **Step 1: Write the failing tests**

```go
// server-go/internal/handler/device_flow_test.go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests exercise routing / shape only. Service-level behavior is
// covered by auth/device_flow tests. We use a minimal fake by injecting
// a stub through the handler interface introduced in device_flow.go.

func TestDeviceFlowHandler_InfoMissingUserCode(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlow{}}
	r := newGinTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestDeviceFlowHandler_InfoNotFound(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlow{}}
	r := newGinTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info?user_code=NOPE", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestDeviceFlowHandler_InfoFound(t *testing.T) {
	svc := &fakeDeviceFlow{
		entry: &deviceCodeEntry{UserCode: "ABCD", Status: "pending", ExpiresAt: 1234567890},
	}
	h := &DeviceFlowHandler{Svc: svc}
	r := newGinTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info?user_code=ABCD", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body["user_code"] != "ABCD" || body["status"] != "pending" {
		t.Fatalf("body=%v", body)
	}
	if body["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %v", body["authenticated"])
	}
}

func TestDeviceFlowHandler_ApproveRequiresLogin(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlow{}}
	r := newGinTestRouter()
	r.POST("/approve", h.Approve)

	req := httptest.NewRequest(http.MethodPost, "/approve", strings.NewReader(`{"user_code":"ABCD"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
}

func TestDeviceFlowHandler_DenySuccess(t *testing.T) {
	svc := &fakeDeviceFlow{
		entry: &deviceCodeEntry{UserCode: "ABCD", Status: "denied"},
	}
	h := &DeviceFlowHandler{Svc: svc}
	r := newGinTestRouter()
	r.POST("/deny", h.Deny)

	req := httptest.NewRequest(http.MethodPost, "/deny", strings.NewReader(`{"user_code":"ABCD"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !svc.denyCalled {
		t.Fatalf("Deny not called")
	}
}
```

Add a small test helper `internal/handler/device_flow_test_helpers.go` (separate file is fine to keep helpers next to the test):

```go
// server-go/internal/handler/device_flow_test_helpers.go
//go:build test || true

package handler

import (
	"context"

	"git-ai-server/internal/auth"

	"github.com/gin-gonic/gin"
)

func newGinTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

type deviceCodeEntry = struct {
	UserCode  string
	Status    string
	ExpiresAt int64
	Subject   *auth.TokenSubject
}

type fakeDeviceFlow struct {
	entry      *deviceCodeEntry
	denyCalled bool
}

func (f *fakeDeviceFlow) GetDeviceCodeByUserCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	if f.entry == nil || f.entry.UserCode != code {
		return nil, nil
	}
	return &auth.DeviceCodeInfo{
		UserCode:  f.entry.UserCode,
		Status:    f.entry.Status,
		ExpiresAt: f.entry.ExpiresAt,
		Subject:   f.entry.Subject,
	}, nil
}

func (f *fakeDeviceFlow) ApproveDeviceCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	return nil, nil
}

func (f *fakeDeviceFlow) DenyDeviceCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	f.denyCalled = true
	if f.entry == nil {
		return nil, nil
	}
	return &auth.DeviceCodeInfo{UserCode: f.entry.UserCode, Status: "denied"}, nil
}

func (f *fakeDeviceFlow) UpdateDeviceCodeSubject(ctx context.Context, code string, s auth.TokenSubject) error {
	return nil
}

func (f *fakeDeviceFlow) DecodeAccessToken(token string) (*auth.Claims, error) {
	return nil, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server-go && go test ./internal/handler/ -run DeviceFlowHandler -v
```

Expected: undefined `DeviceFlowHandler`.

- [ ] **Step 3: Write the implementation**

```go
// server-go/internal/handler/device_flow.go
package handler

import (
	"context"
	"net/http"
	"time"

	"git-ai-server/internal/auth"

	"github.com/gin-gonic/gin"
)

// deviceFlowService is the subset of *auth.DeviceFlowService used by this
// handler. Declared as an interface for testability.
type deviceFlowService interface {
	GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	ApproveDeviceCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	DenyDeviceCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	UpdateDeviceCodeSubject(ctx context.Context, userCode string, subject auth.TokenSubject) error
	DecodeAccessToken(accessToken string) (*auth.Claims, error)
}

type DeviceFlowHandler struct {
	Svc deviceFlowService
}

type deviceFlowBody struct {
	UserCode string `json:"user_code"`
}

type deviceFlowInfoResponse struct {
	UserCode      string   `json:"user_code"`
	Status        string   `json:"status"`
	ExpiresAt     *string  `json:"expires_at,omitempty"`
	Authenticated bool     `json:"authenticated"`
	Subject       *subject `json:"subject,omitempty"`
}

type subject struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (h *DeviceFlowHandler) Info(c *gin.Context) {
	userCode := c.Query("user_code")
	if userCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	entry, err := h.Svc.GetDeviceCodeByUserCode(c.Request.Context(), userCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}

	resp := deviceFlowInfoResponse{
		UserCode: entry.UserCode,
		Status:   entry.Status,
	}
	if entry.ExpiresAt > 0 {
		s := time.UnixMilli(entry.ExpiresAt).Format(time.RFC3339)
		resp.ExpiresAt = &s
	}

	if claims := h.claimsFromCookie(c); claims != nil && claims.Subject != "" {
		resp.Authenticated = true
		resp.Subject = &subject{Name: claims.Name, Email: claims.Email}
	} else if entry.Subject != nil {
		resp.Subject = &subject{Name: entry.Subject.Name, Email: entry.Subject.Email}
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeviceFlowHandler) Approve(c *gin.Context) {
	var body deviceFlowBody
	if err := c.ShouldBindJSON(&body); err != nil || body.UserCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	claims := h.claimsFromCookie(c)
	if claims == nil || claims.Subject == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}

	realSubject := auth.TokenSubject{
		Sub:           claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		PersonalOrgID: claims.PersonalOrgID,
		Orgs:          claims.Orgs,
		Role:          claims.Role,
	}
	if err := h.Svc.UpdateDeviceCodeSubject(c.Request.Context(), body.UserCode, realSubject); err != nil {
		Internal(c, err)
		return
	}

	entry, err := h.Svc.ApproveDeviceCode(c.Request.Context(), body.UserCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}
	if entry.Status == "denied" {
		c.JSON(http.StatusConflict, gin.H{"error": "device code already denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": entry.Status})
}

func (h *DeviceFlowHandler) Deny(c *gin.Context) {
	var body deviceFlowBody
	if err := c.ShouldBindJSON(&body); err != nil || body.UserCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	entry, err := h.Svc.DenyDeviceCode(c.Request.Context(), body.UserCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": entry.Status})
}

func (h *DeviceFlowHandler) claimsFromCookie(c *gin.Context) *auth.Claims {
	token := auth.ExtractAccessTokenFromCookie(c.GetHeader("Cookie"))
	if token == "" {
		return nil
	}
	claims, err := h.Svc.DecodeAccessToken(token)
	if err != nil {
		return nil
	}
	return claims
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd server-go && go test ./internal/handler/ -run DeviceFlowHandler -v
```

Expected: all 5 handler tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server-go/internal/handler/device_flow.go server-go/internal/handler/device_flow_test.go server-go/internal/handler/device_flow_test_helpers.go
git commit -m "server-go: add JSON device-flow handler (info/approve/deny)"
```

---

## Task 5: Wire new routes; remove HTML routes, handlers, and templates

**Files:**
- Modify: `server-go/cmd/server/main.go` (extensive)
- Delete: `server-go/internal/templates/` (entire directory)

This is the integration step. After this task the binary no longer renders HTML; the SPA owns all browser-facing UI.

- [ ] **Step 1: Remove template machinery from `main.go`**

In `cmd/server/main.go`:
- Delete imports: `embed`, `html/template`, `net/url`, `strings`, `math` (verify nothing else still needs them after the rest of the deletions in this step).
- Delete `//go:embed templates/*.html`, `var templateFS`, `var templates`, the entire `init()` (lines 31-49).
- Delete the structs `deviceFlowPageData`, `deviceResultPageData`, `dashboardPageData` (lines 323-365).
- Delete functions: `handleDeviceFlowPage`, `handleDeviceApprove`, `handleDeviceDeny`, `handleMePage`, `buildDashboardPageData`, `renderResult`, `renderLoginRequired`, `toFloat`, `toInt`, `toString`, `handleLoginPage` (lines 367-662 — verify line span matches when you open the file).
- Delete the trailing `var _ = os.Getenv` (line 728) and the `os` import if no longer used.

- [ ] **Step 2: Replace the HTML routes block with the JSON device-flow routes**

Find this block (current lines 152-161):

```go
	// Login page
	r.GET("/login", handleLoginPage())

	// OAuth Device Flow HTML pages
	r.GET("/oauth/device", handleDeviceFlowPage(deviceFlowSvc))
	r.POST("/oauth/device/approve", handleDeviceApprove(deviceFlowSvc, cfg.JWTSecret, isProduction))
	r.POST("/oauth/device/deny", handleDeviceDeny(deviceFlowSvc, isProduction))

	// /me dashboard page (cookie-based session)
	r.GET("/me", handleMePage(deviceFlowSvc, dashboardSvc))
```

Delete it. The SPA owns these paths now.

- [ ] **Step 3: Construct `DeviceFlowHandler` and CSRF middleware**

In the `// Handlers` section (after `sysConfigH := ...`), add:

```go
	deviceFlowH := &handler.DeviceFlowHandler{Svc: deviceFlowSvc}
```

In the global middleware section (right after `r.Use(corsMiddleware(cfg.CORSOrigin))`), add the CSRF middleware **before** any group handlers:

```go
	csrfMW := middleware.CSRFProtect()
```

(Don't `r.Use(csrfMW)` globally — apply per-group below so worker routes are unaffected.)

- [ ] **Step 4: Add the new device-flow API group with CSRF**

Inside the `api := r.Group("/api")` block, after the existing `api.GET("/me", jwtMW, compatH.GetMe)` line, add:

```go
		// Device flow (cookie-session). info is read-only; approve/deny are
		// CSRF-protected writes that also require a logged-in cookie.
		device := api.Group("/oauth/device", jsonLimit)
		{
			device.GET("/info", deviceFlowH.Info)
			device.POST("/approve", csrfMW, deviceFlowH.Approve)
			device.POST("/deny", csrfMW, deviceFlowH.Deny)
		}
```

- [ ] **Step 5: Apply CSRF to existing cookie-write routes**

Modify these route registrations to include `csrfMW`:

```go
		api.POST("/user/login", jsonLimit, loginH.Login)            // unchanged: no cookie yet
		api.GET("/user/logout", loginH.Logout)                       // GET — no CSRF needed
		api.POST("/user/logout", csrfMW, loginH.Logout)
		api.POST("/user/register", jsonLimit, jwtMW, csrfMW, adminOnly(), loginH.Register)
		api.POST("/bundles", jsonLimit, jwtMW, csrfMW, bundleH.Create)
```

In the dashboard group:

```go
		dashboard.POST("/generate-report", jwtMW, csrfMW, dashboardH.GenerateReport)
```

In the config group, change the group declaration so writes get CSRF:

```go
		cfgGroup := api.Group("/config", jsonLimit, jwtMW)
		{
			cfgGroup.GET("", sysConfigH.GetAll)
			cfgGroup.GET("/:key", sysConfigH.GetByKey)
			cfgGroup.POST("", csrfMW, sysConfigH.Create)
			cfgGroup.PATCH("/:key", csrfMW, sysConfigH.Update)
			cfgGroup.DELETE("/:key", csrfMW, sysConfigH.Delete)
		}
```

`authorshipWrite` and `cas` groups stay as-is — they're under `workerMW`, and the CSRF middleware bypasses any request that carries `Authorization: Bearer` or `X-API-Key`. Cookie callers on those routes will hit CSRF only if you also add `csrfMW`; we don't, because the SPA does not call worker write routes today. (If that changes, add `csrfMW` on the specific cookie-callable subgroup at that time.)

- [ ] **Step 6: Delete the templates directory**

```bash
rm -rf server-go/internal/templates
```

- [ ] **Step 7: Verify build and tests**

```bash
cd server-go && go build ./... && go test ./...
```

Expected: PASS. If tests fail because `releases_test.go` or another file referenced the deleted symbols, that means a stray reference — fix it.

- [ ] **Step 8: Manual smoke test**

In one terminal:

```bash
cd server-go && go run ./cmd/server
```

In another:

```bash
# 401 because /me now requires a token (no more /me HTML page; only /api/me JSON)
curl -i http://localhost:3000/api/me
# 404 — old HTML routes are gone
curl -i http://localhost:3000/login
curl -i http://localhost:3000/oauth/device?user_code=XXX
# 200 with JSON shape
curl -i http://localhost:3000/api/oauth/device/info?user_code=XXX
```

Expected: `/login` and `/oauth/device` return 404; `/api/oauth/device/info` returns JSON (404 for unknown code).

- [ ] **Step 9: Commit**

```bash
git add server-go/cmd/server/main.go
git rm -r server-go/internal/templates
git commit -m "server-go: drop html/template routes; add /api/oauth/device/* + CSRF"
```

---

## Task 6: Frontend project scaffold

**Files:**
- Create: `server-go/web/package.json`, `server-go/web/tsconfig.json`, `server-go/web/vite.config.ts`, `server-go/web/index.html`, `server-go/web/.gitignore`, `server-go/web/src/main.tsx`, `server-go/web/src/App.tsx`, `server-go/web/src/styles/globals.css`

The scaffold compiles a "Hello world" SPA. Routing and pages come in later tasks.

- [ ] **Step 1: Create `package.json`**

```json
{
  "name": "git-ai-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "typecheck": "tsc -b --noEmit"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.0"
  },
  "devDependencies": {
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.3",
    "vite": "^5.4.0"
  }
}
```

- [ ] **Step 2: Create `tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": false,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Create `vite.config.ts`**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:3000",
      "/health": "http://localhost:3000",
      "/workers": "http://localhost:3000",
      "/worker": "http://localhost:3000",
      "/releases": "http://localhost:3000",
    },
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});
```

- [ ] **Step 4: Create `index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Git AI</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: Create `.gitignore`**

```gitignore
node_modules/
dist/
.vite/
*.log
```

- [ ] **Step 6: Create `src/main.tsx`**

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./styles/globals.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
```

- [ ] **Step 7: Create `src/App.tsx` (placeholder)**

```tsx
export default function App() {
  return <div>Git AI — bootstrapping…</div>;
}
```

- [ ] **Step 8: Create `src/styles/globals.css`**

```css
:root {
  color-scheme: light;
  --bg: #f8fafc;
  --panel: #ffffff;
  --border: #e2e8f0;
  --text: #0f172a;
  --muted: #64748b;
  --accent: #0f766e;
  --accent-soft: #f0fdfa;
  --danger: #be123c;
  --success: #15803d;
}

* { box-sizing: border-box; }

html, body, #root { height: 100%; }

body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  line-height: 1.5;
}

button {
  font: inherit;
  cursor: pointer;
}

a { color: var(--accent); }
```

- [ ] **Step 9: Install deps and build**

```bash
cd server-go/web && pnpm install && pnpm build
```

Expected: build succeeds, produces `dist/index.html` and hashed assets under `dist/assets/`.

- [ ] **Step 10: Commit**

```bash
git add server-go/web/package.json server-go/web/pnpm-lock.yaml server-go/web/tsconfig.json server-go/web/vite.config.ts server-go/web/index.html server-go/web/.gitignore server-go/web/src/
git commit -m "server-go/web: scaffold React + Vite SPA"
```

---

## Task 7: API client + types

**Files:**
- Create: `server-go/web/src/api/client.ts`, `server-go/web/src/types/api.ts`

- [ ] **Step 1: Create `src/types/api.ts`**

```ts
export interface MeResponse {
  id: string;
  email: string;
  name: string;
  role: string;
  personal_org_id: string;
  orgs: Array<{
    org_id: string;
    org_name: string;
    org_slug: string;
    role: string;
  }>;
}

export interface DashboardStats {
  aiCode?: { percentage: number; totalAddedLines: number; committedAiLines: number };
  aiOutput?: { generated: number; edited: number };
  leaders?: {
    topAgent?: { label: string; promptCount: number };
    topModel?: { label: string; promptCount: number };
  };
  activity?: { activePromptCount: number; checkpointFileCount: number };
  metricsSummary?: { eventCount7d: number; repoCount7d: number; lastSyncAt?: string };
  today?: { activityCount: number; promptCount: number; fileCount: number; lastUpdatedAt?: string };
}

export interface DeviceFlowInfo {
  user_code: string;
  status: "pending" | "approved" | "denied";
  expires_at?: string;
  authenticated: boolean;
  subject?: { name: string; email: string };
}
```

- [ ] **Step 2: Create `src/api/client.ts`**

```ts
export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown, message?: string) {
    super(message ?? `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

function readCookie(name: string): string {
  const match = document.cookie
    .split(";")
    .map(s => s.trim())
    .find(s => s.startsWith(name + "="));
  return match ? decodeURIComponent(match.slice(name.length + 1)) : "";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const method = (init?.method ?? "GET").toUpperCase();
  const unsafe = !["GET", "HEAD", "OPTIONS"].includes(method);

  if (unsafe) {
    const token = readCookie("csrf_token");
    if (token) headers.set("X-CSRF-Token", token);
    if (init?.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
  }

  const res = await fetch(path, { ...init, credentials: "include", headers });
  const text = await res.text();
  const parsed = text ? safeJSON(text) : null;

  if (!res.ok) {
    throw new ApiError(res.status, parsed, typeof parsed === "object" && parsed && "error" in parsed ? String((parsed as { error: unknown }).error) : undefined);
  }
  return parsed as T;
}

function safeJSON(s: string): unknown {
  try { return JSON.parse(s); } catch { return s; }
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body == null ? undefined : JSON.stringify(body) }),
};
```

- [ ] **Step 3: Verify typecheck**

```bash
cd server-go/web && pnpm typecheck
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/api/ server-go/web/src/types/
git commit -m "server-go/web: add typed API client with CSRF header"
```

---

## Task 8: ProtectedRoute + App router

**Files:**
- Create: `server-go/web/src/components/ProtectedRoute.tsx`, `server-go/web/src/hooks/useMe.ts`
- Modify: `server-go/web/src/App.tsx`

`useMe` does a single `GET /api/me` and caches the result. `ProtectedRoute` shows a loading state, navigates to `/login?redirect=...` on 401, and renders children when authenticated.

- [ ] **Step 1: Create `src/hooks/useMe.ts`**

```ts
import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { MeResponse } from "../types/api";

type State =
  | { status: "loading" }
  | { status: "authenticated"; me: MeResponse }
  | { status: "anonymous" }
  | { status: "error"; error: Error };

export function useMe(): State {
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    api.get<MeResponse>("/api/me")
      .then(me => { if (!cancelled) setState({ status: "authenticated", me }); })
      .catch(err => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          setState({ status: "anonymous" });
        } else {
          setState({ status: "error", error: err });
        }
      });
    return () => { cancelled = true; };
  }, []);

  return state;
}
```

- [ ] **Step 2: Create `src/components/ProtectedRoute.tsx`**

```tsx
import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useMe } from "../hooks/useMe";
import type { MeResponse } from "../types/api";

interface Props {
  children: (me: MeResponse) => ReactNode;
}

export default function ProtectedRoute({ children }: Props) {
  const state = useMe();
  const location = useLocation();

  if (state.status === "loading") {
    return <div style={{ padding: 24 }}>Loading…</div>;
  }
  if (state.status === "anonymous") {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect=${redirect}`} replace />;
  }
  if (state.status === "error") {
    return <div style={{ padding: 24, color: "var(--danger)" }}>Error: {state.error.message}</div>;
  }
  return <>{children(state.me)}</>;
}
```

- [ ] **Step 3: Replace `src/App.tsx`**

```tsx
import { Navigate, Route, Routes } from "react-router-dom";
import Login from "./routes/Login";
import Me from "./routes/Me";
import DeviceFlow from "./routes/DeviceFlow";
import DeviceResult from "./routes/DeviceResult";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/me" element={<Me />} />
      <Route path="/oauth/device" element={<DeviceFlow />} />
      <Route path="/oauth/device/result" element={<DeviceResult />} />
      <Route path="*" element={<Navigate to="/me" replace />} />
    </Routes>
  );
}
```

(The four route components don't yet exist — the next tasks add them. Build will fail until Task 11.)

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/hooks/ server-go/web/src/components/ProtectedRoute.tsx server-go/web/src/App.tsx
git commit -m "server-go/web: add useMe + ProtectedRoute, wire router"
```

---

## Task 9: Login page

**Files:**
- Create: `server-go/web/src/routes/Login.tsx`, `server-go/web/src/api/auth.ts`

- [ ] **Step 1: Create `src/api/auth.ts`**

```ts
import { api } from "./client";
import type { MeResponse } from "../types/api";

interface LoginResponse {
  success: boolean;
  message: string;
  data: MeResponse;
  access_token: string;
  csrf_token: string;
}

export const authApi = {
  login: (username: string, password: string) =>
    api.post<LoginResponse>("/api/user/login", { username, password }),
  logout: () => api.post<{ success: boolean }>("/api/user/logout"),
};
```

Note: `/api/user/login` does **not** require an existing CSRF token (no cookie yet), but the client sends `X-CSRF-Token` if present from a previous session — that's harmless because the middleware only runs on routes where CSRF is mounted, and we deliberately did not mount it on `/api/user/login`.

- [ ] **Step 2: Create `src/routes/Login.tsx`**

```tsx
import { FormEvent, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { authApi } from "../api/auth";

export default function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const redirect = params.get("redirect") ?? "/me";

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await authApi.login(username, password);
      navigate(redirect, { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setError(typeof err.body === "object" && err.body && "message" in err.body
          ? String((err.body as { message: unknown }).message)
          : err.message);
      } else {
        setError(String(err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main style={{ maxWidth: 400, margin: "80px auto", padding: 32, background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 16 }}>
      <h1 style={{ marginTop: 0 }}>Sign in</h1>
      <form onSubmit={onSubmit}>
        <label style={{ display: "block", marginBottom: 12 }}>
          <div style={{ marginBottom: 4 }}>Username</div>
          <input
            value={username}
            onChange={e => setUsername(e.target.value)}
            autoFocus
            required
            style={{ width: "100%", padding: 8 }}
          />
        </label>
        <label style={{ display: "block", marginBottom: 16 }}>
          <div style={{ marginBottom: 4 }}>Password</div>
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            required
            style={{ width: "100%", padding: 8 }}
          />
        </label>
        {error && <div style={{ color: "var(--danger)", marginBottom: 12 }}>{error}</div>}
        <button type="submit" disabled={submitting} style={{ width: "100%", padding: 10, background: "var(--accent)", color: "white", border: "none", borderRadius: 8 }}>
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </main>
  );
}
```

- [ ] **Step 3: Build and verify**

```bash
cd server-go/web && pnpm build 2>&1 | tail -30
```

Expected: build still fails on `Me`, `DeviceFlow`, `DeviceResult` imports — that's fine, fixed in next tasks. Confirm `Login.tsx` itself has no type errors by running:

```bash
cd server-go/web && pnpm typecheck 2>&1 | grep -v "Cannot find module './routes/(Me|DeviceFlow|DeviceResult)'" || true
```

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/api/auth.ts server-go/web/src/routes/Login.tsx
git commit -m "server-go/web: add Login page"
```

---

## Task 10: /me dashboard page

**Files:**
- Create: `server-go/web/src/routes/Me.tsx`, `server-go/web/src/api/dashboard.ts`, `server-go/web/src/components/Stat.tsx`

- [ ] **Step 1: Create `src/api/dashboard.ts`**

```ts
import { api } from "./client";
import type { DashboardStats } from "../types/api";

export const dashboardApi = {
  stats: () => api.get<DashboardStats>("/api/dashboard/stats"),
};
```

- [ ] **Step 2: Create `src/components/Stat.tsx`**

```tsx
import type { ReactNode } from "react";

export default function Stat({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div style={{ background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 12, padding: 16 }}>
      <div style={{ color: "var(--muted)", fontSize: 12, textTransform: "uppercase", letterSpacing: 0.5 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 700, marginTop: 4 }}>{value}</div>
      {hint && <div style={{ color: "var(--muted)", fontSize: 13, marginTop: 4 }}>{hint}</div>}
    </div>
  );
}
```

- [ ] **Step 3: Create `src/routes/Me.tsx`**

```tsx
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import ProtectedRoute from "../components/ProtectedRoute";
import Stat from "../components/Stat";
import { dashboardApi } from "../api/dashboard";
import { authApi } from "../api/auth";
import type { DashboardStats, MeResponse } from "../types/api";

function MeContent({ me }: { me: MeResponse }) {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    dashboardApi.stats().then(setStats).catch(err => setError(String(err)));
  }, []);

  async function onLogout() {
    try { await authApi.logout(); } catch { /* ignore */ }
    navigate("/login", { replace: true });
  }

  const ai = stats?.aiCode;
  const today = stats?.today;
  const ms = stats?.metricsSummary;

  return (
    <main style={{ maxWidth: 1000, margin: "0 auto", padding: "40px 20px 80px" }}>
      <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 32 }}>
        <div>
          <h1 style={{ fontSize: 28, marginBottom: 4 }}>Hi, {me.name || me.email}</h1>
          <div style={{ color: "var(--muted)" }}>{me.email} · {me.role}</div>
        </div>
        <button onClick={onLogout} style={{ padding: "8px 14px", background: "transparent", border: "1px solid var(--border)", borderRadius: 8 }}>
          Sign out
        </button>
      </header>

      {error && <div style={{ color: "var(--danger)", marginBottom: 16 }}>Failed to load dashboard: {error}</div>}

      <section style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 16, marginBottom: 32 }}>
        <Stat label="AI code %" value={ai ? `${ai.percentage.toFixed(1)}%` : "—"} hint={ai ? `${ai.committedAiLines}/${ai.totalAddedLines} lines` : undefined} />
        <Stat label="Today activity" value={today?.activityCount ?? "—"} hint={today?.lastUpdatedAt ? `last: ${today.lastUpdatedAt}` : undefined} />
        <Stat label="Today prompts" value={today?.promptCount ?? "—"} />
        <Stat label="7d events" value={ms?.eventCount7d ?? "—"} hint={ms?.repoCount7d != null ? `${ms.repoCount7d} repos` : undefined} />
      </section>

      <section style={{ background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 12, padding: 20 }}>
        <h2 style={{ fontSize: 14, color: "var(--muted)", textTransform: "uppercase", letterSpacing: 0.5, marginTop: 0 }}>Top agent / model</h2>
        <div style={{ display: "flex", gap: 24, flexWrap: "wrap" }}>
          <div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>Agent</div>
            <div style={{ fontWeight: 600 }}>{stats?.leaders?.topAgent?.label ?? "—"}</div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>{stats?.leaders?.topAgent?.promptCount ?? 0} prompts</div>
          </div>
          <div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>Model</div>
            <div style={{ fontWeight: 600 }}>{stats?.leaders?.topModel?.label ?? "—"}</div>
            <div style={{ color: "var(--muted)", fontSize: 13 }}>{stats?.leaders?.topModel?.promptCount ?? 0} prompts</div>
          </div>
        </div>
      </section>
    </main>
  );
}

export default function Me() {
  return <ProtectedRoute>{me => <MeContent me={me} />}</ProtectedRoute>;
}
```

- [ ] **Step 4: Commit**

```bash
git add server-go/web/src/api/dashboard.ts server-go/web/src/components/Stat.tsx server-go/web/src/routes/Me.tsx
git commit -m "server-go/web: add /me dashboard page"
```

---

## Task 11: Device flow + result pages

**Files:**
- Create: `server-go/web/src/api/device.ts`, `server-go/web/src/routes/DeviceFlow.tsx`, `server-go/web/src/routes/DeviceResult.tsx`

- [ ] **Step 1: Create `src/api/device.ts`**

```ts
import { api } from "./client";
import type { DeviceFlowInfo } from "../types/api";

export const deviceApi = {
  info: (userCode: string) =>
    api.get<DeviceFlowInfo>(`/api/oauth/device/info?user_code=${encodeURIComponent(userCode)}`),
  approve: (userCode: string) =>
    api.post<{ status: string }>("/api/oauth/device/approve", { user_code: userCode }),
  deny: (userCode: string) =>
    api.post<{ status: string }>("/api/oauth/device/deny", { user_code: userCode }),
};
```

- [ ] **Step 2: Create `src/routes/DeviceFlow.tsx`**

```tsx
import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { deviceApi } from "../api/device";
import type { DeviceFlowInfo } from "../types/api";

export default function DeviceFlow() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const userCode = params.get("user_code") ?? "";

  const [info, setInfo] = useState<DeviceFlowInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!userCode) {
      setError("Missing user_code in URL.");
      return;
    }
    deviceApi.info(userCode)
      .then(setInfo)
      .catch(err => {
        if (err instanceof ApiError && err.status === 404) {
          navigate("/oauth/device/result?status=error&reason=not_found", { replace: true });
        } else {
          setError(String(err));
        }
      });
  }, [userCode, navigate]);

  if (error) return <main style={{ padding: 24, color: "var(--danger)" }}>{error}</main>;
  if (!info) return <main style={{ padding: 24 }}>Loading…</main>;

  if (!info.authenticated) {
    const redirect = encodeURIComponent(`/oauth/device?user_code=${userCode}`);
    navigate(`/login?redirect=${redirect}`, { replace: true });
    return null;
  }

  async function handle(action: "approve" | "deny") {
    setBusy(true);
    try {
      if (action === "approve") await deviceApi.approve(userCode);
      else await deviceApi.deny(userCode);
      navigate(`/oauth/device/result?status=${action === "approve" ? "ok" : "denied"}`, { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? `${err.status}: ${err.message}` : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <main style={{ maxWidth: 480, margin: "80px auto", padding: 32, background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 16 }}>
      <h1 style={{ marginTop: 0 }}>Authorize CLI</h1>
      <p style={{ color: "var(--muted)" }}>
        A command-line tool is requesting access as <strong>{info.subject?.name ?? "(unknown)"}</strong>
        {info.subject?.email ? ` (${info.subject.email})` : ""}.
      </p>
      <p style={{ color: "var(--muted)", fontSize: 13 }}>
        Code: <code>{info.user_code}</code> · expires {info.expires_at ?? "n/a"}
      </p>
      <div style={{ display: "flex", gap: 12, marginTop: 24 }}>
        <button onClick={() => handle("approve")} disabled={busy} style={{ flex: 1, padding: 10, background: "var(--accent)", color: "white", border: "none", borderRadius: 8 }}>
          Approve
        </button>
        <button onClick={() => handle("deny")} disabled={busy} style={{ flex: 1, padding: 10, background: "transparent", color: "var(--danger)", border: "1px solid var(--danger)", borderRadius: 8 }}>
          Deny
        </button>
      </div>
    </main>
  );
}
```

- [ ] **Step 3: Create `src/routes/DeviceResult.tsx`**

```tsx
import { Link, useSearchParams } from "react-router-dom";

const COPY: Record<string, { title: string; message: string; tone: "ok" | "error" }> = {
  ok: { title: "Device approved", message: "CLI authorization completed. You can return to your terminal.", tone: "ok" },
  denied: { title: "Device denied", message: "CLI authorization was denied. You can close this tab.", tone: "error" },
  error: { title: "Authorization error", message: "The device code could not be processed.", tone: "error" },
};

export default function DeviceResult() {
  const [params] = useSearchParams();
  const status = params.get("status") ?? "error";
  const c = COPY[status] ?? COPY.error;

  return (
    <main style={{ maxWidth: 480, margin: "80px auto", padding: 32, textAlign: "center", background: "var(--panel)", border: "1px solid var(--border)", borderRadius: 16 }}>
      <h1 style={{ color: c.tone === "error" ? "var(--danger)" : "var(--success)", marginTop: 0 }}>{c.title}</h1>
      <p style={{ color: "var(--muted)" }}>{c.message}</p>
      <Link to="/me" style={{ display: "inline-block", marginTop: 16 }}>Open dashboard</Link>
    </main>
  );
}
```

- [ ] **Step 4: Build the SPA**

```bash
cd server-go/web && pnpm build
```

Expected: `dist/index.html` and `dist/assets/*` produced, no TS errors.

- [ ] **Step 5: Manual end-to-end smoke (with backend running)**

In one terminal:

```bash
cd server-go && go run ./cmd/server
```

In another:

```bash
cd server-go/web && pnpm dev
```

Open `http://localhost:5173/login`, sign in with the bootstrap admin user, confirm `/me` renders stats. Visit `http://localhost:5173/oauth/device?user_code=XXX` (use a real device code from `git-ai login` flow) and approve.

- [ ] **Step 6: Commit**

```bash
git add server-go/web/src/api/device.ts server-go/web/src/routes/DeviceFlow.tsx server-go/web/src/routes/DeviceResult.tsx
git commit -m "server-go/web: add device-flow + result pages"
```

---

## Task 12: Nginx config — static + reverse proxy

**Files:**
- Modify: `server-go/deploy/nginx.conf`

The existing 443 server block routes everything to the Go backend. Replace the catch-all `location /` with a static-files-first layout, keeping the rate-limited `/api/user/login` and worker device-code blocks.

- [ ] **Step 1: Replace the catch-all `location /` block**

Inside the `server { listen 443 ssl http2; ... }` block, **after** the existing `location = /api/user/login` and `location ~ ^/workers?/oauth/device/code$` rules, replace the current `location / { proxy_pass ... }` block (lines 79-97) with the following:

```nginx
    root /var/www/git-ai/dist;
    index index.html;

    # API → Go
    location /api/ {
        proxy_http_version 1.1;
        proxy_pass http://git_ai_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Port 443;
        proxy_connect_timeout 5s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # CLI / worker compatibility → Go
    location /workers/ { proxy_pass http://git_ai_backend; include /etc/nginx/snippets/git-ai-proxy.conf; }
    location /worker/  { proxy_pass http://git_ai_backend; include /etc/nginx/snippets/git-ai-proxy.conf; }
    location /releases { proxy_pass http://git_ai_backend; include /etc/nginx/snippets/git-ai-proxy.conf; }

    # Vite hash-named assets — long cache
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        try_files $uri =404;
    }

    # SPA fallback
    location / {
        try_files $uri /index.html;
    }
```

(`/etc/nginx/snippets/git-ai-proxy.conf` is a small file the deploy script will create in Task 13 with the `proxy_set_header` lines so we don't repeat them. If you'd rather inline them, expand each `include` directive instead.)

- [ ] **Step 2: Update the existing `location = /health` block** (it stays as-is, but verify it's outside the SPA fallback path)

The existing `location = /health` block at the bottom is fine as-is.

- [ ] **Step 3: Sanity-check syntax**

If you have nginx locally:

```bash
nginx -t -c /Users/hg/git/git-ai/server-go/deploy/nginx.conf
```

If not, defer to Task 14's deploy smoke test.

- [ ] **Step 4: Commit**

```bash
git add server-go/deploy/nginx.conf
git commit -m "server-go: nginx serves SPA dist + reverse-proxies /api"
```

---

## Task 13: deploy.sh — build web, install snippets, sync dist

**Files:**
- Modify: `server-go/scripts/deploy.sh`

Three changes:
1. `cmd_build` also builds the web project.
2. New `cmd_install_web` (callable on target machine) installs the proxy snippet and ensures `/var/www/git-ai/dist` exists.
3. `cmd_install` (or a new `cmd_publish_web`) syncs the dist directory.

- [ ] **Step 1: Extend `cmd_build`**

Inside `cmd_build`, after the `go build` step (after the `ok "构建完成: ..."` line), append:

```bash
    if [[ -d "web" ]]; then
        info "构建 web (前端)..."
        if ! command -v pnpm &>/dev/null; then
            die "未找到 pnpm，请先安装 pnpm 8+ (npm i -g pnpm)"
        fi
        (cd web && pnpm install --frozen-lockfile && pnpm build)
        ok "前端构建完成: $(pwd)/web/dist"
    else
        warn "未发现 web/ 目录，跳过前端构建"
    fi
```

- [ ] **Step 2: Add `cmd_install_web`**

Add a new function above the dispatch switch:

```bash
# ─────────────────── install-web ───────────────────
cmd_install_web() {
    [[ $EUID -eq 0 ]] || die "请使用 sudo 运行"

    local web_root="/var/www/git-ai/dist"
    local snippets_dir="/etc/nginx/snippets"
    local snippet="${snippets_dir}/git-ai-proxy.conf"

    info "安装前端静态目录与 nginx 片段..."
    mkdir -p "${web_root}"
    chown -R www-data:www-data "${web_root}" 2>/dev/null || true
    chmod 755 "${web_root}"

    mkdir -p "${snippets_dir}"
    cat >"${snippet}" <<'EOF'
proxy_http_version 1.1;
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto https;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Port 443;
EOF
    ok "已写入 ${snippet}"

    # 同步前端 dist（若与脚本同目录）
    if [[ -d "$(dirname "$0")/dist" ]]; then
        rsync -a --delete "$(dirname "$0")/dist/" "${web_root}/"
        ok "已同步前端到 ${web_root}"
    elif [[ -d "$(dirname "$0")/../web/dist" ]]; then
        rsync -a --delete "$(dirname "$0")/../web/dist/" "${web_root}/"
        ok "已同步前端到 ${web_root}"
    else
        warn "未找到 web/dist 或 ./dist，请手动 rsync 到 ${web_root}"
    fi

    info "请测试 nginx 配置: sudo nginx -t && sudo systemctl reload nginx"
}
```

- [ ] **Step 3: Add `install-web` to the dispatch**

Find the `case "${1:-}" in` block at the bottom and add a new case:

```bash
        install-web)
            cmd_install_web
            ;;
```

Update the usage hint at the top of the file to mention `install-web`.

- [ ] **Step 4: Lint with shellcheck (optional)**

```bash
shellcheck server-go/scripts/deploy.sh || true
```

- [ ] **Step 5: Commit**

```bash
git add server-go/scripts/deploy.sh
git commit -m "server-go: deploy.sh builds web, installs nginx snippet + dist"
```

---

## Task 14: End-to-end smoke

**Files:** none (verification only)

- [ ] **Step 1: Run the full backend test suite**

```bash
cd server-go && go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run the full frontend typecheck + build**

```bash
cd server-go/web && pnpm typecheck && pnpm build
```

Expected: PASS, dist produced.

- [ ] **Step 3: Local two-process smoke**

Terminal 1:
```bash
cd server-go && go run ./cmd/server
```

Terminal 2:
```bash
cd server-go/web && pnpm dev
```

Browser: `http://localhost:5173/login` → sign in → land on `/me` → see stats → click Sign out → back at `/login`.

CLI device flow check:
- Trigger a real device code (e.g. `git-ai login`).
- Open `http://localhost:5173/oauth/device?user_code=XXX`.
- Confirm approve and deny paths each land on `/oauth/device/result?status=...`.

- [ ] **Step 4: Commit any final tweaks discovered during smoke**

```bash
git add -A && git commit -m "server-go: smoke-fix follow-ups" || true
```

(Skip if no changes.)

---

## Self-Review

**Spec coverage:**
- Architecture diagram → Task 5 (routing) + Task 12 (nginx) ✓
- Backend deletions (`templates/`, HTML handlers) → Task 5 ✓
- New `/api/oauth/device/*` endpoints → Task 4 + Task 5 ✓
- CSRF middleware (double-submit, skip on Bearer / X-API-Key) → Task 1 + 2 ✓
- Login response includes `csrf_token` body + non-HttpOnly cookie → Task 3 ✓
- Frontend `web/` directory layout → Tasks 6–11 ✓
- Routes table (`/login`, `/me`, `/oauth/device`, `/oauth/device/result`, `*`) → Task 8 ✓
- `client.ts` cookie read + `X-CSRF-Token` injection + 401 handling → Task 7 ✓
- Vite dev proxy → Task 6 ✓
- Nginx config (static root, SPA fallback, `/api/*` proxy, hash-asset cache, retained rate-limit) → Task 12 ✓
- `deploy.sh` web build + dist sync → Task 13 ✓
- Three data flows (login / dashboard / device) verified end-to-end → Task 14 ✓
- Tests: CSRF helpers, CSRF middleware, device-flow handler → Tasks 1, 2, 4 ✓
- Open question deferrals (visual fidelity, component lib, token rotation, Playwright) → out of scope, plan provides functional v1 ✓

**Placeholder scan:** searched for "TBD"/"TODO"/"implement later" — none.

**Type consistency:**
- `MeResponse` shape matches backend `JWTAuthMiddleware`'s `c.Set("user", gin.H{...})` keys (`id`, `email`, `name`, `role`, `personal_org_id`, `orgs`).
- `DashboardStats` mirrors fields used by the original `buildDashboardPageData` helper.
- `DeviceFlowInfo` matches `deviceFlowInfoResponse` field tags in Task 4.
- Backend `csrf_token` JSON key in login response matches frontend's expected response field.
- CSRF cookie name `csrf_token` is identical in `auth/csrf.go` and the frontend `readCookie` call.
- `deviceFlowService` interface in handler aligns with the methods used by the existing `*auth.DeviceFlowService` (`GetDeviceCodeByUserCode`, `ApproveDeviceCode`, `DenyDeviceCode`, `UpdateDeviceCodeSubject`, `DecodeAccessToken`).

No issues found.
