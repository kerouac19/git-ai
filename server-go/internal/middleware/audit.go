package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sensitiveBodyKeys = map[string]struct{}{
	"password":      {},
	"token":         {},
	"secret":        {},
	"key":           {},
	"authorization": {},
	"auth":          {},
}

var sensitiveHeaderKeys = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"x-api-key":     {},
	"x-auth-token":  {},
}

func AuditMiddleware(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Read and buffer the body so downstream handlers still have access.
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		c.Next()

		// Capture values after the handler chain has executed.
		status := c.Writer.Status()
		duration := time.Since(start)

		userID := "anonymous"
		if u, exists := c.Get("user"); exists {
			if m, ok := u.(gin.H); ok {
				if id, ok := m["id"].(string); ok && id != "" {
					userID = id
				}
			}
		}

		action := fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path)
		resource := c.Request.URL.Path
		ip := clientIP(c)
		userAgent := c.Request.Header.Get("User-Agent")
		success := status < 400
		details := fmt.Sprintf("Duration: %dms, Status: %d", duration.Milliseconds(), status)

		params := buildParams(c, bodyBytes)
		paramsJSON, _ := json.Marshal(params)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var ua *string
			if userAgent != "" {
				ua = &userAgent
			}

			_, err := pool.Exec(ctx, `
				INSERT INTO public.audit_logs (
					user_id, action, resource, params_json,
					ip, user_agent, occurred_at, success, details
				) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9)`,
				userID, action, resource, string(paramsJSON),
				ip, ua, time.Now(), success, details,
			)
			if err != nil {
				log.Printf("[audit] failed to record audit log: %v", err)
			}
		}()
	}
}

func buildParams(c *gin.Context, body []byte) map[string]any {
	params := map[string]any{
		"query":   c.Request.URL.Query(),
		"headers": safeHeaders(c),
	}

	if len(body) > 0 {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			params["body"] = sanitizeBody(parsed)
		}
	}

	return params
}

func sanitizeBody(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	sanitized := make(map[string]any, len(m))
	for k, val := range m {
		if _, sensitive := sensitiveBodyKeys[strings.ToLower(k)]; sensitive {
			sanitized[k] = "[REDACTED]"
			continue
		}
		if nested, ok := val.(map[string]any); ok {
			sanitized[k] = sanitizeBody(nested)
		} else {
			sanitized[k] = val
		}
	}
	return sanitized
}

func safeHeaders(c *gin.Context) map[string]string {
	safe := make(map[string]string)
	for name, values := range c.Request.Header {
		if _, sensitive := sensitiveHeaderKeys[strings.ToLower(name)]; sensitive {
			safe[name] = "[REDACTED]"
		} else {
			safe[name] = strings.Join(values, ", ")
		}
	}
	return safe
}

func clientIP(c *gin.Context) string {
	if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
		return strings.SplitN(forwarded, ",", 2)[0]
	}
	if realIP := c.GetHeader("X-Real-Ip"); realIP != "" {
		return realIP
	}
	return c.ClientIP()
}
