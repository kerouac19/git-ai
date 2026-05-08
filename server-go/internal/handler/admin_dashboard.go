package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git-ai-server/internal/model"

	"github.com/gin-gonic/gin"
)

// AdminDashboardSvc is the surface AdminDashboardHandler depends on. Defined
// here so tests can swap in a fake without touching the real DB.
type AdminDashboardSvc interface {
	GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error)
}

type AdminDashboardHandler struct {
	Svc AdminDashboardSvc
}

var validAdminRanges = map[string]bool{"7d": true, "30d": true}

func (h *AdminDashboardHandler) GetGlobalStats(c *gin.Context) {
	rangeKey := c.DefaultQuery("range", "7d")
	if !validAdminRanges[rangeKey] {
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
	if data == nil {
		Internal(c, errors.New("admin dashboard service returned nil data"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
