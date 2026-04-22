package handler

import (
	"net/http"
	"time"

	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	Svc *service.DashboardService
}

func (h *DashboardHandler) GetPublicStats(c *gin.Context) {
	stats, err := h.Svc.GetPublicStats(c.Request.Context())
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) GetStats(c *gin.Context) {
	// Ignore ?userId=; the dashboard belongs to the authenticated caller.
	// Admin cross-tenant inspection is a separate endpoint we haven't
	// designed yet — don't let query parameters widen the scope.
	subject, _, ok := userSubjectAndRole(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
		return
	}

	stats, err := h.Svc.GetDashboardStats(c.Request.Context(), subject)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) GenerateReport(c *gin.Context) {
	var params map[string]interface{}
	_ = c.ShouldBindJSON(&params)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Report generation initiated",
		"reportId": "sample-report-id",
	})
}
