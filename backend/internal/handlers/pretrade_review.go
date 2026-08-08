package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PreTradeReviewHandler struct {
	db          *gorm.DB
	reviewer    services.PreTradeAIReviewer
	credentials *services.LLMCredentialStore
}

type createPreTradeReviewRequest struct {
	RunAI           bool           `json:"run_ai"`
	LLMProvider     string         `json:"llm_provider"`
	Model           string         `json:"model"`
	BackendURL      string         `json:"backend_url"`
	CurrentEvidence map[string]any `json:"current_evidence"`
}

type acknowledgePreTradeFindingRequest struct {
	FindingCode string  `json:"finding_code"`
	Reason      *string `json:"reason"`
}

func NewPreTradeReviewHandler(database *gorm.DB, reviewer services.PreTradeAIReviewer, credentials *services.LLMCredentialStore) *PreTradeReviewHandler {
	return &PreTradeReviewHandler{db: database, reviewer: reviewer, credentials: credentials}
}

func (h *PreTradeReviewHandler) Create(c *gin.Context) {
	planID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "trade plan id is invalid")
		return
	}
	var request createPreTradeReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	request.LLMProvider = strings.ToLower(strings.TrimSpace(request.LLMProvider))
	request.Model = strings.TrimSpace(request.Model)
	request.BackendURL = strings.TrimSpace(request.BackendURL)
	if request.RunAI && (request.LLMProvider == "" || request.Model == "") {
		Fail(c, http.StatusBadRequest, "llm_provider and model are required when run_ai is true")
		return
	}

	var plan models.TradePlan
	err = h.db.WithContext(c.Request.Context()).First(&plan, "id = ?", planID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "trade plan not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan could not be loaded")
		return
	}

	profile, _ := NewProfileHandler(h.db, nil).loadProfile(c)
	deterministic := services.BuildPersonalizedPreTradeReview(plan, profile)
	review := models.PreTradeRiskReview{
		TradePlanID: plan.ID, PlanSnapshot: preTradePlanSnapshot(plan),
		HardConflicts: deterministic.HardConflicts, RiskWarnings: deterministic.RiskWarnings,
		BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: deterministic.UnresolvedQuestions,
		ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{},
		AIStatus: models.PreTradeAIStatusNotRequested,
	}
	if request.RunAI {
		review.AIProvider = stringPointer(request.LLMProvider)
		review.AIModel = stringPointer(request.Model)
		if h.reviewer == nil {
			review.AIStatus = models.PreTradeAIStatusUnavailable
		} else {
			apiKey := ""
			if h.credentials != nil {
				apiKey, err = h.credentials.Resolve(c.Request.Context(), request.LLMProvider, request.BackendURL)
				if err != nil {
					Fail(c, http.StatusInternalServerError, "stored LLM credential could not be loaded")
					return
				}
			}
			behaviorEvidence, principles, playbooks, profileContext, behaviorErr := h.loadEligiblePersonalMemory(c, plan)
			if behaviorErr != nil {
				review.UnresolvedQuestions = append(review.UnresolvedQuestions, models.PreTradeFinding{
					Code: "behavior_evidence_unavailable", Severity: "low", Category: "unknown", Message: "历史行为证据暂时无法读取。", Evidence: map[string]any{"reason": behaviorErr.Error()},
				})
			}
			aiResult, reviewErr := h.reviewer.Review(c.Request.Context(), services.PreTradeAIReviewInput{
				Ticker: plan.Ticker, Provider: request.LLMProvider, Model: request.Model, BackendURL: request.BackendURL, APIKey: apiKey,
				PlanSnapshot: review.PlanSnapshot, ResearchEvidence: h.loadResearchEvidence(c, plan), CurrentEvidence: request.CurrentEvidence, BehaviorObservations: behaviorEvidence,
				TradingProfile: profileContext, TradingPrinciples: principles, TradingPlaybooks: playbooks,
			})
			if reviewErr != nil {
				review.AIStatus = models.PreTradeAIStatusUnavailable
			} else {
				review.AIStatus = models.PreTradeAIStatusCompleted
				review.RiskWarnings = append(review.RiskWarnings, aiResult.RiskWarnings...)
				review.BehaviorReminders = aiResult.BehaviorReminders
				review.UnresolvedQuestions = append(review.UnresolvedQuestions, aiResult.UnresolvedQuestions...)
				review.ChangesSinceResearch = aiResult.ChangesSinceResearch
				review.Freshness = aiResult.Freshness
				review.Sources = aiResult.Sources
			}
		}
	}

	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var latest int
		if err := tx.Model(&models.PreTradeRiskReview{}).Where("trade_plan_id = ?", plan.ID).Select("COALESCE(MAX(version), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		review.Version = latest + 1
		return tx.Create(&review).Error
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "pre-trade risk review could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: review, Error: nil})
}

