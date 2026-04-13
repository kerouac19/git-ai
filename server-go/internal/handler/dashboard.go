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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get public statistics: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *DashboardHandler) GetStats(c *gin.Context) {
	userID := c.Query("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	stats, err := h.Svc.GetDashboardStats(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get dashboard statistics: " + err.Error(),
		})
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
