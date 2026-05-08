package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

const CSRFCookieName = "csrf_token"

func GenerateCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// SerializeCSRFCookie returns a Set-Cookie header value for the CSRF token.
// The token must be safe for direct cookie-value use (no ";", CR, or LF) —
// callers should produce it via GenerateCSRFToken.
func SerializeCSRFCookie(token string, maxAgeSeconds int, isProduction bool) string {
	attrs := []string{
		fmt.Sprintf("%s=%s", CSRFCookieName, token),
		"Path=/",
		"SameSite=Lax",
		fmt.Sprintf("Max-Age=%d", maxAgeSeconds),
	}
	if isProduction {
		attrs = append(attrs, "Secure")
	}
	return strings.Join(attrs, "; ")
}

func ClearCSRFCookie(isProduction bool) string {
	attrs := []string{
		CSRFCookieName + "=",
		"Path=/",
		"SameSite=Lax",
		"Max-Age=0",
	}
	if isProduction {
		attrs = append(attrs, "Secure")
	}
	return strings.Join(attrs, "; ")
}

func ExtractCSRFTokenFromCookie(cookieHeader string) string {
	for _, segment := range strings.Split(cookieHeader, ";") {
		segment = strings.TrimSpace(segment)
		parts := strings.SplitN(segment, "=", 2)
		if len(parts) != 2 || parts[0] != CSRFCookieName {
			continue
		}
		return parts[1]
	}
	return ""
}
