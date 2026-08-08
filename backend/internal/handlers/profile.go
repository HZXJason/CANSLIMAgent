package handlers

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PlaybookSuggestionRequest struct {
	LLMProvider string `json:"llm_provider"`
	Model       string `json:"model"`
	BackendURL  string `json:"backend_url"`
}

type PlaybookSuggester interface {
	Suggest(context.Context, services.PlaybookSuggestionInput) ([]services.PlaybookSuggestion, error)
}

type ProfileHandler struct {
	db          *gorm.DB
	suggester   PlaybookSuggester
	credentials *services.LLMCredentialStore
	now         func() time.Time
}

func (h *ProfileHandler) WithCredentialStore(store *services.LLMCredentialStore) *ProfileHandler {
	h.credentials = store
	return h
}

func NewProfileHandler(database *gorm.DB, suggester PlaybookSuggester) *ProfileHandler {
	return &ProfileHandler{db: database, suggester: suggester, now: time.Now}
}

func RegisterProfileRoutes(router gin.IRoutes, handler *ProfileHandler) {
	router.GET("/profile", handler.Get)
	router.PUT("/profile", handler.Put)
	router.GET("/profile/principles", handler.ListPrinciples)
	router.POST("/profile/principles", handler.CreatePrinciple)
	router.PATCH("/profile/principles/:id", handler.PatchPrinciple)
	router.GET("/profile/playbooks", handler.ListPlaybooks)
	router.POST("/profile/playbooks", handler.CreatePlaybook)
	router.PATCH("/profile/playbooks/:id", handler.PatchPlaybook)
	router.POST("/profile/playbook-suggestions", handler.SuggestPlaybooks)
	router.GET("/profile/dashboard", handler.Dashboard)
	router.GET("/profile/ai-memory", handler.GetAIMemory)
	router.PATCH("/profile/ai-memory", handler.PatchAIMemory)
	router.PATCH("/profile/behavior-observations/:id", handler.PatchObservation)
	router.GET("/profile/export", handler.Export)
}

func defaultProfile() models.TradingProfile {
	return models.TradingProfile{ID: 1, Configured: false, PrimaryMarkets: []string{}, TradingHorizons: []string{}}
}

func defaultMemoryPreference() models.AIMemoryPreference {
	return models.AIMemoryPreference{ID: 1, PrinciplesEnabled: true, ObservationsEnabled: true, PlaybooksEnabled: true}
}

func (h *ProfileHandler) loadProfile(context *gin.Context) (models.TradingProfile, error) {
	var profile models.TradingProfile
	err := h.db.WithContext(context.Request.Context()).First(&profile, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultProfile(), nil
	}
	return profile, err
}

func (h *ProfileHandler) loadMemory(context *gin.Context) (models.AIMemoryPreference, error) {
	var preference models.AIMemoryPreference
	err := h.db.WithContext(context.Request.Context()).First(&preference, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultMemoryPreference(), nil
	}
	return preference, err
}

func (h *ProfileHandler) Get(c *gin.Context) {
	profile, err := h.loadProfile(c)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "个人交易系统读取失败")
		return
	}
	OK(c, profile)
}

type profileRequest struct {
	PrimaryMarkets         []string `json:"primary_markets"`
	TradingHorizons        []string `json:"trading_horizons"`
	MaxRiskPerTradePercent *float64 `json:"max_risk_per_trade_percent"`
	MaxDailyLossPercent    *float64 `json:"max_daily_loss_percent"`
	MaxOpenPositions       *int     `json:"max_open_positions"`
}

