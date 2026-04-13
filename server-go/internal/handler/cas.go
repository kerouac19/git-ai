package handler

import (
	"net/http"

	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type CasHandler struct {
	Svc *service.CasService
}

func (h *CasHandler) Upload(c *gin.Context) {
	var body struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is required"})
		return
	}

	hash, err := h.Svc.UploadContent(c.Request.Context(), body.Content, body.ContentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to upload content: " + err.Error(),
		})
		return
	}

	ct := body.ContentType
	if ct == "" {
		ct = "text/plain"
	}

	c.JSON(http.StatusOK, gin.H{
		"hash":        hash,
		"success":     true,
		"message":     "Content uploaded successfully",
		"contentType": ct,
	})
}

func (h *CasHandler) Read(c *gin.Context) {
	hash := c.Param("hash")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Hash is required"})
		return
	}

	result, err := h.Svc.ReadContent(c.Request.Context(), hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read content: " + err.Error(),
		})
		return
	}

	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Content not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"content":     result.Content,
		"hash":        hash,
		"success":     true,
		"contentType": result.ContentType,
	})
}
