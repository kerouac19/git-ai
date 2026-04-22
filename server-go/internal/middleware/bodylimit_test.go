package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimitAllowsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(1024))
	r.POST("/", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Fatalf("ReadAll err = %v", err)
		}
		c.String(http.StatusOK, string(body))
	})

	payload := strings.Repeat("x", 100)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != payload {
		t.Fatalf("body length = %d, want %d", len(w.Body.String()), len(payload))
	}
}

func TestBodyLimitReturnsMaxBytesErrorOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var readErr error
	r := gin.New()
	r.Use(BodyLimit(64))
	r.POST("/", func(c *gin.Context) {
		_, readErr = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 1024)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var maxErr *http.MaxBytesError
	if !errors.As(readErr, &maxErr) {
		t.Fatalf("expected *http.MaxBytesError, got %T: %v", readErr, readErr)
	}
	if maxErr.Limit != 64 {
		t.Fatalf("maxErr.Limit = %d, want 64", maxErr.Limit)
	}
}

func TestBodyLimitZeroDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodyLimit(0))
	r.POST("/", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Fatalf("ReadAll err = %v", err)
		}
		c.String(http.StatusOK, string(body))
	})

	payload := strings.Repeat("x", 4096)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (zero limit must be a no-op)", w.Code)
	}
}
