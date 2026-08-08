package handlers

import (
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ScreenDefinitionHandler struct{ db *gorm.DB }

func NewScreenDefinitionHandler(database *gorm.DB) *ScreenDefinitionHandler {
	return &ScreenDefinitionHandler{db: database}
}

func (h *ScreenDefinitionHandler) List(c *gin.Context) {
	var definitions []models.ScreenDefinition
	query := h.db.WithContext(c.Request.Context()).Order("is_system desc, created_at asc").Limit(200)
	if market := strings.ToUpper(strings.TrimSpace(c.Query("asset_market"))); market != "" {
		query = query.Where("asset_market = ?", market)
	}
	if err := query.Find(&definitions).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "筛选模板读取失败")
		return
	}
	OK(c, definitions)
}

func (h *ScreenDefinitionHandler) Create(c *gin.Context) {
	var definition models.ScreenDefinition
	if err := c.ShouldBindJSON(&definition); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	definition.ID = [16]byte{}
	definition.Name = strings.TrimSpace(definition.Name)
	definition.AssetMarket = strings.ToUpper(strings.TrimSpace(definition.AssetMarket))
	definition.StockPool = strings.ToUpper(strings.TrimSpace(definition.StockPool))
	definition.IsSystem = false
	if message := validateScreenDefinition(definition); message != "" {
		Fail(c, http.StatusBadRequest, message)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&definition).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "筛选模板保存失败")
		return
	}
	OK(c, definition)
}

func validateScreenDefinition(definition models.ScreenDefinition) string {
	if definition.Name == "" {
		return "模板名称不能为空"
	}
	if definition.AssetMarket != "US" && definition.AssetMarket != "CN" {
		return "asset_market must be US or CN"
	}
	if !models.ValidMarket(definition.StockPool) {
		return "stock_pool is not supported"
	}
	if len(definition.Conditions) == 0 || len(definition.Conditions) > 30 {
		return "conditions must contain 1 to 30 rules"
	}
	fields := map[string]bool{"quarterly_eps_growth": true, "annual_eps_growth": true, "volume_ratio": true, "relative_strength": true, "institution_delta": true, "avg_dollar_volume": true, "volatility": true}
	for _, condition := range definition.Conditions {
		if !fields[condition.Field] {
			return "unsupported condition field: " + condition.Field
		}
		if condition.Operator != "gte" && condition.Operator != "lte" {
			return "operator must be gte or lte"
		}
		if condition.UnknownPolicy != "unknown" {
			return "unknown data must remain unknown"
		}
	}
	return ""
}
