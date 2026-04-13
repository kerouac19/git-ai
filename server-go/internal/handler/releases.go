package handler

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

var releaseChannels = []string{"latest", "next", "enterprise-latest", "enterprise-next"}

type ReleaseHandler struct{}

func (h *ReleaseHandler) GetReleases(c *gin.Context) {
	sums := releaseSHA256SUMS()
	sum := sha256.Sum256([]byte(sums))
	checksum := fmt.Sprintf("%x", sum[:])
	version := strings.TrimSpace(os.Getenv("GIT_AI_RELEASE_TAG"))
	if version == "" {
		version = "v0.0.0"
	}

	channels := gin.H{}
	for _, channel := range releaseChannels {
		channels[channel] = gin.H{
			"version":  version,
			"checksum": checksum,
		}
	}

	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

func (h *ReleaseHandler) Download(c *gin.Context) {
	channel := c.Param("channel")
	if !knownReleaseChannel(channel) {
		c.JSON(http.StatusNotFound, gin.H{"error": "release channel not found"})
		return
	}

	name := c.Param("name")
	switch name {
	case "SHA256SUMS":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(releaseSHA256SUMS()))
	case "install.sh":
		c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(installSH()))
	case "install.ps1":
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(installPS1()))
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "release artifact not found"})
	}
}

func knownReleaseChannel(channel string) bool {
	for _, c := range releaseChannels {
		if c == channel {
			return true
		}
	}
	return false
}

func releaseSHA256SUMS() string {
	sh := sha256.Sum256([]byte(installSH()))
	ps1 := sha256.Sum256([]byte(installPS1()))
	return fmt.Sprintf("%x  install.sh\n%x  install.ps1\n",
		sh[:],
		ps1[:],
	)
}

func installSH() string {
	return `#!/bin/sh
set -eu
echo "This private git-ai server does not publish install.sh artifacts."
echo "Install or upgrade git-ai from your organization's approved distribution channel."
exit 1
`
}

func installPS1() string {
	return `Write-Error "This private git-ai server does not publish install.ps1 artifacts. Install or upgrade git-ai from your organization's approved distribution channel."
exit 1
`
}
