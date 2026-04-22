package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ClientIP returns a best-effort client IP for the request.
//
// When trustProxy is true the function honors X-Forwarded-For (first entry)
// and then X-Real-Ip. Otherwise those headers are ignored and the raw peer
// from RemoteAddr is used — that way callers behind an untrusted network
// can't spoof the IP that ends up in audit logs.
func ClientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// XFF is a comma-separated list; the left-most entry is the
			// original client per the de-facto convention.
			first := strings.SplitN(xff, ",", 2)[0]
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip")); realIP != "" {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIPFromGin is a thin adapter for handlers that already hold a
// *gin.Context.
func ClientIPFromGin(c *gin.Context, trustProxy bool) string {
	if c == nil {
		return ""
	}
	return ClientIP(c.Request, trustProxy)
}

// ForwardedProtoOrDefault returns the X-Forwarded-Proto value when trustProxy
// is true and the header is set; otherwise returns fallback. Used to derive
// the scheme for URLs built from the request when we're behind a terminating
// proxy that the operator has explicitly trusted.
func ForwardedProtoOrDefault(r *http.Request, trustProxy bool, fallback string) string {
	if !trustProxy || r == nil {
		return fallback
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		return fallback
	}
	// XFP may also be a comma-separated list.
	first := strings.SplitN(proto, ",", 2)[0]
	if p := strings.TrimSpace(first); p != "" {
		return strings.ToLower(p)
	}
	return fallback
}