func (h *ProfileHandler) Put(c *gin.Context) {
	var request profileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	request.PrimaryMarkets = cleanUpperList(request.PrimaryMarkets)
	request.TradingHorizons = cleanLowerList(request.TradingHorizons)
	for _, market := range request.PrimaryMarkets {
		if market != "US" && market != "CN" {
			Fail(c, http.StatusBadRequest, "主要市场只能是 US 或 CN")
			return
		}
	}
	for _, horizon := range request.TradingHorizons {
		if horizon != "intraday" && horizon != "swing" && horizon != "position" {
			Fail(c, http.StatusBadRequest, "交易周期不受支持")
			return
		}
	}
	if invalidPercent(request.MaxRiskPerTradePercent) || invalidPercent(request.MaxDailyLossPercent) {
		Fail(c, http.StatusBadRequest, "风险比例必须大于 0 且不超过 100")
		return
	}
	if request.MaxOpenPositions != nil && (*request.MaxOpenPositions < 1 || *request.MaxOpenPositions > 100) {
		Fail(c, http.StatusBadRequest, "最大持仓数必须在 1 到 100 之间")
		return
	}
	profile := models.TradingProfile{ID: 1, Configured: true, PrimaryMarkets: request.PrimaryMarkets, TradingHorizons: request.TradingHorizons, MaxRiskPerTradePercent: request.MaxRiskPerTradePercent, MaxDailyLossPercent: request.MaxDailyLossPercent, MaxOpenPositions: request.MaxOpenPositions}
	if err := h.db.WithContext(c.Request.Context()).Save(&profile).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "个人交易系统保存失败")
		return
	}
	OK(c, profile)
}

type principleRequest struct {
	Category  *string `json:"category"`
	Content   *string `json:"content"`
	Enabled   *bool   `json:"enabled"`
	AIEnabled *bool   `json:"ai_enabled"`
	SortOrder *int    `json:"sort_order"`
}

func (h *ProfileHandler) ListPrinciples(c *gin.Context) {
	var principles []models.TradingPrinciple
	if err := h.db.WithContext(c.Request.Context()).Order("category asc, sort_order asc, created_at asc").Find(&principles).Error; err != nil {
		Fail(c, 500, "交易纪律读取失败")
		return
	}
	OK(c, principles)
}

func (h *ProfileHandler) CreatePrinciple(c *gin.Context) {
	var request principleRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Category == nil || request.Content == nil {
		Fail(c, 400, "类别和内容不能为空")
		return
	}
	category, content := strings.ToLower(strings.TrimSpace(*request.Category)), strings.TrimSpace(*request.Content)
	if !models.ValidPrincipleCategory(category) || content == "" {
		Fail(c, 400, "交易纪律内容或类别不正确")
		return
	}
	principle := models.TradingPrinciple{Category: category, Content: content, Enabled: true, AIEnabled: true}
	if request.Enabled != nil {
		principle.Enabled = *request.Enabled
	}
	if request.AIEnabled != nil {
		principle.AIEnabled = *request.AIEnabled
	}
	if request.SortOrder != nil {
		principle.SortOrder = *request.SortOrder
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&principle).Error; err != nil {
		Fail(c, 500, "交易纪律保存失败")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: principle})
}

func (h *ProfileHandler) PatchPrinciple(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "交易纪律")
	if !ok {
		return
	}
	var principle models.TradingPrinciple
	if !loadByID(c, h.db, &principle, id, "交易纪律") {
		return
	}
	var request principleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求体格式不正确")
		return
	}
	updates := map[string]any{"updated_at": h.now().UTC()}
	if request.Category != nil {
		value := strings.ToLower(strings.TrimSpace(*request.Category))
		if !models.ValidPrincipleCategory(value) {
			Fail(c, 400, "交易纪律类别不正确")
			return
		}
		updates["category"] = value
	}
	if request.Content != nil {
		value := strings.TrimSpace(*request.Content)
		if value == "" {
			Fail(c, 400, "交易纪律内容不能为空")
			return
		}
		updates["content"] = value
	}
	if request.Enabled != nil {
		updates["enabled"] = *request.Enabled
	}
	if request.AIEnabled != nil {
		updates["ai_enabled"] = *request.AIEnabled
	}
	if request.SortOrder != nil {
		updates["sort_order"] = *request.SortOrder
	}
	h.db.WithContext(c.Request.Context()).Model(&principle).Updates(updates)
	h.db.First(&principle, "id = ?", id)
	OK(c, principle)
}

type playbookRequest struct {
	Name              *string   `json:"name"`
	Markets           *[]string `json:"markets"`
	Horizons          *[]string `json:"horizons"`
	MarketContext     *string   `json:"market_context"`
	EntryRules        *string   `json:"entry_rules"`
	InvalidationRules *string   `json:"invalidation_rules"`
	StopRules         *string   `json:"stop_rules"`
	TargetRules       *string   `json:"target_rules"`
	PositionRules     *string   `json:"position_rules"`
	NoTradeRules      *string   `json:"no_trade_rules"`
	Notes             *string   `json:"notes"`
	Status            *string   `json:"status"`
	AIEnabled         *bool     `json:"ai_enabled"`
	Source            *string   `json:"source"`
	EvidenceReviewIDs *[]string `json:"evidence_review_ids"`
}

