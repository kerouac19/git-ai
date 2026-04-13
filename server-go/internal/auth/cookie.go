package auth

import (
	"fmt"
	"net/url"
	"strings"
)

const SessionCookieName = "git_ai_session"

func SerializeSessionCookie(accessToken string, maxAgeSeconds int, isProduction bool) string {
	attributes := []string{
		fmt.Sprintf("%s=%s", SessionCookieName, url.QueryEscape(accessToken)),
		"Path=/",
		"HttpOnly",
		"SameSite=Lax",
		fmt.Sprintf("Max-Age=%d", maxAgeSeconds),
	}

	if isProduction {
		attributes = append(attributes, "Secure")
	}

	return strings.Join(attributes, "; ")
}

func ClearSessionCookie(isProduction bool) string {
	attributes := []string{
		SessionCookieName + "=",
		"Path=/",
		"HttpOnly",
		"SameSite=Lax",
		"Max-Age=0",
	}

	if isProduction {
		attributes = append(attributes, "Secure")
	}

	return strings.Join(attributes, "; ")
}

func ExtractAccessTokenFromCookie(cookieHeader string) string {
	for _, segment := range strings.Split(cookieHeader, ";") {
		segment = strings.TrimSpace(segment)
		parts := strings.SplitN(segment, "=", 2)
		if len(parts) != 2 {
			continue
		}

		name := parts[0]
		if name != SessionCookieName {
			continue
		}

		rawValue := parts[1]
		if rawValue == "" {
			return ""
		}

		decoded, err := url.QueryUnescape(rawValue)
		if err != nil {
			return rawValue
		}
		return decoded
	}

	return ""
}
