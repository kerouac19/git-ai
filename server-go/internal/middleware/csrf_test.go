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

func TestCSRFBlocksUnsafeWithNoCookieHeader(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	// no Cookie header, no X-CSRF-Token header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (no cookie at all)", w.Code)
	}
}

func TestCSRFDoesNotSkipForNonBearerAuth(t *testing.T) {
	r := newCSRFRouter()
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(""))
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (Basic auth must not bypass CSRF)", w.Code)
	}
}
