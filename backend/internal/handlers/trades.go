package handlers

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TradeHandler struct {
	db      *gorm.DB
	regimes *services.MarketRegimeService
}

func NewTradeHandler(database *gorm.DB, regimes *services.MarketRegimeService) *TradeHandler {
	return &TradeHandler{db: database, regimes: regimes}
}

func RegisterTradeRoutes(router gin.IRoutes, handler *TradeHandler) {
	router.GET("/trades", handler.List)
	router.GET("/trades/summary", handler.Summary)
	router.GET("/trades/:id", handler.Get)
	router.PATCH("/trades/:id/stop-loss", handler.UpdateStopLoss)
	router.POST("/trades/:id/link-plan", handler.LinkPlan)
	router.PUT("/trades/:id/review", handler.UpsertReview)
	router.POST("/position-sizer", handler.PositionSizer)
}

type enrichedTradeResponse struct {
	models.Trade
	Fills       []journalEntryResponse `json:"fills"`
	TradePlan   *tradePlanResponse     `json:"trade_plan,omitempty"`
	TradeReview *models.TradeReview    `json:"trade_review,omitempty"`
}

func (h *TradeHandler) List(c *gin.Context) {
	statusFilter := strings.ToLower(strings.TrimSpace(c.Query("status")))
	marketFilter := strings.ToUpper(strings.TrimSpace(c.Query("market")))

	if _, err := services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID); err != nil {
		Fail(c, http.StatusInternalServerError, "trades could not be synced")
		return
	}

	trades, err := h.loadEnrichedTrades(c, statusFilter, marketFilter)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trades could not be loaded")
		return
	}
	OK(c, trades)
}

func (h *TradeHandler) Summary(c *gin.Context) {
	if _, err := services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID); err != nil {
		Fail(c, http.StatusInternalServerError, "trades could not be synced")
		return
	}
	summary, err := h.computeSummary(c, strings.ToUpper(strings.TrimSpace(c.Query("market"))))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trades summary could not be computed")
		return
	}
	OK(c, summary)
}

func (h *TradeHandler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "交易仓位")
	if !ok {
		return
	}
	var trade models.Trade
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&trade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, "trade not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade could not be loaded")
		return
	}
	enriched, err := h.enrichTrades(c, []models.Trade{trade})
	if err != nil || len(enriched) == 0 {
		Fail(c, http.StatusInternalServerError, "trade details could not be loaded")
		return
	}
	OK(c, enriched[0])
}

func (h *TradeHandler) UpdateStopLoss(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "交易仓位")
	if !ok {
		return
	}
	var req struct {
		StopLoss *float64 `json:"stop_loss"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.StopLoss == nil || *req.StopLoss <= 0 {
		Fail(c, http.StatusBadRequest, "stop_loss must be positive")
		return
	}
	var trade models.Trade
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&trade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, "trade not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade could not be loaded")
		return
	}
	if trade.Status != models.TradeStatusOpen {
		Fail(c, http.StatusBadRequest, "only open trades can update stop loss")
		return
	}

	stop := *req.StopLoss
	openRisk := 0.0
	if trade.Direction == models.TradeDirectionShort {
		if stop > trade.AvgEntryPrice {
			openRisk = math.Round((stop-trade.AvgEntryPrice)*trade.OpenQuantity*10000) / 10000
		}
	} else if trade.AvgEntryPrice > stop {
		openRisk = math.Round((trade.AvgEntryPrice-stop)*trade.OpenQuantity*10000) / 10000
	}

	violations := make([]string, 0, len(trade.AutoViolations))
	for _, code := range trade.AutoViolations {
		if code != "missing_stop_loss" {
			violations = append(violations, code)
		}
	}

	now := time.Now().UTC()
	trade.CurrentStopLoss = &stop
	trade.OpenRiskAmount = openRisk
	trade.AutoViolations = violations
	trade.UpdatedAt = now
	if err := h.db.WithContext(c.Request.Context()).Model(&trade).Select("current_stop_loss", "open_risk_amount", "auto_violations", "updated_at").Updates(&trade).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade stop_loss could not be updated")
		return
	}
	_, _ = services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID)
	_ = h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&trade).Error
	OK(c, trade)
}

func (h *TradeHandler) LinkPlan(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "交易仓位")
	if !ok {
		return
	}
	var req struct {
		TradePlanID string `json:"trade_plan_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	planID, err := uuid.Parse(strings.TrimSpace(req.TradePlanID))
	if err != nil {
		Fail(c, http.StatusBadRequest, "trade_plan_id is invalid")
		return
	}
	var trade models.Trade
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&trade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, "trade not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade could not be loaded")
		return
	}
	var plan models.TradePlan
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", planID, models.DefaultUserID).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, "trade plan not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade plan could not be loaded")
		return
	}
	if !strings.EqualFold(plan.Ticker, trade.Symbol) {
		Fail(c, http.StatusBadRequest, "trade plan ticker does not match trade symbol")
		return
	}

	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.TradeJournalEntry{}).
			Where("user_id = ? AND trade_id = ? AND trade_plan_id IS NULL", models.DefaultUserID, trade.ID).
			Update("trade_plan_id", plan.ID).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Trade{}).
			Where("user_id = ? AND id = ?", models.DefaultUserID, trade.ID).
			Update("trade_plan_id", plan.ID).Error; err != nil {
			return err
		}
		if plan.Status == models.TradePlanStatusDraft || plan.Status == models.TradePlanStatusWaiting {
			now := time.Now().UTC()
			if err := tx.Model(&models.TradePlan{}).
				Where("id = ? AND user_id = ?", plan.ID, models.DefaultUserID).
				Updates(map[string]any{"status": models.TradePlanStatusExecuted, "executed_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade could not be linked to plan")
		return
	}
	_, _ = services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID)
	enriched, err := h.loadEnrichedTrades(c, "", "")
	if err == nil {
		for _, item := range enriched {
			if item.ID == trade.ID {
				OK(c, item)
				return
			}
		}
	}
	OK(c, trade)
}