func (h *ProfileHandler) ListPlaybooks(c *gin.Context) {
	var playbooks []models.TradingPlaybook
	query := h.db.WithContext(c.Request.Context()).Order("case status when 'active' then 0 when 'draft' then 1 else 2 end, updated_at desc")
	if c.Query("include_archived") != "true" {
		query = query.Where("status <> ?", models.PlaybookStatusArchived)
	}
	if err := query.Find(&playbooks).Error; err != nil {
		Fail(c, 500, "策略剧本读取失败")
		return
	}
	OK(c, playbooks)
}

func (h *ProfileHandler) CreatePlaybook(c *gin.Context) {
	var request playbookRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Name == nil {
		Fail(c, 400, "剧本名称不能为空")
		return
	}
	name := strings.TrimSpace(*request.Name)
	if name == "" {
		Fail(c, 400, "剧本名称不能为空")
		return
	}
	playbook := models.TradingPlaybook{Name: name, Markets: []string{}, Horizons: []string{}, Status: models.PlaybookStatusDraft, Source: "user", EvidenceReviewIDs: []string{}, AIEnabled: true}
	applyPlaybookRequest(&playbook, request)
	if !models.ValidPlaybookStatus(playbook.Status) {
		Fail(c, 400, "策略剧本状态不正确")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&playbook).Error; err != nil {
		Fail(c, 500, "策略剧本保存失败")
		return
	}
	c.JSON(201, APIResponse{Success: true, Data: playbook})
}

func (h *ProfileHandler) PatchPlaybook(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "策略剧本")
	if !ok {
		return
	}
	var playbook models.TradingPlaybook
	if !loadByID(c, h.db, &playbook, id, "策略剧本") {
		return
	}
	var request playbookRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求体格式不正确")
		return
	}
	applyPlaybookRequest(&playbook, request)
	if strings.TrimSpace(playbook.Name) == "" || !models.ValidPlaybookStatus(playbook.Status) {
		Fail(c, 400, "策略剧本名称或状态不正确")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&playbook).Error; err != nil {
		Fail(c, 500, "策略剧本保存失败")
		return
	}
	OK(c, playbook)
}

func applyPlaybookRequest(playbook *models.TradingPlaybook, request playbookRequest) {
	if request.Name != nil {
		playbook.Name = strings.TrimSpace(*request.Name)
	}
	if request.Markets != nil {
		playbook.Markets = cleanUpperList(*request.Markets)
	}
	if request.Horizons != nil {
		playbook.Horizons = cleanLowerList(*request.Horizons)
	}
	for target, value := range map[**string]*string{&playbook.MarketContext: request.MarketContext, &playbook.EntryRules: request.EntryRules, &playbook.InvalidationRules: request.InvalidationRules, &playbook.StopRules: request.StopRules, &playbook.TargetRules: request.TargetRules, &playbook.PositionRules: request.PositionRules, &playbook.NoTradeRules: request.NoTradeRules, &playbook.Notes: request.Notes} {
		if value != nil {
			*target = cleanOptionalString(value)
		}
	}
	if request.Status != nil {
		playbook.Status = strings.ToLower(strings.TrimSpace(*request.Status))
	}
	if request.AIEnabled != nil {
		playbook.AIEnabled = *request.AIEnabled
	}
	if request.Source != nil {
		playbook.Source = strings.TrimSpace(*request.Source)
	}
	if request.EvidenceReviewIDs != nil {
		playbook.EvidenceReviewIDs = *request.EvidenceReviewIDs
		playbook.EvidenceCount = len(*request.EvidenceReviewIDs)
	}
}

func (h *ProfileHandler) GetAIMemory(c *gin.Context) {
	preference, err := h.loadMemory(c)
	if err != nil {
		Fail(c, 500, "AI 记忆设置读取失败")
		return
	}
	OK(c, preference)
}

