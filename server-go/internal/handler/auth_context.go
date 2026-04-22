package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// userSubjectAndRole extracts the authenticated principal's id and role from
// the gin context. Returns false when no user is on the context (caller made
// a mistake by mounting an auth'd endpoint without an auth middleware).
func userSubjectAndRole(c *gin.Context) (subject string, role string, ok bool) {
	u, exists := c.Get("user")
	if !exists {
		return "", "", false
	}
	m, ok := u.(gin.H)
	if !ok {
		return "", "", false
	}
	id, _ := m["id"].(string)
	r, _ := m["role"].(string)
	return id, r, id != ""
}

// requireSelfOrAdmin enforces that the authenticated principal is either
// acting on their own data (claims.sub == targetUserID) or has the admin
// role. When the check fails, a 403 response is written and the function
// returns false so handlers can early-return.
func requireSelfOrAdmin(c *gin.Context, targetUserID string) bool {
	subject, role, ok := userSubjectAndRole(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "Authorization required",
		})
		return false
	}
	if role == "admin" {
		return true
	}
	if targetUserID != "" && subject == targetUserID {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": "Forbidden",
	})
	return false
}
