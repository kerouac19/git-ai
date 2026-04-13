package handler

import (
	"net/http"
	"strings"
	"time"

	"git-ai-server/internal/auth"
	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	loginAccessTokenTTL  = 3600 * time.Second
	loginRefreshTokenTTL = 90 * 24 * time.Hour
)

type LoginHandler struct {
	UserSvc      *service.UserService
	JWTSecret    string
	IsProduction bool
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (h *LoginHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Username and password are required"})
		return
	}

	user, err := h.UserSvc.FindByUsernameOrEmail(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid username or password"})
		return
	}

	if user.Status != model.UserStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Account is disabled"})
		return
	}

	if err := service.ValidatePassword(user.PasswordHash, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid username or password"})
		return
	}

	subject := userToSubject(user)

	accessToken, err := auth.SignAccessToken(subject, h.JWTSecret, loginAccessTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue token"})
		return
	}

	refreshToken, err := auth.SignRefreshToken(auth.TokenSubject{
		Sub:   subject.Sub,
		Email: subject.Email,
		Name:  subject.Name,
		Role:  subject.Role,
	}, h.JWTSecret, loginRefreshTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue token"})
		return
	}

	c.Header("Set-Cookie", auth.SerializeSessionCookie(accessToken, int(loginAccessTokenTTL.Seconds()), h.IsProduction))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"role":         user.Role,
			"status":       user.Status,
		},
		"access_token":       accessToken,
		"token_type":         "Bearer",
		"expires_in":         int(loginAccessTokenTTL.Seconds()),
		"refresh_token":      refreshToken,
		"refresh_expires_in": int(loginRefreshTokenTTL.Seconds()),
	})
}

func (h *LoginHandler) Logout(c *gin.Context) {
	c.Header("Set-Cookie", auth.ClearSessionCookie(h.IsProduction))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out"})
}

func (h *LoginHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Username and password are required"})
		return
	}

	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Password must be at least 8 characters"})
		return
	}

	hash, err := service.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Internal error"})
		return
	}

	user := &model.User{
		Username:     req.Username,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Role:         "user",
		Status:       model.UserStatusEnabled,
	}

	if err := h.UserSvc.Create(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Username or email already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "User created"})
}

func userToSubject(user *model.User) auth.TokenSubject {
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	return auth.TokenSubject{
		Sub:           user.ID,
		Email:         user.Email,
		Name:          name,
		PersonalOrgID: user.ID,
		Role:          user.Role,
		Orgs: []auth.Org{
			{
				OrgID:   user.ID,
				OrgName: name,
				OrgSlug: user.Username,
				Role:    user.Role,
			},
		},
	}
}
