package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func JWTAuthMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}

		claims, err := VerifyToken(tokenString, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		if claims.Type != "" && claims.Type != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		c.Set("user", gin.H{
			"id":              claims.Subject,
			"email":           claims.Email,
			"name":            claims.Name,
			"role":            claims.Role,
			"personal_org_id": claims.PersonalOrgID,
			"orgs":            claims.Orgs,
		})

		c.Next()
	}
}

func WorkerAuthMiddleware(secret string, apiKeys []string, apiKeySubject TokenSubject) gin.HandlerFunc {
	acceptedAPIKeys := make(map[string]struct{}, len(apiKeys))
	for _, key := range apiKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed != "" {
			acceptedAPIKeys[trimmed] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		tokenString := extractToken(c)
		if tokenString != "" {
			if claims, err := VerifyToken(tokenString, secret); err == nil {
				if claims.Type == "" || claims.Type == "access" {
					setUserFromClaims(c, claims)
					c.Next()
					return
				}
			}
		}

		apiKey := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if apiKey != "" {
			if _, ok := acceptedAPIKeys[apiKey]; ok {
				setUserFromSubject(c, apiKeySubject)
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
	}
}

func extractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	cookieHeader := c.GetHeader("Cookie")
	if cookieHeader != "" {
		if token := ExtractAccessTokenFromCookie(cookieHeader); token != "" {
			return token
		}
	}

	return ""
}

func setUserFromClaims(c *gin.Context, claims *Claims) {
	c.Set("user", gin.H{
		"id":              claims.Subject,
		"email":           claims.Email,
		"name":            claims.Name,
		"role":            claims.Role,
		"personal_org_id": claims.PersonalOrgID,
		"orgs":            claims.Orgs,
	})
}

func setUserFromSubject(c *gin.Context, subject TokenSubject) {
	c.Set("user", gin.H{
		"id":              subject.Sub,
		"email":           subject.Email,
		"name":            subject.Name,
		"role":            subject.Role,
		"personal_org_id": subject.PersonalOrgID,
		"orgs":            subject.Orgs,
	})
}