func (h *ProfileHandler) PatchAIMemory(c *gin.Context) {
	var request struct {
		PrinciplesEnabled   *bool `json:"principles_enabled"`
		ObservationsEnabled *bool `json:"observations_enabled"`
		PlaybooksEnabled    *bool `json:"playbooks_enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求体格式不正确")
		return
	}
	preference, err := h.loadMemory(c)
	if err != nil {
		Fail(c, 500, "AI 记忆设置读取失败")
		return
	}
	if request.PrinciplesEnabled != nil {
		preference.PrinciplesEnabled = *request.PrinciplesEnabled
	}
	if request.ObservationsEnabled != nil {
		preference.ObservationsEnabled = *request.ObservationsEnabled
	}
	if request.PlaybooksEnabled != nil {
		preference.PlaybooksEnabled = *request.PlaybooksEnabled
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&preference).Error; err != nil {
		Fail(c, 500, "AI 记忆设置保存失败")
		return
	}
	OK(c, preference)
}

func (h *ProfileHandler) PatchObservation(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id", "行为观察")
	if !ok {
		return
	}
	var observation models.BehaviorObservation
	if !loadByID(c, h.db, &observation, id, "行为观察") {
		return
	}
	var request struct {
		UserCorrection *string `json:"user_correction"`
		AIEnabled      *bool   `json:"ai_enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, 400, "请求体格式不正确")
		return
	}
	updates := map[string]any{"updated_at": h.now().UTC()}
	if request.UserCorrection != nil {
		updates["user_correction"] = cleanOptionalString(request.UserCorrection)
	}
	if request.AIEnabled != nil {
		updates["ai_enabled"] = *request.AIEnabled
	}
	if err := h.db.WithContext(c.Request.Context()).Model(&observation).Updates(updates).Error; err != nil {
		Fail(c, 500, "行为观察更新失败")
		return
	}
	h.db.First(&observation, "id = ?", id)
	OK(c, observation)
}

type profileProcessMetrics struct {
	Executions           int     `json:"executions"`
	PlannedExecutions    int     `json:"planned_executions"`
	PlannedExecutionRate float64 `json:"planned_execution_rate"`
	PretradeReviewed     int     `json:"pretrade_reviewed"`
	PretradeReviewRate   float64 `json:"pretrade_review_rate"`
	CompletedReviews     int     `json:"completed_reviews"`
	ReviewCompletionRate float64 `json:"review_completion_rate"`
	FollowedReviews      int     `json:"followed_reviews"`
	PlanFollowRate       float64 `json:"plan_follow_rate"`
	DeviatedReviews      int     `json:"deviated_reviews"`
	OffPlanExecutions    int     `json:"off_plan_executions"`
}
type outcomeGroup struct {
	Market        string   `json:"market"`
	Currency      string   `json:"currency"`
	RealizedPnL   float64  `json:"realized_pnl"`
	MatchedTrades int      `json:"matched_trades"`
	WinningTrades int      `json:"winning_trades"`
	WinRate       *float64 `json:"win_rate"`
	OpenQuantity  float64  `json:"open_quantity"`
	CoverageRate  float64  `json:"coverage_rate"`
}
type profileTrendPoint struct {
	Date                 string  `json:"date"`
	Executions           int     `json:"executions"`
	PlannedExecutionRate float64 `json:"planned_execution_rate"`
	PretradeReviewRate   float64 `json:"pretrade_review_rate"`
	ReviewCompletionRate float64 `json:"review_completion_rate"`
	PlanFollowRate       float64 `json:"plan_follow_rate"`
	OffPlanExecutions    int     `json:"off_plan_executions"`
	DeviatedReviews      int     `json:"deviated_reviews"`
}

func (h *ProfileHandler) Dashboard(c *gin.Context) {
	rangeName, start, ok := profileRange(c.Query("range"), h.now().UTC())
	if !ok {
		Fail(c, 400, "range must be 7d, 30d, 90d, or all")
		return
	}
	query := h.db.WithContext(c.Request.Context()).Order("executed_at asc")
	if start != nil {
		query = query.Where("executed_at >= ?", *start)
	}
	var entries []models.TradeJournalEntry
	if err := query.Find(&entries).Error; err != nil {
		Fail(c, 500, "成长数据读取失败")
		return
	}
	metrics := h.processMetrics(entries)
	outcomes := buildOutcomeGroups(entries)
	trends := h.buildTrends(entries)
	from := (*time.Time)(nil)
	if len(entries) > 0 {
		value := entries[0].ExecutedAt
		from = &value
	}
	OK(c, gin.H{"range": rangeName, "from": from, "to": h.now().UTC(), "as_of": h.now().UTC(), "sample_size": len(entries), "process": metrics, "outcomes": outcomes, "trends": trends})
}

