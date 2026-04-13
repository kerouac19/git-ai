package handler

import (
	"net/http"
	"strconv"

	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthorshipHandler struct {
	Svc *service.AuthorshipService
}

func (h *AuthorshipHandler) SaveRecord(c *gin.Context) {
	var dto model.AuthorshipRecordDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rec, err := h.Svc.SaveRecord(c.Request.Context(), dto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save authorship record: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Authorship record saved",
		"recordId": rec.ID,
	})
}

func (h *AuthorshipHandler) SaveCommitAttribution(c *gin.Context) {
	var dto model.CommitAttributionDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attr, err := h.Svc.SaveCommitAttribution(c.Request.Context(), dto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save commit attribution: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Commit attribution saved",
		"attributionId": attr.ID,
	})
}

func (h *AuthorshipHandler) GetUserCommits(c *gin.Context) {
	userID := c.Param("userId")

	limit := parsePaginationValue(c.Query("limit"), 50)
	offset := parsePaginationValue(c.Query("offset"), 0)

	records, total, err := h.Svc.FindAll(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve authorship records: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    records,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *AuthorshipHandler) GetUserCommitByHash(c *gin.Context) {
	userID := c.Param("userId")
	commitHash := c.Param("commitHash")

	records, err := h.Svc.FindByCommitHash(c.Request.Context(), commitHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve authorship records: " + err.Error(),
		})
		return
	}

	filtered := make([]model.AuthorshipRecord, 0)
	for _, rec := range records {
		if rec.UserID == userID {
			filtered = append(filtered, rec)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    filtered,
	})
}

func (h *AuthorshipHandler) GetCommitAttribution(c *gin.Context) {
	commitHash := c.Param("commitHash")

	attr, err := h.Svc.FindCommitAttributionByHash(c.Request.Context(), commitHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve commit attribution: " + err.Error(),
		})
		return
	}

	if attr == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Commit attribution not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    attr,
	})
}

func (h *AuthorshipHandler) SyncAuthorship(c *gin.Context) {
	userID := c.Param("userId")

	var dto model.AuthorshipRecordDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rec, err := h.Svc.MergeAuthorshipData(c.Request.Context(), userID, dto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to sync authorship data: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Authorship data synced successfully",
		"recordId": rec.ID,
	})
}

func parsePaginationValue(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < 0 {
		return 0
	}
	return parsed
}
