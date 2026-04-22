package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Internal logs err server-side and returns a generic 500 payload to the
// client. Use this anywhere you would otherwise write err.Error() into a
// 5xx response body.
//
// When err wraps an *http.MaxBytesError the client sees 413 instead of 500;
// bind/read callers get uniform handling for that specific over-limit case
// without having to detect it themselves.
func Internal(c *gin.Context, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "payload too large",
		})
		return
	}
	log.Printf("[handler] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"error": "internal server error",
	})
}
