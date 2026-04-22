package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps the request body to limit bytes by wrapping c.Request.Body in
// http.MaxBytesReader. Over-limit reads return *http.MaxBytesError from Read;
// handler.Internal maps that error to HTTP 413 uniformly.
//
// Must run before any middleware or handler that reads the body.
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
