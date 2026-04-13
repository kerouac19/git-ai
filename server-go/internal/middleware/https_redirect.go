package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

var healthPaths = map[string]struct{}{
	"/health":  {},
	"/healthz": {},
	"/readyz":  {},
	"/livez":   {},
}

func HTTPSRedirectMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, skip := healthPaths[c.Request.URL.Path]; skip {
			c.Next()
			return
		}

		if !isSecure(c) {
			host := c.Request.Host
			if host != "" {
				target := fmt.Sprintf("https://%s%s", host, c.Request.RequestURI)
				c.Redirect(http.StatusMovedPermanently, target)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
