package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestWorkerAuthMiddlewareAllowsAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/test", WorkerAuthMiddleware("secret", []string{"test-key"}, TokenSubject{
		Sub:           "user-1",
		Email:         "user@example.com",
		Name:          "Test User",
		PersonalOrgID: "org-1",
		Role:          "user",
	}), func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			t.Fatal("expected user context")
		}
		userMap, ok := user.(gin.H)
		if !ok {
			t.Fatalf("expected gin.H user context, got %T", user)
		}
		if userMap["id"] != "user-1" {
			t.Fatalf("expected user id user-1, got %v", userMap["id"])
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestWorkerAuthMiddlewareRejectsInvalidAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/test", WorkerAuthMiddleware("secret", []string{"test-key"}, TokenSubject{
		Sub: "user-1",
	}), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

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

func TestJWTAuthMiddlewareAcceptsAccessToken(t *testing.T) {
	secret := "test-secret"
	subject := TokenSubject{Sub: "user-123", Email: "u@example.com", Name: "u", Role: "user"}

	accessToken, err := SignAccessToken(subject, secret, time.Hour)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	r := gin.New()
	r.GET("/protected", JWTAuthMiddleware(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (access token should pass JWTAuthMiddleware); body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkerAuthMiddlewareAcceptsAccessToken(t *testing.T) {
	secret := "test-secret"
	subject := TokenSubject{Sub: "user-123", Email: "u@example.com", Name: "u", Role: "user"}

	accessToken, err := SignAccessToken(subject, secret, time.Hour)
	if err != nil {
		t.Fatalf("SignAccessToken: %v", err)
	}

	r := gin.New()
	r.GET("/worker-protected", WorkerAuthMiddleware(secret, nil, TokenSubject{}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/worker-protected", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (access token should pass WorkerAuthMiddleware); body=%s", rec.Code, rec.Body.String())
	}
}