func (h *TradeHandler) UpsertReview(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "交易仓位")
	if !ok {
		return
	}
	var req struct {
		ProcessQuality  string  `json:"process_quality"`
		DeviationReason *string `json:"deviation_reason"`
		EmotionBefore   *string `json:"emotion_before"`
		EmotionAfter    *string `json:"emotion_after"`
		OutcomeNotes    *string `json:"outcome_notes"`
		Lesson          *string `json:"lesson"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	quality := strings.ToLower(strings.TrimSpace(req.ProcessQuality))
	if quality != "followed" && quality != "adjusted" && quality != "deviated" {
		Fail(c, http.StatusBadRequest, "process_quality must be followed, adjusted, or deviated")
		return
	}

	var trade models.Trade
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, models.DefaultUserID).First(&trade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, "trade not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade could not be loaded")
		return
	}
	var firstFill models.TradeJournalEntry
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND trade_id = ?", models.DefaultUserID, trade.ID).
		Order("executed_at asc, created_at asc").
		First(&firstFill).Error; err != nil {
		Fail(c, http.StatusBadRequest, "trade has no linked fills to review")
		return
	}

	var review models.TradeReview
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND journal_entry_id = ?", models.DefaultUserID, firstFill.ID).Order("version desc").First(&review).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			created, createErr := appendTradeReviewVersion(tx, firstFill)
			if createErr != nil {
				return createErr
			}
			review = created
		} else if err != nil {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"trade_id":         trade.ID,
			"process_quality":  quality,
			"deviation_reason": cleanOptionalString(req.DeviationReason),
			"emotion_before":   cleanOptionalString(req.EmotionBefore),
			"emotion_after":    cleanOptionalString(req.EmotionAfter),
			"outcome_notes":    cleanOptionalString(req.OutcomeNotes),
			"lesson":           cleanOptionalString(req.Lesson),
			"completed_at":     now,
			"updated_at":       now,
		}
		if err := tx.Model(&review).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", review.ID).First(&review).Error
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be saved")
		return
	}
	_, _ = services.RebuildUserTrades(c.Request.Context(), h.db, models.DefaultUserID)
	enriched, loadErr := h.loadEnrichedTrades(c, "", "")
	if loadErr == nil {
		for _, item := range enriched {
			if item.ID == trade.ID {
				OK(c, gin.H{"trade": item, "review": review})
				return
			}
		}
	}
	OK(c, gin.H{"trade": trade, "review": review})
}

func (h *TradeHandler) PositionSizer(c *gin.Context) {
	var req struct {
		Symbol                  string   `json:"symbol"`
		Direction               string   `json:"direction"`
		AccountEquity           *float64 `json:"account_equity"`
		AccountCurrency         string   `json:"account_currency"`
		StandardRiskPercent     *float64 `json:"standard_risk_percent"`
		MaxPositionPercent      *float64 `json:"max_position_percent"`
		MaxPortfolioHeatPercent *float64 `json:"max_portfolio_heat_percent"`
		RiskMultiple            *float64 `json:"risk_multiple"`
		EntryPrice              float64  `json:"entry_price"`
		StopLoss                float64  `json:"stop_loss"`
		TakeProfit              *float64 `json:"take_profit"`
		ATR14                   *float64 `json:"atr_14"`
		MarketRegime            string   `json:"market_regime"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	profile, _ := NewProfileHandler(h.db, nil).loadProfile(c)
	equity := 100000.0
	if req.AccountEquity != nil && *req.AccountEquity > 0 {
		equity = *req.AccountEquity
	} else if profile.AccountEquity != nil && *profile.AccountEquity > 0 {
		equity = *profile.AccountEquity
	}
	stdRiskPct := 1.0
	if req.StandardRiskPercent != nil && *req.StandardRiskPercent > 0 {
		stdRiskPct = *req.StandardRiskPercent
	} else if profile.StandardRiskPercent != nil && *profile.StandardRiskPercent > 0 {
		stdRiskPct = *profile.StandardRiskPercent
	} else if profile.MaxRiskPerTradePercent != nil && *profile.MaxRiskPerTradePercent > 0 {
		stdRiskPct = *profile.MaxRiskPerTradePercent
	}
	maxPosPct := 20.0
	if req.MaxPositionPercent != nil && *req.MaxPositionPercent > 0 {
		maxPosPct = *req.MaxPositionPercent
	} else if profile.MaxPortfolioConcentrationPercent != nil && *profile.MaxPortfolioConcentrationPercent > 0 {
		maxPosPct = *profile.MaxPortfolioConcentrationPercent
	}
	maxHeatPct := 5.0
	if req.MaxPortfolioHeatPercent != nil && *req.MaxPortfolioHeatPercent > 0 {
		maxHeatPct = *req.MaxPortfolioHeatPercent
	} else if profile.MaxPortfolioHeatPercent != nil && *profile.MaxPortfolioHeatPercent > 0 {
		maxHeatPct = *profile.MaxPortfolioHeatPercent
	}
	riskMult := 1.0
	if req.RiskMultiple != nil && *req.RiskMultiple > 0 {
		riskMult = *req.RiskMultiple
	}

	var openHeat float64
	if h.db != nil && h.db.Migrator().HasTable(&models.Trade{}) {
		var openTrades []models.Trade
		_ = h.db.WithContext(c.Request.Context()).
			Where("user_id = ? AND status = ?", models.DefaultUserID, models.TradeStatusOpen).
			Find(&openTrades).Error
		for _, tr := range openTrades {
			openHeat += tr.OpenRiskAmount
		}
	}

	regime := strings.TrimSpace(req.MarketRegime)
	if regime == "" && h.regimes != nil {
		assetMkt := "US"
		if inferTradeMarket(req.Symbol) == models.TradeMarketChinaA {
			assetMkt = "CN"
		}
		regime = h.regimes.Get(c.Request.Context(), assetMkt).Regime
	}

	result, err := services.CalculatePositionSize(services.PositionSizeInput{
		Symbol:                  req.Symbol,
		Direction:               req.Direction,
		AccountEquity:           equity,
		AccountCurrency:         req.AccountCurrency,
		StandardRiskPercent:     stdRiskPct,
		MaxPositionPercent:      maxPosPct,
		MaxPortfolioHeatPercent: maxHeatPct,
		CurrentOpenHeatAmount:   openHeat,
		RiskMultiple:            riskMult,
		EntryPrice:              req.EntryPrice,
		StopLoss:                req.StopLoss,
		TakeProfit:              req.TakeProfit,
		ATR14:                   req.ATR14,
		MarketRegime:            regime,
	})
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, result)
}

