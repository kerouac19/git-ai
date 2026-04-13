package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
