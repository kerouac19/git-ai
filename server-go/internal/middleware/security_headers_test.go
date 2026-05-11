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
