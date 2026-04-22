package handler

import (
	"errors"
	"io"
	"net/http"

	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

const maxReleaseUploadBytes = 1 << 20 // 1 MiB

// ReleaseAdminHandler serves write-side (operator) release endpoints guarded
// by a shared upload Bearer token.
type ReleaseAdminHandler struct {
	Store *service.ReleaseStore
}

// PutArtifact handles PUT /api/releases/:channel/artifacts/:tag/:name
func (h *ReleaseAdminHandler) PutArtifact(c *gin.Context) {
	channel := c.Param("channel")
	tag := c.Param("tag")
	name := c.Param("name")

	if !service.KnownReleaseChannel(channel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
		return
	}
	if err := service.ValidateTag(tag); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag"})
		return
	}
	if !service.KnownReleaseArtifact(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact name"})
		return
	}

	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxReleaseUploadBytes)
	defer body.Close()

	err := h.Store.PutArtifact(channel, tag, name, body)
	if err != nil {
		// http.MaxBytesReader returns *http.MaxBytesError on Go 1.19+; also
		// detect by message for older runtimes.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
			return
		}
		switch {
		case errors.Is(err, service.ErrReleaseConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "artifact exists with different content"})
		case errors.Is(err, service.ErrReleaseInvalidChannel),
			errors.Is(err, service.ErrReleaseInvalidTag),
			errors.Is(err, service.ErrReleaseInvalidName):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			Internal(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PutCurrent handles PUT /api/releases/:channel/current.json
func (h *ReleaseAdminHandler) PutCurrent(c *gin.Context) {
	channel := c.Param("channel")
	if !service.KnownReleaseChannel(channel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
		return
	}

	limited := http.MaxBytesReader(c.Writer, c.Request.Body, maxReleaseUploadBytes)
	defer limited.Close()
	body, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.Store.PutCurrent(channel, body); err != nil {
		switch {
		case errors.Is(err, service.ErrReleaseChecksumMismatch):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrReleaseBadCurrentJSON),
			errors.Is(err, service.ErrReleaseMissingArtifact),
			errors.Is(err, service.ErrReleaseInvalidTag),
			errors.Is(err, service.ErrReleaseInvalidChannel):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			Internal(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetCurrent handles GET /api/releases/:channel/current.json (for sync
// scripts to check whether the pointer is already up-to-date).
func (h *ReleaseAdminHandler) GetCurrent(c *gin.Context) {
	channel := c.Param("channel")
	if !service.KnownReleaseChannel(channel) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
		return
	}
	raw, err := h.Store.GetCurrentRaw(channel)
	if err != nil {
		if errors.Is(err, service.ErrReleaseNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		Internal(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}