func (h *TradeHandler) loadEnrichedTrades(c *gin.Context, statusFilter, marketFilter string) ([]enrichedTradeResponse, error) {
	query := h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID).Order("opened_at desc")
	if statusFilter != "" && statusFilter != "all" {
		query = query.Where("status = ?", statusFilter)
	}
	var trades []models.Trade
	if err := query.Limit(500).Find(&trades).Error; err != nil {
		return nil, err
	}
	filtered := make([]models.Trade, 0, len(trades))
	for _, tr := range trades {
		if marketFilter == "US" && tr.Market != models.TradeMarketUS {
			continue
		}
		if marketFilter == "CN" && tr.Market != models.TradeMarketChinaA {
			continue
		}
		filtered = append(filtered, tr)
	}
	return h.enrichTrades(c, filtered)
}

func (h *TradeHandler) enrichTrades(c *gin.Context, trades []models.Trade) ([]enrichedTradeResponse, error) {
	result := make([]enrichedTradeResponse, 0, len(trades))
	if len(trades) == 0 {
		return result, nil
	}
	tradeIDs := make([]uuid.UUID, 0, len(trades))
	planIDs := make([]uuid.UUID, 0, len(trades))
	for _, tr := range trades {
		tradeIDs = append(tradeIDs, tr.ID)
		if tr.TradePlanID != nil {
			planIDs = append(planIDs, *tr.TradePlanID)
		}
	}

	var fills []models.TradeJournalEntry
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ? AND trade_id IN ?", models.DefaultUserID, tradeIDs).
		Order("executed_at asc").
		Find(&fills).Error; err != nil {
		return nil, err
	}
	fillsByTrade := make(map[uuid.UUID][]journalEntryResponse)
	for _, fill := range fills {
		if fill.TradeID != nil {
			fillsByTrade[*fill.TradeID] = append(fillsByTrade[*fill.TradeID], newJournalEntryResponse(fill))
		}
	}

	planByID := make(map[uuid.UUID]tradePlanResponse)
	if len(planIDs) > 0 {
		var plans []models.TradePlan
		if err := h.db.WithContext(c.Request.Context()).Where("user_id = ? AND id IN ?", models.DefaultUserID, planIDs).Find(&plans).Error; err != nil {
			return nil, err
		}
		for _, plan := range plans {
			planByID[plan.ID] = newTradePlanResponse(plan)
		}
	}

	reviewByTrade := make(map[uuid.UUID]models.TradeReview)
	if h.db.Migrator().HasTable(&models.TradeReview{}) {
		var reviews []models.TradeReview
		if err := h.db.WithContext(c.Request.Context()).
			Where("user_id = ? AND trade_id IN ?", models.DefaultUserID, tradeIDs).
			Order("version desc").
			Find(&reviews).Error; err != nil {
			return nil, err
		}
		for _, rev := range reviews {
			if rev.TradeID != nil {
				if _, exists := reviewByTrade[*rev.TradeID]; !exists {
					reviewByTrade[*rev.TradeID] = rev
				}
			}
		}
	}

	for _, tr := range trades {
		item := enrichedTradeResponse{
			Trade: tr,
			Fills: fillsByTrade[tr.ID],
		}
		if item.Fills == nil {
			item.Fills = []journalEntryResponse{}
		}
		if tr.TradePlanID != nil {
			if plan, ok := planByID[*tr.TradePlanID]; ok {
				copyPlan := plan
				item.TradePlan = &copyPlan
			}
		}
		if rev, ok := reviewByTrade[tr.ID]; ok {
			copyRev := rev
			item.TradeReview = &copyRev
		}
		result = append(result, item)
	}
	return result, nil
}

