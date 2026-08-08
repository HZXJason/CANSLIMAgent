package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TradeReviewHandler struct {
	db *gorm.DB
}

type linkJournalPlanRequest struct {
	TradePlanID string `json:"trade_plan_id"`
}

type updateTradeReviewRequest struct {
	ProcessQuality  *string `json:"process_quality"`
	DeviationReason *string `json:"deviation_reason"`
	EmotionBefore   *string `json:"emotion_before"`
	EmotionAfter    *string `json:"emotion_after"`
	OutcomeNotes    *string `json:"outcome_notes"`
	Lesson          *string `json:"lesson"`
	Complete        *bool   `json:"complete"`
}

type behaviorObservationFeedbackRequest struct {
	Feedback string `json:"feedback"`
}

func NewTradeReviewHandler(database *gorm.DB) *TradeReviewHandler {
	return &TradeReviewHandler{db: database}
}

func RegisterTradeReviewRoutes(router gin.IRoutes, handler *TradeReviewHandler) {
	router.POST("/journal/:id/review", handler.Create)
	router.GET("/journal/:id/review", handler.Get)
	router.PATCH("/journal/:id/review", handler.Patch)
	router.POST("/journal/:id/link-plan", handler.LinkPlan)
	router.GET("/journal/:id/plan-candidates", handler.PlanCandidates)
	router.GET("/behavior-observations", handler.ListBehaviorObservations)
	router.POST("/behavior-observations/:observationId/feedback", handler.UpdateBehaviorObservationFeedback)
}

func (h *TradeReviewHandler) ListBehaviorObservations(c *gin.Context) {
	observations, err := syncBehaviorObservations(h.db.WithContext(c.Request.Context()))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "behavior observations could not be generated")
		return
	}
	OK(c, observations)
}

func (h *TradeReviewHandler) UpdateBehaviorObservationFeedback(c *gin.Context) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("observationId")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "behavior observation id is invalid")
		return
	}
	var request behaviorObservationFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	request.Feedback = strings.ToLower(strings.TrimSpace(request.Feedback))
	if request.Feedback != "accepted" && request.Feedback != "not_applicable" && request.Feedback != "needs_more_evidence" {
		Fail(c, http.StatusBadRequest, "feedback must be accepted, not_applicable, or needs_more_evidence")
		return
	}
	var observation models.BehaviorObservation
	err = h.db.WithContext(c.Request.Context()).First(&observation, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "behavior observation not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "behavior observation could not be loaded")
		return
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(c.Request.Context()).Model(&observation).Updates(map[string]any{"feedback": request.Feedback, "feedback_at": now, "updated_at": now}).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "behavior observation feedback could not be saved")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).First(&observation, "id = ?", id).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "behavior observation could not be reloaded")
		return
	}
	OK(c, observation)
}

func syncBehaviorObservations(database *gorm.DB) ([]models.BehaviorObservation, error) {
	var all []models.TradeReview
	if err := database.Where("completed_at IS NOT NULL").Order("journal_entry_id ASC, version DESC").Find(&all).Error; err != nil {
		return nil, err
	}
	latest := make([]models.TradeReview, 0, len(all))
	seen := make(map[uuid.UUID]bool)
	for _, review := range all {
		if seen[review.JournalEntryID] {
			continue
		}
		seen[review.JournalEntryID] = true
		latest = append(latest, review)
	}
	built := services.BuildBehaviorObservations(latest)
	keys := make([]string, 0, len(built))
	for index := range built {
		keys = append(keys, built[index].Key)
		if err := database.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"scenario", "status", "sample_count", "confidence", "message", "evidence_review_ids", "updated_at"}),
		}).Create(&built[index]).Error; err != nil {
			return nil, err
		}
	}
	var stored []models.BehaviorObservation
	if err := database.Where("key IN ?", keys).Order("key ASC").Find(&stored).Error; err != nil {
		return nil, err
	}
	return stored, nil
}

