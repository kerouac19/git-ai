package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func SecurityHeadersMiddleware(trustProxy bool) gin.HandlerFunc {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-src 'self'",
		"object-src 'none'",
	}, "; ")

	return func(c *gin.Context) {
		h := c.Writer.Header()

		if isSecure(c, trustProxy) {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Content-Security-Policy", csp)
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		c.Next()
	}
}

// isSecure reports whether the request was received over TLS. When trustProxy
// is true, the X-Forwarded-Proto header is also honored — operators must
// explicitly opt in via TRUST_PROXY=true (or numeric hop count) so that an
// attacker cannot spoof "https" upstream over plain HTTP.
func isSecure(c *gin.Context, trustProxy bool) bool {
	if c.Request.TLS != nil {
		return true
	}
	if !trustProxy {
		return false
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	for _, p := range strings.Split(proto, ",") {
		if strings.TrimSpace(strings.ToLower(p)) == "https" {
			return true
		}
	}
	return false
}
