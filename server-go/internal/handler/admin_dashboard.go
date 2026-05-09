package handler

import (
	"context"
	"net/http"
	"time"

	"git-ai-server/internal/model"

	"github.com/gin-gonic/gin"
)

// AdminDashboardSvc is the surface the dashboard handler depends on. Defined
// here so tests can swap in a fake without touching the real DB. (Type name
// is preserved for now; renaming is deferred churn.)
type AdminDashboardSvc interface {
	GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error)
}

type AdminDashboardHandler struct {
	Svc AdminDashboardSvc
}

// GetGlobalStats returns the cross-user/cross-org dashboard payload to any
// authenticated user. Routed at GET /api/dashboard/global.
func (h *AdminDashboardHandler) GetGlobalStats(c *gin.Context) {
	rangeKey := c.DefaultQuery("range", "7d")
	if rangeKey != "7d" && rangeKey != "30d" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid range; expected 7d or 30d",
		})
		return
	}

	data, err := h.Svc.GetGlobalStats(c.Request.Context(), rangeKey)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
