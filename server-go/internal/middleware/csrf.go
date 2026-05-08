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
