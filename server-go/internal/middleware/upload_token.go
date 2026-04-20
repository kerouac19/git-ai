package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// UploadTokenAuth returns a middleware that enforces a single shared Bearer
// token for release upload endpoints. When token is empty, the middleware
// rejects the request with 503 to indicate the feature is disabled.
func UploadTokenAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				gin.H{"error": "release upload disabled: RELEASE_UPLOAD_TOKEN not set"})
			return
		}
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		got := strings.TrimSpace(header[len(prefix):])
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
