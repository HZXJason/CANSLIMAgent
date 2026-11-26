package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DeskHandler struct {
	db      *gorm.DB
	regimes *services.MarketRegimeService
	now     func() time.Time
}

func NewDeskHandler(database *gorm.DB, regimes *services.MarketRegimeService) *DeskHandler {
	return &DeskHandler{db: database, regimes: regimes, now: time.Now}
}

func RegisterDeskRoutes(router gin.IRoutes, handler *DeskHandler) {
	router.GET("/desk", handler.Get)
	router.GET("/market-regime", handler.MarketRegime)
}

func (h *DeskHandler) MarketRegime(c *gin.Context) {
	market := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("market", "US")))
	if market != "US" && market != "CN" {
		Fail(c, http.StatusBadRequest, "market must be US or CN")
		return
	}
	if h.regimes == nil {
		OK(c, services.MarketRegimeReport{
			Market: market, Status: services.RegimeUnavailable, Regime: services.RegimeUnavailable,
			RegimeLabel: "大盘数据暂不可用", Guidance: "大盘指数服务未初始化。", Indices: []services.IndexRegime{},
		})
		return
	}
	OK(c, h.regimes.Get(c.Request.Context(), market))
}

func (h *DeskHandler) Get(c *gin.Context) {
	market := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("market", "US")))
	zone := "America/New_York"
	if market == "CN" {
		zone = "Asia/Shanghai"
	} else if market != "US" {
		Fail(c, http.StatusBadRequest, "market must be US or CN")
		return
	}
	location, _ := time.LoadLocation(zone)
	now := h.now().In(location)

	regime := services.MarketRegimeReport{
		Market: market, Status: services.RegimeUnavailable, Regime: services.RegimeUnavailable,
		RegimeLabel: "大盘数据暂不可用", Guidance: "大盘指数服务未初始化。", Indices: []services.IndexRegime{},
	}
	if h.regimes != nil {
		regime = h.regimes.Get(c.Request.Context(), market)
	}

	_, _ = services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID)
	tradeHandler := NewTradeHandler(h.db, h.regimes)
	summary, err := tradeHandler.computeSummary(c, market)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "desk summary could not be computed")
		return
	}
	allTrades, err := tradeHandler.loadEnrichedTrades(c, "all", market)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "desk trades could not be loaded")
		return
	}

	openTrades := make([]enrichedTradeResponse, 0)
	pendingReviews := make([]enrichedTradeResponse, 0)
	for _, tr := range allTrades {
		if tr.Status == models.TradeStatusOpen {
			openTrades = append(openTrades, tr)
		} else if tr.Status == models.TradeStatusClosed && (tr.TradeReview == nil || tr.TradeReview.CompletedAt == nil) {
			pendingReviews = append(pendingReviews, tr)
		}
	}

	var plans []models.TradePlan
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND status IN ?", models.DefaultUserID, []string{models.TradePlanStatusDraft, models.TradePlanStatusWaiting}).
		Order("updated_at desc").Limit(40).Find(&plans).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "desk plans could not be loaded")
		return
	}
	waitingPlans := make([]tradePlanResponse, 0)
	for _, plan := range plans {
		if todayTradeMarketMatches(market, inferTradeMarket(plan.Ticker)) {
			waitingPlans = append(waitingPlans, newTradePlanResponse(plan))
		}
	}

	OK(c, gin.H{
		"market":          market,
		"timezone":        zone,
		"local_date":      now.Format("2006-01-02"),
		"session":         marketSession(market, now),
		"market_regime":   regime,
		"summary":         summary,
		"open_trades":     openTrades,
		"waiting_plans":   waitingPlans,
		"pending_reviews": pendingReviews,
	})
}