func (h *ProfileHandler) processMetrics(entries []models.TradeJournalEntry) profileProcessMetrics {
	metrics := profileProcessMetrics{Executions: len(entries)}
	linkedIDs := []uuid.UUID{}
	entryIDs := []uuid.UUID{}
	for _, entry := range entries {
		entryIDs = append(entryIDs, entry.ID)
		if entry.TradePlanID == nil {
			metrics.OffPlanExecutions++
		} else {
			metrics.PlannedExecutions++
			linkedIDs = append(linkedIDs, *entry.TradePlanID)
		}
	}
	if len(linkedIDs) > 0 {
		var reviews []models.PreTradeRiskReview
		h.db.Where("trade_plan_id IN ?", linkedIDs).Order("created_at asc").Find(&reviews)
		firstReviewAt := map[uuid.UUID]time.Time{}
		for _, review := range reviews {
			if _, exists := firstReviewAt[review.TradePlanID]; !exists {
				firstReviewAt[review.TradePlanID] = review.CreatedAt
			}
		}
		for _, entry := range entries {
			if entry.TradePlanID != nil {
				if createdAt, exists := firstReviewAt[*entry.TradePlanID]; exists && !createdAt.After(entry.ExecutedAt) {
					metrics.PretradeReviewed++
				}
			}
		}
	}
	if len(entryIDs) > 0 {
		var reviews []models.TradeReview
		h.db.Where("journal_entry_id IN ? AND completed_at IS NOT NULL", entryIDs).Order("journal_entry_id asc, version desc").Find(&reviews)
		seen := map[uuid.UUID]bool{}
		for _, review := range reviews {
			if seen[review.JournalEntryID] {
				continue
			}
			seen[review.JournalEntryID] = true
			metrics.CompletedReviews++
			if review.ProcessQuality == "followed" {
				metrics.FollowedReviews++
			}
			if review.ProcessQuality == "deviated" {
				metrics.DeviatedReviews++
			}
		}
	}
	metrics.PlannedExecutionRate = ratio(metrics.PlannedExecutions, metrics.Executions)
	metrics.PretradeReviewRate = ratio(metrics.PretradeReviewed, metrics.PlannedExecutions)
	metrics.ReviewCompletionRate = ratio(metrics.CompletedReviews, metrics.Executions)
	metrics.PlanFollowRate = ratio(metrics.FollowedReviews, metrics.CompletedReviews)
	return metrics
}

func (h *ProfileHandler) buildTrends(entries []models.TradeJournalEntry) []profileTrendPoint {
	groups := map[string][]models.TradeJournalEntry{}
	order := []string{}
	for _, entry := range entries {
		key := entry.ExecutedAt.UTC().Format("2006-01-02")
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], entry)
	}
	result := make([]profileTrendPoint, 0, len(order))
	for _, date := range order {
		metrics := h.processMetrics(groups[date])
		result = append(result, profileTrendPoint{Date: date, Executions: metrics.Executions, PlannedExecutionRate: metrics.PlannedExecutionRate, PretradeReviewRate: metrics.PretradeReviewRate, ReviewCompletionRate: metrics.ReviewCompletionRate, PlanFollowRate: metrics.PlanFollowRate, OffPlanExecutions: metrics.OffPlanExecutions, DeviatedReviews: metrics.DeviatedReviews})
	}
	return result
}

type fifoLot struct{ quantity, unitCost float64 }