func (h *TradeHandler) computeSummary(c *gin.Context, marketFilter string) (services.TradesSummary, error) {
	var trades []models.Trade
	if err := h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID).Find(&trades).Error; err != nil {
		return services.TradesSummary{}, err
	}
	var fills []models.TradeJournalEntry
	if err := h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID).Find(&fills).Error; err != nil {
		return services.TradesSummary{}, err
	}
	if marketFilter == "US" || marketFilter == "CN" {
		filteredTrades := make([]models.Trade, 0, len(trades))
		for _, tr := range trades {
			if todayTradeMarketMatches(marketFilter, tr.Market) {
				filteredTrades = append(filteredTrades, tr)
			}
		}
		trades = filteredTrades
		filteredFills := make([]models.TradeJournalEntry, 0, len(fills))
		for _, fill := range fills {
			if todayTradeMarketMatches(marketFilter, fill.Market) {
				filteredFills = append(filteredFills, fill)
			}
		}
		fills = filteredFills
	}

	var reviews []models.TradeReview
	if h.db.Migrator().HasTable(&models.TradeReview{}) {
		_ = h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID).Order("version desc").Find(&reviews).Error
	}
	var pretrade []models.PreTradeRiskReview
	if h.db.Migrator().HasTable(&models.PreTradeRiskReview{}) {
		_ = h.db.WithContext(c.Request.Context()).Where("user_id = ?", models.DefaultUserID).Find(&pretrade).Error
	}
	profile, _ := NewProfileHandler(h.db, nil).loadProfile(c)
	return services.ComputeTradesSummary(trades, profile, fills, reviews, pretrade), nil
}