func (h *TradeReviewHandler) Patch(c *gin.Context) {
	entryID, ok := parseJournalEntryID(c)
	if !ok {
		return
	}
	var request updateTradeReviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	var review models.TradeReview
	err := h.db.WithContext(c.Request.Context()).Where("journal_entry_id = ?", entryID).Order("version DESC").First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "trade review not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be loaded")
		return
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if request.ProcessQuality != nil {
		quality := strings.ToLower(strings.TrimSpace(*request.ProcessQuality))
		if !validProcessQuality(quality) {
			Fail(c, http.StatusBadRequest, "process_quality must be unreviewed, followed, adjusted, or deviated")
			return
		}
		updates["process_quality"] = quality
		review.ProcessQuality = quality
	}
	for field, value := range map[string]*string{
		"deviation_reason": request.DeviationReason, "emotion_before": request.EmotionBefore, "emotion_after": request.EmotionAfter,
		"outcome_notes": request.OutcomeNotes, "lesson": request.Lesson,
	} {
		if value != nil {
			updates[field] = cleanOptionalString(value)
		}
	}
	if request.Complete != nil {
		if *request.Complete {
			if review.ProcessQuality == "unreviewed" {
				Fail(c, http.StatusBadRequest, "process_quality is required before completing a trade review")
				return
			}
			updates["completed_at"] = time.Now().UTC()
		} else {
			updates["completed_at"] = nil
		}
	}
	if err := h.db.WithContext(c.Request.Context()).Model(&review).Updates(updates).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be updated")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).First(&review, "id = ?", review.ID).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be reloaded")
		return
	}
	OK(c, review)
}

func (h *TradeReviewHandler) Create(c *gin.Context) {
	entry, ok := h.loadEntry(c)
	if !ok {
		return
	}
	var review models.TradeReview
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		created, err := appendTradeReviewVersion(tx, entry)
		review = created
		return err
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: review, Error: nil})
}

func (h *TradeReviewHandler) Get(c *gin.Context) {
	entryID, ok := parseJournalEntryID(c)
	if !ok {
		return
	}
	var review models.TradeReview
	err := h.db.WithContext(c.Request.Context()).Where("journal_entry_id = ?", entryID).Order("version DESC").First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "trade review not found")
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "trade review could not be loaded")
		return
	}
	OK(c, review)
}

func (h *TradeReviewHandler) PlanCandidates(c *gin.Context) {
	entry, ok := h.loadEntry(c)
	if !ok {
		return
	}
	direction := models.TradePlanDirectionBuy
	if entry.Side == models.TradeSideSell {
		direction = models.TradePlanDirectionSell
	}
	var plans []models.TradePlan
	if err := h.db.WithContext(c.Request.Context()).
		Where("ticker = ? AND direction = ? AND status IN ?", entry.Symbol, direction, []string{models.TradePlanStatusDraft, models.TradePlanStatusWaiting}).
		Order("plan_date DESC").Limit(20).Find(&plans).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan candidates could not be loaded")
		return
	}
	responses := make([]tradePlanResponse, 0, len(plans))
	for _, plan := range plans {
		responses = append(responses, newTradePlanResponse(plan))
	}
	OK(c, responses)
}