func buildOutcomeGroups(entries []models.TradeJournalEntry) []outcomeGroup {
	type state struct {
		result                       outcomeGroup
		lots                         map[string][]fifoLot
		buyQuantity, matchedQuantity float64
	}
	states := map[string]*state{}
	for _, entry := range entries {
		key := entry.Market + "|" + entry.Currency
		current := states[key]
		if current == nil {
			current = &state{result: outcomeGroup{Market: entry.Market, Currency: entry.Currency}, lots: map[string][]fifoLot{}}
			states[key] = current
		}
		lots := current.lots[entry.Symbol]
		if entry.Side == models.TradeSideBuy {
			lots = append(lots, fifoLot{entry.Quantity, entry.Price + entry.Fee/max(entry.Quantity, 1)})
			current.buyQuantity += entry.Quantity
		} else {
			remaining, tradePnL := entry.Quantity, -entry.Fee
			for remaining > 0 && len(lots) > 0 {
				matched := min(remaining, lots[0].quantity)
				tradePnL += matched * (entry.Price - lots[0].unitCost)
				remaining -= matched
				lots[0].quantity -= matched
				current.matchedQuantity += matched
				if lots[0].quantity <= 0.00000001 {
					lots = lots[1:]
				}
			}
			if entry.Quantity > remaining {
				current.result.MatchedTrades++
				current.result.RealizedPnL += tradePnL
				if tradePnL > 0 {
					current.result.WinningTrades++
				}
			}
		}
		current.lots[entry.Symbol] = lots
	}
	result := make([]outcomeGroup, 0, len(states))
	for _, current := range states {
		for _, lots := range current.lots {
			for _, lot := range lots {
				current.result.OpenQuantity += lot.quantity
			}
		}
		if current.result.MatchedTrades > 0 {
			value := ratio(current.result.WinningTrades, current.result.MatchedTrades)
			current.result.WinRate = &value
		}
		current.result.CoverageRate = 0
		if current.buyQuantity > 0 {
			current.result.CoverageRate = current.matchedQuantity / current.buyQuantity
		}
		result = append(result, current.result)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Market+result[i].Currency < result[j].Market+result[j].Currency })
	return result
}

func (h *ProfileHandler) SuggestPlaybooks(c *gin.Context) {
	if h.suggester == nil {
		Fail(c, http.StatusServiceUnavailable, "策略剧本建议服务暂不可用")
		return
	}
	var request PlaybookSuggestionRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.LLMProvider) == "" || strings.TrimSpace(request.Model) == "" {
		Fail(c, 400, "模型服务和模型不能为空")
		return
	}
	var reviews []models.TradeReview
	h.db.Where("completed_at IS NOT NULL").Order("created_at desc").Limit(100).Find(&reviews)
	observations, _ := syncBehaviorObservations(h.db)
	apiKey := ""
	var err error
	if h.credentials != nil {
		apiKey, err = h.credentials.Resolve(c.Request.Context(), request.LLMProvider, request.BackendURL)
		if err != nil {
			Fail(c, 500, "模型凭据读取失败")
			return
		}
	}
	suggestions, err := h.suggester.Suggest(c.Request.Context(), services.PlaybookSuggestionInput{LLMProvider: request.LLMProvider, Model: request.Model, BackendURL: request.BackendURL, APIKey: apiKey, Reviews: reviews, Observations: observations})
	if err != nil {
		Fail(c, 502, "策略剧本建议失败: "+err.Error())
		return
	}
	OK(c, gin.H{"status": "completed", "suggestions": suggestions})
}

func (h *ProfileHandler) Export(c *gin.Context) {
	profile, _ := h.loadProfile(c)
	memory, _ := h.loadMemory(c)
	var principles []models.TradingPrinciple
	var playbooks []models.TradingPlaybook
	var observations []models.BehaviorObservation
	h.db.Order("sort_order asc").Find(&principles)
	h.db.Order("updated_at desc").Find(&playbooks)
	h.db.Order("key asc").Find(&observations)
	OK(c, gin.H{"schema_version": "alphalens.personal-trading-system.v1", "exported_at": h.now().UTC(), "profile": profile, "principles": principles, "playbooks": playbooks, "behavior_observations": observations, "ai_memory": memory})
}

func profileRange(value string, now time.Time) (string, *time.Time, bool) {
	if value == "" {
		value = "30d"
	}
	if value == "all" {
		return value, nil, true
	}
	days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || (days != 7 && days != 30 && days != 90) {
		return "", nil, false
	}
	start := now.AddDate(0, 0, -days)
	return value, &start, true
}
func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
func invalidPercent(value *float64) bool { return value != nil && (*value <= 0 || *value > 100) }
func cleanUpperList(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func cleanLowerList(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func parseUUIDParam(c *gin.Context, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param(name)))
	if err != nil {
		Fail(c, 400, label+" ID 不正确")
		return uuid.Nil, false
	}
	return id, true
}
func loadByID(c *gin.Context, db *gorm.DB, target any, id uuid.UUID, label string) bool {
	err := db.WithContext(c.Request.Context()).First(target, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, 404, label+"不存在")
		return false
	}
	if err != nil {
		Fail(c, 500, label+"读取失败")
		return false
	}
	return true
}
