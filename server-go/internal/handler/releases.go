package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

// readableReleaseChannels is the list of channels the client may request from
// /worker/releases. Order matches what the plan describes.
var readableReleaseChannels = []string{"latest", "next", "enterprise-latest", "enterprise-next"}

// ReleaseHandler serves read-side (client-facing) release endpoints.
type ReleaseHandler struct {
	Store *service.ReleaseStore
}

// GetReleases handles GET /worker/releases. Returns a channels map with
// version+checksum per channel. Enterprise channels fall back to their public
// counterparts via the store's ResolveEffectiveChannel.
func (h *ReleaseHandler) GetReleases(c *gin.Context) {
	channels := gin.H{}
	for _, requested := range readableReleaseChannels {
		effective, ok := h.Store.ResolveEffectiveChannel(requested)
		if !ok {
			continue
		}
		raw, err := h.Store.GetCurrentRaw(effective)
		if err != nil {
			continue
		}
		var cur service.CurrentPointer
		if err := json.Unmarshal(raw, &cur); err != nil {
			continue
		}
		if cur.Tag == "" || cur.Checksum == "" {
			continue
		}
		channels[requested] = gin.H{
			"version":  cur.Tag,
			"checksum": cur.Checksum,
		}
	}
	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

// Download handles GET /worker/releases/:channel/download/:name. It resolves
// the channel (with enterprise fallback), reads current.json to find the tag,
// and streams the artifact file.
func (h *ReleaseHandler) Download(c *gin.Context) {
	channel := c.Param("channel")
	name := c.Param("name")

	if !service.KnownReleaseChannel(channel) {
		c.JSON(http.StatusNotFound, gin.H{"error": "release channel not found"})
		return
	}
	if !service.KnownReleaseArtifact(name) {
		c.JSON(http.StatusNotFound, gin.H{"error": "release artifact not found"})
		return
	}

	rc, _, err := h.Store.OpenCurrentArtifact(channel, name)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReleaseNotFound):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no releases synchronized yet"})
		case errors.Is(err, service.ErrReleaseMissingArtifact):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "release data inconsistent"})
		case errors.Is(err, service.ErrReleaseBadCurrentJSON),
			errors.Is(err, service.ErrReleaseInvalidTag):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "release pointer malformed"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	defer rc.Close()

	switch name {
	case "SHA256SUMS":
		c.Header("Content-Type", "text/plain; charset=utf-8")
	case "install.sh":
		c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	case "install.ps1":
		c.Header("Content-Type", "text/plain; charset=utf-8")
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}
