package handler

import (
	"net/http"

	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

type SysConfigHandler struct {
	Svc *service.SysConfigService
}

func (h *SysConfigHandler) GetAll(c *gin.Context) {
	category := c.Query("category")
	key := c.Query("key")

	configs, err := h.Svc.GetAllConfigs(c.Request.Context(), category, key)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
		"message": "Configurations retrieved successfully",
	})
}

func (h *SysConfigHandler) GetByKey(c *gin.Context) {
	key := c.Param("key")

	config, err := h.Svc.GetConfig(c.Request.Context(), key)
	if err != nil {
		Internal(c, err)
		return
	}

	if config == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"data":    nil,
			"message": "Configuration not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
		"message": "Configuration retrieved successfully",
	})
}

func (h *SysConfigHandler) Create(c *gin.Context) {
	var dto model.CreateConfigDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := h.Svc.CreateConfig(c.Request.Context(), dto)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    config,
		"message": "Configuration created successfully",
	})
}

func (h *SysConfigHandler) Update(c *gin.Context) {
	key := c.Param("key")

	var dto model.UpdateConfigDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config, err := h.Svc.UpdateConfig(c.Request.Context(), key, dto)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
		"message": "Configuration updated successfully",
	})
}

func (h *SysConfigHandler) Delete(c *gin.Context) {
	key := c.Param("key")

	err := h.Svc.DeleteConfig(c.Request.Context(), key)
	if err != nil {
		Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nil,
		"message": "Configuration deleted successfully",
	})
}
