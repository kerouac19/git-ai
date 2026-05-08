package handler

import (
	"context"
	"net/http"
	"time"

	"git-ai-server/internal/auth"

	"github.com/gin-gonic/gin"
)

// deviceFlowService is the subset of *auth.DeviceFlowService used by this
// handler. Declared as an interface for testability.
type deviceFlowService interface {
	GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	ApproveDeviceCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	DenyDeviceCode(ctx context.Context, userCode string) (*auth.DeviceCodeInfo, error)
	UpdateDeviceCodeSubject(ctx context.Context, userCode string, subject auth.TokenSubject) error
	DecodeAccessToken(accessToken string) (*auth.Claims, error)
}

// Compile-time assertion: *auth.DeviceFlowService must satisfy deviceFlowService.
var _ deviceFlowService = (*auth.DeviceFlowService)(nil)

type DeviceFlowHandler struct {
	Svc deviceFlowService
}

type deviceFlowBody struct {
	UserCode string `json:"user_code"`
}

type deviceFlowSubject struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type deviceFlowInfoResponse struct {
	UserCode      string             `json:"user_code"`
	Status        string             `json:"status"`
	ExpiresAt     *string            `json:"expires_at,omitempty"`
	Authenticated bool               `json:"authenticated"`
	Subject       *deviceFlowSubject `json:"subject,omitempty"`
}

func (h *DeviceFlowHandler) Info(c *gin.Context) {
	userCode := c.Query("user_code")
	if userCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	entry, err := h.Svc.GetDeviceCodeByUserCode(c.Request.Context(), userCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}

	resp := deviceFlowInfoResponse{
		UserCode: entry.UserCode,
		Status:   entry.Status,
	}
	if entry.ExpiresAt > 0 {
		s := time.UnixMilli(entry.ExpiresAt).Format(time.RFC3339)
		resp.ExpiresAt = &s
	}

	if claims := h.claimsFromCookie(c); claims != nil && claims.Subject != "" {
		resp.Authenticated = true
		resp.Subject = &deviceFlowSubject{Name: claims.Name, Email: claims.Email}
	} else if entry.Subject != nil {
		resp.Subject = &deviceFlowSubject{Name: entry.Subject.Name, Email: entry.Subject.Email}
	}

	c.JSON(http.StatusOK, resp)
}

func (h *DeviceFlowHandler) Approve(c *gin.Context) {
	var body deviceFlowBody
	if err := c.ShouldBindJSON(&body); err != nil || body.UserCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	claims := h.claimsFromCookie(c)
	if claims == nil || claims.Subject == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}

	// Look up current state first so we can correctly map already-handled codes.
	current, err := h.Svc.GetDeviceCodeByUserCode(c.Request.Context(), body.UserCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}
	if current.Status == "denied" {
		c.JSON(http.StatusConflict, gin.H{"error": "device code already denied"})
		return
	}
	if current.Status == "approved" {
		c.JSON(http.StatusOK, gin.H{"status": "approved"})
		return
	}

	realSubject := auth.TokenSubject{
		Sub:           claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		PersonalOrgID: claims.PersonalOrgID,
		Orgs:          claims.Orgs,
		Role:          claims.Role,
	}
	if err := h.Svc.UpdateDeviceCodeSubject(c.Request.Context(), body.UserCode, realSubject); err != nil {
		Internal(c, err)
		return
	}

	entry, err := h.Svc.ApproveDeviceCode(c.Request.Context(), body.UserCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": entry.Status})
}

func (h *DeviceFlowHandler) Deny(c *gin.Context) {
	var body deviceFlowBody
	if err := c.ShouldBindJSON(&body); err != nil || body.UserCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_code is required"})
		return
	}

	entry, err := h.Svc.DenyDeviceCode(c.Request.Context(), body.UserCode)
	if err != nil {
		Internal(c, err)
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device code not found or expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": entry.Status})
}

func (h *DeviceFlowHandler) claimsFromCookie(c *gin.Context) *auth.Claims {
	token := auth.ExtractAccessTokenFromCookie(c.GetHeader("Cookie"))
	if token == "" {
		return nil
	}
	claims, err := h.Svc.DecodeAccessToken(token)
	if err != nil {
		return nil
	}
	return claims
}