func (h *TradeReviewHandler) LinkPlan(c *gin.Context) {
	entry, ok := h.loadEntry(c)
	if !ok {
		return
	}
	var request linkJournalPlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	planID, err := uuid.Parse(strings.TrimSpace(request.TradePlanID))
	if err != nil {
		Fail(c, http.StatusBadRequest, "trade_plan_id is invalid")
		return
	}
	if entry.TradePlanID != nil && *entry.TradePlanID != planID {
		Fail(c, http.StatusConflict, "journal entry is already linked to another trade plan")
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
	if !strings.EqualFold(plan.Ticker, entry.Symbol) || !planDirectionMatchesEntry(plan.Direction, entry.Side) {
		Fail(c, http.StatusBadRequest, "trade plan symbol or direction does not match journal entry")
		return
	}
	if plan.Status != models.TradePlanStatusDraft && plan.Status != models.TradePlanStatusWaiting && !(plan.Status == models.TradePlanStatusExecuted && plan.ExecutedJournalID != nil && *plan.ExecutedJournalID == entry.ID) {
		Fail(c, http.StatusBadRequest, "closed trade plan cannot be linked to this journal entry")
		return
	}

	var review models.TradeReview
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.TradeJournalEntry{}).Where("id = ? AND trade_plan_id IS NULL", entry.ID).Update("trade_plan_id", plan.ID).Error; err != nil {
			return err
		}
		entry.TradePlanID = &plan.ID
		if err := tx.Model(&models.TradePlan{}).Where("id = ?", plan.ID).Updates(map[string]any{
			"status": models.TradePlanStatusExecuted, "executed_journal_id": entry.ID, "executed_at": entry.ExecutedAt, "updated_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		created, err := appendTradeReviewVersion(tx, entry)
		review = created
		return err
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be linked to trade plan")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: review, Error: nil})
}

func appendTradeReviewVersion(tx *gorm.DB, entry models.TradeJournalEntry) (models.TradeReview, error) {
	var plan *models.TradePlan
	if entry.TradePlanID != nil {
		var loaded models.TradePlan
		if err := tx.First(&loaded, "id = ?", *entry.TradePlanID).Error; err != nil {
			return models.TradeReview{}, err
		}
		plan = &loaded
	}
	var latestVersion int
	if err := tx.Model(&models.TradeReview{}).Where("journal_entry_id = ?", entry.ID).Select("COALESCE(MAX(version), 0)").Scan(&latestVersion).Error; err != nil {
		return models.TradeReview{}, err
	}
	planSnapshot := map[string]any{}
	var planID *uuid.UUID
	if plan != nil {
		planSnapshot = preTradePlanSnapshot(*plan)
		id := plan.ID
		planID = &id
	}
	review := models.TradeReview{
		JournalEntryID: entry.ID, Version: latestVersion + 1, TradePlanID: planID,
		PlanSnapshot: planSnapshot, ExecutionSnapshot: tradeExecutionSnapshot(entry),
		Diff: services.BuildTradeExecutionDiff(plan, entry), ProcessQuality: "unreviewed",
	}
	return review, tx.Create(&review).Error
}

func tradeExecutionSnapshot(entry models.TradeJournalEntry) map[string]any {
	return map[string]any{
		"id": entry.ID.String(), "executed_at": entry.ExecutedAt, "symbol": entry.Symbol, "name": entry.Name,
		"side": entry.Side, "quantity": entry.Quantity, "price": entry.Price, "amount": entry.Amount, "fee": entry.Fee,
		"market": entry.Market, "currency": entry.Currency, "order_id": entry.OrderID, "source": entry.Source,
	}
}

func planDirectionMatchesEntry(direction, side string) bool {
	return (direction == models.TradePlanDirectionBuy && side == models.TradeSideBuy) || (direction == models.TradePlanDirectionSell && side == models.TradeSideSell)
}

func validProcessQuality(value string) bool {
	return value == "unreviewed" || value == "followed" || value == "adjusted" || value == "deviated"
}

func (h *TradeReviewHandler) loadEntry(c *gin.Context) (models.TradeJournalEntry, bool) {
	entryID, ok := parseJournalEntryID(c)
	if !ok {
		return models.TradeJournalEntry{}, false
	}
	var entry models.TradeJournalEntry
	err := h.db.WithContext(c.Request.Context()).First(&entry, "id = ?", entryID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "journal entry not found")
		return entry, false
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be loaded")
		return entry, false
	}
	return entry, true
}

func parseJournalEntryID(c *gin.Context) (uuid.UUID, bool) {
	entryID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		Fail(c, http.StatusBadRequest, "journal entry id is invalid")
		return uuid.Nil, false
	}
	return entryID, true
}