func (h *PreTradeReviewHandler) loadEligiblePersonalMemory(c *gin.Context, plan models.TradePlan) ([]map[string]any, []map[string]any, []map[string]any, map[string]any, error) {
	profileHandler := NewProfileHandler(h.db, nil)
	preference, err := profileHandler.loadMemory(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	profile, err := profileHandler.loadProfile(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	profileContext := map[string]any{"configured": profile.Configured, "primary_markets": profile.PrimaryMarkets, "trading_horizons": profile.TradingHorizons, "max_risk_per_trade_percent": profile.MaxRiskPerTradePercent, "max_daily_loss_percent": profile.MaxDailyLossPercent, "max_open_positions": profile.MaxOpenPositions}
	behavior := []map[string]any{}
	if preference.ObservationsEnabled {
		behavior, err = h.loadEligibleBehaviorEvidence()
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	principles := []map[string]any{}
	if preference.PrinciplesEnabled {
		var stored []models.TradingPrinciple
		if err = h.db.WithContext(c.Request.Context()).Where("enabled = ? AND ai_enabled = ?", true, true).Order("sort_order asc").Find(&stored).Error; err != nil {
			return nil, nil, nil, nil, err
		}
		for _, item := range stored {
			principles = append(principles, map[string]any{"id": item.ID.String(), "category": item.Category, "content": item.Content, "source_type": "trading_principle"})
		}
	}
	playbooks := []map[string]any{}
	if preference.PlaybooksEnabled {
		var stored []models.TradingPlaybook
		if err = h.db.WithContext(c.Request.Context()).Where("status = ? AND ai_enabled = ?", models.PlaybookStatusActive, true).Find(&stored).Error; err != nil {
			return nil, nil, nil, nil, err
		}
		market := inferTradeMarket(plan.Ticker)
		marketCode := "US"
		if market == models.TradeMarketChinaA {
			marketCode = "CN"
		}
		for _, item := range stored {
			if len(item.Markets) > 0 && !containsString(item.Markets, marketCode) {
				continue
			}
			playbooks = append(playbooks, map[string]any{"id": item.ID.String(), "name": item.Name, "markets": item.Markets, "horizons": item.Horizons, "market_context": item.MarketContext, "entry_rules": item.EntryRules, "invalidation_rules": item.InvalidationRules, "stop_rules": item.StopRules, "target_rules": item.TargetRules, "position_rules": item.PositionRules, "no_trade_rules": item.NoTradeRules, "evidence_review_ids": item.EvidenceReviewIDs, "source_type": "trading_playbook"})
		}
	}
	return behavior, principles, playbooks, profileContext, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (h *PreTradeReviewHandler) loadEligibleBehaviorEvidence() ([]map[string]any, error) {
	observations, err := syncBehaviorObservations(h.db)
	if err != nil {
		return []map[string]any{}, err
	}
	result := []map[string]any{}
	for _, observation := range observations {
		if observation.Status != "eligible" || (observation.AIEnabled != nil && !*observation.AIEnabled) || (observation.Feedback != nil && *observation.Feedback == "not_applicable") {
			continue
		}
		result = append(result, map[string]any{
			"id": observation.ID.String(), "key": observation.Key, "scenario": observation.Scenario, "status": observation.Status,
			"sample_count": observation.SampleCount, "confidence": observation.Confidence, "message": observation.Message,
			"evidence_review_ids": observation.EvidenceReviewIDs, "feedback": observation.Feedback,
			"user_correction": observation.UserCorrection, "source_type": "behavior_observation",
		})
	}
	return result, nil
}

func (h *PreTradeReviewHandler) List(c *gin.Context) {
	planID, ok := parsePreTradePlanID(c)
	if !ok {
		return
	}
	var reviews []models.PreTradeRiskReview
	if err := h.db.WithContext(c.Request.Context()).Where("trade_plan_id = ?", planID).Order("version DESC").Find(&reviews).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "pre-trade risk reviews could not be loaded")
		return
	}
	for index := range reviews {
		if err := h.db.WithContext(c.Request.Context()).Where("review_id = ?", reviews[index].ID).Order("created_at ASC").Find(&reviews[index].Acknowledgements).Error; err != nil {
			Fail(c, http.StatusInternalServerError, "risk acknowledgements could not be loaded")
			return
		}
	}
	OK(c, reviews)
}

func (h *PreTradeReviewHandler) Latest(c *gin.Context) {
	planID, ok := parsePreTradePlanID(c)
	if !ok {
		return
	}
	var review models.PreTradeRiskReview
	err := h.db.WithContext(c.Request.Context()).Where("trade_plan_id = ?", planID).Order("version DESC").First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "pre-trade risk review not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "pre-trade risk review could not be loaded")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Where("review_id = ?", review.ID).Order("created_at ASC").Find(&review.Acknowledgements).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "risk acknowledgements could not be loaded")
		return
	}
	OK(c, review)
}

func (h *PreTradeReviewHandler) Acknowledge(c *gin.Context) {
	planID, ok := parsePreTradePlanID(c)
	if !ok {
		return
	}
	reviewID, err := uuid.Parse(strings.TrimSpace(c.Param("reviewId")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "pre-trade risk review id is invalid")
		return
	}
	var request acknowledgePreTradeFindingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	request.FindingCode = strings.TrimSpace(request.FindingCode)
	if request.FindingCode == "" {
		Fail(c, http.StatusBadRequest, "finding_code is required")
		return
	}

	var review models.PreTradeRiskReview
	err = h.db.WithContext(c.Request.Context()).Where("id = ? AND trade_plan_id = ?", reviewID, planID).First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "pre-trade risk review not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "pre-trade risk review could not be loaded")
		return
	}
	if !containsHighRiskFinding(review, request.FindingCode) {
		Fail(c, http.StatusBadRequest, "finding_code does not identify a high-risk finding in this review")
		return
	}
	acknowledgement := models.RiskAcknowledgement{ReviewID: review.ID, FindingCode: request.FindingCode, Reason: cleanOptionalString(request.Reason)}
	if err := h.db.WithContext(c.Request.Context()).Create(&acknowledgement).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "risk acknowledgement could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: acknowledgement, Error: nil})
}

