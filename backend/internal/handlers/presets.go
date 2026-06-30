package handlers

import (
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PresetHandler struct {
	db *gorm.DB
}

func NewPresetHandler(database *gorm.DB) *PresetHandler {
	return &PresetHandler{db: database}
}

func (h *PresetHandler) List(c *gin.Context) {
	var presets []models.ThresholdPreset
	if err := h.db.WithContext(c.Request.Context()).Order("is_default desc, created_at asc").Find(&presets).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "阈值方案读取失败")
		return
	}
	OK(c, presets)
}

func (h *PresetHandler) Create(c *gin.Context) {
	var req models.ThresholdPreset
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	req.ID = 0
	req.Name = strings.TrimSpace(req.Name)
	req.IsDefault = false
	if req.Name == "" {
		Fail(c, http.StatusBadRequest, "方案名称不能为空")
		return
	}
	if req.AMinYears <= 0 {
		req.AMinYears = 3
	}
	if req.SMAPeriod <= 0 {
		req.SMAPeriod = 50
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&req).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "阈值方案保存失败")
		return
	}
	OK(c, req)
}

func (h *PresetHandler) Delete(c *gin.Context) {
	var preset models.ThresholdPreset
	if err := h.db.WithContext(c.Request.Context()).First(&preset, "id = ?", c.Param("id")).Error; err != nil {
		Fail(c, http.StatusNotFound, "阈值方案不存在")
		return
	}
	if preset.IsDefault {
		Fail(c, http.StatusBadRequest, "奥尼尔默认方案不可删除")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Delete(&preset).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "阈值方案删除失败")
		return
	}
	OK(c, gin.H{"deleted": true})
}
