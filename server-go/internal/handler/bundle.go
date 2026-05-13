package handler

import (
	"fmt"
	"net/http"

	"git-ai-server/internal/middleware"
	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type BundleHandler struct {
	Svc        *service.BundleService
	TrustProxy bool
}

func (h *BundleHandler) Create(c *gin.Context) {
	userID, _, ok := userSubjectAndRole(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
		return
	}

	var req model.CreateBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
		return
	}

	bundle, err := h.Svc.CreateBundle(c.Request.Context(), userID, req)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, model.CreateBundleResponse{
		Success: true,
		ID:      bundle.ID,
		URL:     publicURL(c, h.TrustProxy, "/bundles/"+bundle.ID),
	})
}

func publicURL(c *gin.Context, trustProxy bool, path string) string {
	host := c.Request.Host
	if host == "" {
		host = "localhost"
	}
	protocol := "http"
	if c.Request.TLS != nil {
		protocol = "https"
	}
	protocol = middleware.ForwardedProtoOrDefault(c.Request, trustProxy, protocol)
	return fmt.Sprintf("%s://%s%s", protocol, host, path)
}
