package handler

import (
	"fmt"
	"net/http"
	"strings"

	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type BundleHandler struct {
	Svc *service.BundleService
}

func (h *BundleHandler) Create(c *gin.Context) {
	var req model.CreateBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	bundle, err := h.Svc.CreateBundle(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Failed to create bundle",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.CreateBundleResponse{
		Success: true,
		ID:      bundle.ID,
		URL:     publicURL(c, "/bundles/"+bundle.ID),
	})
}

func publicURL(c *gin.Context, path string) string {
	host := c.Request.Host
	if host == "" {
		host = "localhost"
	}
	protocol := "http"
	if c.Request.TLS != nil {
		protocol = "https"
	}
	if fwdProto := c.GetHeader("X-Forwarded-Proto"); fwdProto != "" {
		protocol = strings.TrimSpace(fwdProto)
	}
	return fmt.Sprintf("%s://%s%s", protocol, host, path)
}
