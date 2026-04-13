package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"git-ai-server/internal/auth"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type CompatibilityHandler struct {
	DashboardSvc  *service.DashboardService
	AuthorshipSvc *service.AuthorshipService
	CasSvc        *service.CasService
	DeviceFlowSvc *auth.DeviceFlowService
	MetricsSvc    *service.MetricsService
}

func (h *CompatibilityHandler) GetStatus(c *gin.Context) {
	publicStats, err := h.DashboardSvc.GetPublicStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"service":     "git-ai-private-deploy-server",
		"version":     "1.0.0",
		"modules":     []string{"authorship", "cas", "dashboard", "config"},
		"publicStats": publicStats,
	})
}

func (h *CompatibilityHandler) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "1.0.0",
		"service": "git-ai-private-deploy-server",
	})
}

func (h *CompatibilityHandler) GetMe(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Authenticated user id is required"})
		return
	}
	userMap, ok := user.(gin.H)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user context"})
		return
	}
	userID, _ := userMap["id"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Authenticated user id is required"})
		return
	}

	dashboard, err := h.DashboardSvc.GetDashboardStats(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	records, total, err := h.AuthorshipSvc.FindAll(c.Request.Context(), userID, 10, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"user": gin.H{
			"id":    userID,
			"email": userMap["email"],
			"name":  userMap["name"],
			"role":  userMap["role"],
		},
		"dashboard":              dashboard,
		"recentAuthorship":       records,
		"totalAuthorshipRecords": total,
	})
}

func (h *CompatibilityHandler) StartDeviceFlow(c *gin.Context) {
	host := c.Request.Host
	if host == "" {
		host = "localhost:3000"
	}
	protocol := "http"
	if c.Request.TLS != nil {
		protocol = "https"
	}
	if fwdProto := c.GetHeader("X-Forwarded-Proto"); fwdProto != "" {
		protocol = fwdProto
	}
	baseURL := fmt.Sprintf("%s://%s", protocol, host)

	resp, err := h.DeviceFlowSvc.StartDeviceFlow(c.Request.Context(), baseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *CompatibilityHandler) ExchangeOAuthToken(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid request body",
		})
		return
	}

	grantType, _ := body["grant_type"].(string)
	if grantType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "grant_type is required",
		})
		return
	}

	var response map[string]interface{}
	var err error

	switch grantType {
	case "urn:ietf:params:oauth:grant-type:device_code":
		deviceCode, _ := body["device_code"].(string)
		response, err = h.DeviceFlowSvc.ExchangeDeviceCode(c.Request.Context(), deviceCode)
	case "refresh_token":
		refreshToken, _ := body["refresh_token"].(string)
		response, err = h.DeviceFlowSvc.ExchangeRefreshToken(refreshToken)
	case "install_nonce":
		installNonce, _ := body["install_nonce"].(string)
		response, err = h.DeviceFlowSvc.ExchangeInstallNonce(installNonce)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "unsupported_grant_type",
			"error_description": fmt.Sprintf("Unsupported grant_type: %s", grantType),
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, hasError := response["error"]; hasError {
		c.JSON(http.StatusBadRequest, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CompatibilityHandler) UploadWorkerMetrics(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Authenticated user id is required"})
		return
	}
	userMap, ok := user.(gin.H)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user context"})
		return
	}
	userID, _ := userMap["id"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Authenticated user id is required"})
		return
	}

	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	batch, err := h.MetricsSvc.ValidateBatchShape(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	distinctID := c.GetHeader("x-distinct-id")
	var distinctIDPtr *string
	if distinctID != "" {
		distinctIDPtr = &distinctID
	}

	uploadErrors, err := h.MetricsSvc.UploadBatch(c.Request.Context(), userID, distinctIDPtr, batch)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"errors":  uploadErrors,
	})
}

func (h *CompatibilityHandler) UploadWorkerCas(c *gin.Context) {
	contentType := c.ContentType()

	if strings.HasPrefix(contentType, "application/json") {
		var body struct {
			Objects []service.CasUploadRequest `json:"objects"`
		}
		if err := c.ShouldBindJSON(&body); err == nil && len(body.Objects) > 0 {
			result, err := h.CasSvc.UploadObjects(c.Request.Context(), body.Objects)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, result)
			return
		}
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Either JSON body \"objects\" or multipart file field \"file\" is required",
		})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
		return
	}

	ct := c.Query("contentType")
	if ct == "" {
		ct = header.Header.Get("Content-Type")
	}
	if ct == "" {
		ct = "application/octet-stream"
	}

	hash, err := h.CasSvc.UploadContent(c.Request.Context(), string(data), ct)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"object_id":   hash,
		"hash":        hash,
		"contentType": ct,
		"message":     "Content uploaded successfully",
	})
}

func (h *CompatibilityHandler) ReadWorkerCas(c *gin.Context) {
	hashes := c.Query("hashes")
	if strings.TrimSpace(hashes) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter \"hashes\" is required"})
		return
	}

	hashList := make([]string, 0)
	for _, h := range strings.Split(hashes, ",") {
		trimmed := strings.TrimSpace(h)
		if trimmed != "" {
			hashList = append(hashList, trimmed)
		}
	}

	if len(hashList) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter \"hashes\" is required"})
		return
	}

	if len(hashList) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A maximum of 100 hashes is supported per request"})
		return
	}

	result, err := h.CasSvc.ReadObjects(c.Request.Context(), hashList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *CompatibilityHandler) CheckoutWorkerCas(c *gin.Context) {
	targetHash := c.Query("id")
	if targetHash == "" {
		targetHash = c.Query("hash")
	}
	if targetHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter \"id\" or \"hash\" is required"})
		return
	}

	result, err := h.CasSvc.ReadContent(c.Request.Context(), targetHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Content not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"object_id":   targetHash,
		"hash":        targetHash,
		"content":     result.Content,
		"contentType": result.ContentType,
	})
}