func parsePreTradePlanID(c *gin.Context) (uuid.UUID, bool) {
	planID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "trade plan id is invalid")
		return uuid.Nil, false
	}
	return planID, true
}

func containsHighRiskFinding(review models.PreTradeRiskReview, code string) bool {
	groups := [][]models.PreTradeFinding{review.HardConflicts, review.RiskWarnings, review.BehaviorReminders, review.UnresolvedQuestions, review.ChangesSinceResearch}
	for _, findings := range groups {
		for _, finding := range findings {
			if finding.Code == code && finding.Severity == "high" {
				return true
			}
		}
	}
	return false
}

func RegisterPreTradeReviewRoutes(router gin.IRoutes, handler *PreTradeReviewHandler) {
	router.POST("/trade-plans/:id/risk-reviews", handler.Create)
	router.GET("/trade-plans/:id/risk-reviews", handler.List)
	router.GET("/trade-plans/:id/risk-reviews/latest", handler.Latest)
	router.POST("/trade-plans/:id/risk-reviews/:reviewId/acknowledgements", handler.Acknowledge)
}

func (h *PreTradeReviewHandler) loadResearchEvidence(c *gin.Context, plan models.TradePlan) map[string]any {
	if plan.UserThesisID == nil {
		return map[string]any{"status": "unknown", "reason": "trade plan has no user thesis"}
	}
	var thesis models.UserThesis
	if err := h.db.WithContext(c.Request.Context()).First(&thesis, "id = ?", *plan.UserThesisID).Error; err != nil {
		return map[string]any{"status": "unknown", "reason": "user thesis could not be loaded"}
	}
	var research models.ResearchCase
	if err := h.db.WithContext(c.Request.Context()).First(&research, "id = ?", thesis.ResearchCaseID).Error; err != nil {
		return map[string]any{"status": "unknown", "reason": "research case could not be loaded"}
	}
	return map[string]any{"status": "available", "research_case_id": research.ID.String(), "evidence_snapshot": research.EvidenceSnapshot, "thesis_id": thesis.ID.String(), "thesis_version": thesis.Version}
}

func preTradePlanSnapshot(plan models.TradePlan) map[string]any {
	return map[string]any{
		"id": plan.ID.String(), "ticker": plan.Ticker, "direction": plan.Direction, "status": plan.Status,
		"plan_date": plan.PlanDate.Format(journalDateLayout), "valid_until": formatDatePointer(plan.ValidUntil),
		"user_thesis_id": formatUUIDPointer(plan.UserThesisID), "entry_trigger": plan.EntryTrigger, "entry_price": plan.EntryPrice,
		"stop_loss": plan.StopLoss, "take_profit": plan.TakeProfit, "position_size": plan.PositionSize,
		"invalidation": plan.Invalidation, "max_risk_amount": plan.MaxRiskAmount, "max_risk_percent": plan.MaxRiskPercent,
		"rationale": plan.Rationale, "notes": plan.Notes, "updated_at": plan.UpdatedAt,
	}
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
