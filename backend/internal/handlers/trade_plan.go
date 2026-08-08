package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var lockedTradePlanFields = map[string]struct{}{
	"ticker":              {},
	"direction":           {},
	"plan_date":           {},
	"valid_until":         {},
	"user_thesis_id":      {},
	"entry_trigger":       {},
	"entry_price":         {},
	"stop_loss":           {},
	"take_profit":         {},
	"position_size":       {},
	"invalidation":        {},
	"max_risk_amount":     {},
	"max_risk_percent":    {},
	"rationale":           {},
	"ai_summary":          {},
	"executed_at":         {},
	"executed_journal_id": {},
}

var errTradePlanAlreadyClosed = errors.New("trade plan was already closed")

type TradePlanHandler struct {
	db *gorm.DB
}

type createTradePlanRequest struct {
	Ticker         string   `json:"ticker"`
	Direction      string   `json:"direction"`
	Status         *string  `json:"status"`
	PlanDate       string   `json:"plan_date"`
	ValidUntil     *string  `json:"valid_until"`
	UserThesisID   *string  `json:"user_thesis_id"`
	EntryTrigger   *string  `json:"entry_trigger"`
	EntryPrice     *float64 `json:"entry_price"`
	StopLoss       *float64 `json:"stop_loss"`
	TakeProfit     *float64 `json:"take_profit"`
	PositionSize   *float64 `json:"position_size"`
	Invalidation   *string  `json:"invalidation"`
	MaxRiskAmount  *float64 `json:"max_risk_amount"`
	MaxRiskPercent *float64 `json:"max_risk_percent"`
	Rationale      *string  `json:"rationale"`
	AISummary      *string  `json:"ai_summary"`
	Notes          *string  `json:"notes"`
}

type executeTradePlanRequest struct {
	Datetime string   `json:"datetime"`
	Name     string   `json:"name"`
	Quantity *float64 `json:"quantity"`
	Price    *float64 `json:"price"`
	Fee      *float64 `json:"fee"`
	Market   string   `json:"market"`
	Currency string   `json:"currency"`
	OrderID  *string  `json:"order_id"`
	Notes    *string  `json:"notes"`
}

type tradePlanResponse struct {
	ID                string     `json:"id"`
	Ticker            string     `json:"ticker"`
	Direction         string     `json:"direction"`
	Status            string     `json:"status"`
	PlanDate          string     `json:"plan_date"`
	ValidUntil        *string    `json:"valid_until"`
	UserThesisID      *string    `json:"user_thesis_id"`
	EntryTrigger      *string    `json:"entry_trigger"`
	EntryPrice        *float64   `json:"entry_price"`
	StopLoss          *float64   `json:"stop_loss"`
	TakeProfit        *float64   `json:"take_profit"`
	PositionSize      *float64   `json:"position_size"`
	Invalidation      *string    `json:"invalidation"`
	MaxRiskAmount     *float64   `json:"max_risk_amount"`
	MaxRiskPercent    *float64   `json:"max_risk_percent"`
	Rationale         *string    `json:"rationale"`
	AISummary         *string    `json:"ai_summary"`
	Notes             *string    `json:"notes"`
	ExecutedJournalID *string    `json:"executed_journal_id"`
	ExecutedAt        *time.Time `json:"executed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type executeTradePlanResponse struct {
	Plan    tradePlanResponse    `json:"plan"`
	Journal journalEntryResponse `json:"journal"`
}

func NewTradePlanHandler(database *gorm.DB) *TradePlanHandler {
	return &TradePlanHandler{db: database}
}

func (h *TradePlanHandler) List(c *gin.Context) {
	query := h.db.WithContext(c.Request.Context()).Order("updated_at DESC")
	if ticker := strings.ToUpper(strings.TrimSpace(c.Query("ticker"))); ticker != "" {
		query = query.Where("ticker = ?", ticker)
	}
	if status := strings.ToLower(strings.TrimSpace(c.Query("status"))); status != "" {
		if !models.ValidTradePlanStatus(status) {
			Fail(c, http.StatusBadRequest, "invalid trade plan status")
			return
		}
		query = query.Where("status = ?", status)
	}

	var plans []models.TradePlan
	if err := query.Limit(1000).Find(&plans).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plans could not be loaded")
		return
	}

	responses := make([]tradePlanResponse, 0, len(plans))
	for _, plan := range plans {
		responses = append(responses, newTradePlanResponse(plan))
	}
	OK(c, responses)
}

func (h *TradePlanHandler) Create(c *gin.Context) {
	var req createTradePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	plan, validationErr := req.toModel()
	if validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}
	if plan.UserThesisID != nil {
		var thesis models.UserThesis
		if err := h.db.WithContext(c.Request.Context()).First(&thesis, "id = ?", *plan.UserThesisID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				Fail(c, http.StatusBadRequest, "user thesis not found")
				return
			}
			Fail(c, http.StatusInternalServerError, "user thesis could not be loaded")
			return
		}
		if strings.ToUpper(strings.TrimSpace(thesis.Symbol)) != plan.Ticker {
			Fail(c, http.StatusBadRequest, "user thesis ticker does not match trade plan")
			return
		}
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&plan).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newTradePlanResponse(plan), Error: nil})
}

func (h *TradePlanHandler) Get(c *gin.Context) {
	plan, ok := h.findPlan(c)
	if !ok {
		return
	}
	OK(c, newTradePlanResponse(plan))
}

func (h *TradePlanHandler) Patch(c *gin.Context) {
	plan, ok := h.findPlan(c)
	if !ok {
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&raw); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	if plan.Status != models.TradePlanStatusDraft {
		for field := range raw {
			if _, locked := lockedTradePlanFields[field]; locked {
				Fail(c, http.StatusBadRequest, "locked trade plan fields cannot be modified")
				return
			}
		}
	}

	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}

	if status, present, err := decodeRequiredPatchString(raw, "status"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		next := strings.ToLower(strings.TrimSpace(status))
		if !models.ValidTradePlanStatus(next) {
			Fail(c, http.StatusBadRequest, "invalid trade plan status")
			return
		}
		if !validTradePlanStatusTransition(plan.Status, next) {
			Fail(c, http.StatusBadRequest, "invalid trade plan status transition")
			return
		}
		updates["status"] = next
	}

	if value, present, err := decodeRequiredPatchDate(raw, "plan_date"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		updates["plan_date"] = value
	}
	if value, present, err := decodeNullablePatchDate(raw, "valid_until"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		updates["valid_until"] = value
	}

	for _, field := range []string{"entry_price", "stop_loss", "take_profit", "position_size", "max_risk_amount", "max_risk_percent"} {
		value, present, err := decodeOptionalFloat(raw, field)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if !present {
			continue
		}
		if number, ok := value.(float64); ok {
			if validationErr := validatePlanNumber(field, number); validationErr != "" {
				Fail(c, http.StatusBadRequest, validationErr)
				return
			}
		}
		updates[field] = value
	}

	for _, field := range []string{"entry_trigger", "invalidation", "rationale", "ai_summary", "notes"} {
		value, present, err := decodeCleanPatchString(raw, field)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if present {
			updates[field] = value
		}
	}

	if err := h.db.WithContext(c.Request.Context()).Model(&plan).Updates(updates).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan could not be updated")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).First(&plan, "id = ?", plan.ID).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan could not be reloaded")
		return
	}
	OK(c, newTradePlanResponse(plan))
}

func (h *TradePlanHandler) Execute(c *gin.Context) {
	plan, ok := h.findPlan(c)
	if !ok {
		return
	}
	if plan.Status == models.TradePlanStatusExecuted {
		Fail(c, http.StatusBadRequest, "trade plan has already been executed")
		return
	}
	if plan.Status == models.TradePlanStatusCancelled || plan.Status == models.TradePlanStatusExpired {
		Fail(c, http.StatusBadRequest, "closed trade plan cannot be executed")
		return
	}
	if plan.Direction == models.TradePlanDirectionWatch {
		Fail(c, http.StatusBadRequest, "watch plans cannot be converted to journal entries")
		return
	}
	unacknowledged, err := findUnacknowledgedHighRiskCodes(h.db.WithContext(c.Request.Context()), plan.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "pre-trade risk acknowledgements could not be checked")
		return
	}
	if len(unacknowledged) > 0 {
		Fail(c, http.StatusConflict, "high-risk findings require acknowledgement: "+strings.Join(unacknowledged, ", "))
		return
	}

	var req executeTradePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	entry, validationErr := req.toJournalEntry(plan)
	if validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}

	now := time.Now().UTC()
	entry.ID = uuid.New()
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&models.TradePlan{}).
			Where("id = ? AND status IN ?", plan.ID, []string{models.TradePlanStatusDraft, models.TradePlanStatusWaiting}).
			Updates(map[string]interface{}{
				"status":              models.TradePlanStatusExecuted,
				"executed_journal_id": entry.ID,
				"executed_at":         now,
				"updated_at":          now,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errTradePlanAlreadyClosed
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		if errors.Is(err, errTradePlanAlreadyClosed) {
			Fail(c, http.StatusConflict, "trade plan was already executed or closed")
			return
		}
		Fail(c, http.StatusInternalServerError, "trade plan could not be executed")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).First(&plan, "id = ?", plan.ID).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "trade plan could not be reloaded")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Data: executeTradePlanResponse{
			Plan:    newTradePlanResponse(plan),
			Journal: newJournalEntryResponse(entry),
		},
		Error: nil,
	})
}

func findUnacknowledgedHighRiskCodes(database *gorm.DB, planID uuid.UUID) ([]string, error) {
	var review models.PreTradeRiskReview
	err := database.Where("trade_plan_id = ?", planID).Order("version DESC").First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	highRisk := make(map[string]bool)
	for _, findings := range [][]models.PreTradeFinding{review.HardConflicts, review.RiskWarnings, review.BehaviorReminders, review.UnresolvedQuestions, review.ChangesSinceResearch} {
		for _, finding := range findings {
			if finding.Severity == "high" {
				highRisk[finding.Code] = true
			}
		}
	}
	var acknowledgements []models.RiskAcknowledgement
	if err := database.Where("review_id = ?", review.ID).Find(&acknowledgements).Error; err != nil {
		return nil, err
	}
	for _, acknowledgement := range acknowledgements {
		delete(highRisk, acknowledgement.FindingCode)
	}
	codes := make([]string, 0, len(highRisk))
	for code := range highRisk {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

func (h *TradePlanHandler) findPlan(c *gin.Context) (models.TradePlan, bool) {
	var plan models.TradePlan
	err := h.db.WithContext(c.Request.Context()).First(&plan, "id = ?", c.Param("id")).Error
	if err == nil {
		return plan, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "trade plan not found")
		return plan, false
	}
	Fail(c, http.StatusInternalServerError, "trade plan could not be loaded")
	return plan, false
}

func (req createTradePlanRequest) toModel() (models.TradePlan, string) {
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		return models.TradePlan{}, "missing required field: ticker"
	}

	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction == "" {
		return models.TradePlan{}, "missing required field: direction"
	}
	if !models.ValidTradePlanDirection(direction) {
		return models.TradePlan{}, "direction must be buy, sell, or watch"
	}

	status := models.TradePlanStatusDraft
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	if !models.ValidTradePlanStatus(status) || status == models.TradePlanStatusExecuted {
		return models.TradePlan{}, "invalid trade plan status"
	}

	planDate, validationErr := parsePlanDateOrToday(req.PlanDate, "plan_date")
	if validationErr != "" {
		return models.TradePlan{}, validationErr
	}
	validUntil, validationErr := parseOptionalPlanDate(req.ValidUntil, "valid_until")
	if validationErr != "" {
		return models.TradePlan{}, validationErr
	}
	var userThesisID *uuid.UUID
	if req.UserThesisID != nil && strings.TrimSpace(*req.UserThesisID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.UserThesisID))
		if err != nil {
			return models.TradePlan{}, "user_thesis_id must be a valid UUID"
		}
		userThesisID = &parsed
	}

	for field, value := range map[string]*float64{
		"entry_price":      req.EntryPrice,
		"stop_loss":        req.StopLoss,
		"take_profit":      req.TakeProfit,
		"position_size":    req.PositionSize,
		"max_risk_amount":  req.MaxRiskAmount,
		"max_risk_percent": req.MaxRiskPercent,
	} {
		if value != nil {
			if validationErr := validatePlanNumber(field, *value); validationErr != "" {
				return models.TradePlan{}, validationErr
			}
		}
	}

	return models.TradePlan{
		Ticker:         ticker,
		Direction:      direction,
		Status:         status,
		PlanDate:       planDate,
		ValidUntil:     validUntil,
		UserThesisID:   userThesisID,
		EntryTrigger:   cleanOptionalString(req.EntryTrigger),
		EntryPrice:     req.EntryPrice,
		StopLoss:       req.StopLoss,
		TakeProfit:     req.TakeProfit,
		PositionSize:   req.PositionSize,
		Invalidation:   cleanOptionalString(req.Invalidation),
		MaxRiskAmount:  req.MaxRiskAmount,
		MaxRiskPercent: req.MaxRiskPercent,
		Rationale:      cleanOptionalString(req.Rationale),
		AISummary:      cleanOptionalString(req.AISummary),
		Notes:          cleanOptionalString(req.Notes),
	}, ""
}

func (req executeTradePlanRequest) toJournalEntry(plan models.TradePlan) (models.TradeJournalEntry, string) {
	datetimeText := strings.TrimSpace(req.Datetime)
	if datetimeText == "" {
		datetimeText = time.Now().UTC().Format(time.RFC3339)
	}
	executedAt, err := parseJournalDatetime(datetimeText)
	if err != nil {
		return models.TradeJournalEntry{}, "datetime must be a valid ISO8601 timestamp"
	}

	price := firstFloat(req.Price, plan.EntryPrice)
	if price == nil {
		return models.TradeJournalEntry{}, "missing required field: price"
	}
	if *price <= 0 {
		return models.TradeJournalEntry{}, "price must be positive"
	}
	if req.Quantity == nil {
		return models.TradeJournalEntry{}, "missing required field: quantity"
	}
	if *req.Quantity <= 0 {
		return models.TradeJournalEntry{}, "quantity must be positive"
	}
	fee := 0.0
	if req.Fee != nil {
		fee = *req.Fee
	}
	if fee < 0 {
		return models.TradeJournalEntry{}, "fee must be zero or positive"
	}
	market := strings.ToLower(strings.TrimSpace(req.Market))
	if market == "" {
		market = inferTradeMarket(plan.Ticker)
	}
	if !models.ValidTradeMarket(market) {
		return models.TradeJournalEntry{}, "market must be us, hk, china_a, crypto, or other"
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = defaultTradeCurrency(market)
	}

	notes := cleanOptionalString(req.Notes)
	if notes == nil {
		notes = plan.Notes
	}

	var tradePlanID *uuid.UUID
	if plan.ID != uuid.Nil {
		id := plan.ID
		tradePlanID = &id
	}

	return models.TradeJournalEntry{
		ExecutedAt:  executedAt,
		Symbol:      plan.Ticker,
		Name:        strings.TrimSpace(req.Name),
		Side:        plan.Direction,
		Quantity:    *req.Quantity,
		Price:       *price,
		Amount:      *req.Quantity * *price,
		Fee:         fee,
		Market:      market,
		Currency:    currency,
		OrderID:     cleanOptionalString(req.OrderID),
		TradePlanID: tradePlanID,
		Source:      "trade_plan",
		Notes:       notes,
	}, ""
}

func newTradePlanResponse(plan models.TradePlan) tradePlanResponse {
	return tradePlanResponse{
		ID:                plan.ID.String(),
		Ticker:            plan.Ticker,
		Direction:         plan.Direction,
		Status:            plan.Status,
		PlanDate:          plan.PlanDate.Format(journalDateLayout),
		ValidUntil:        formatDatePointer(plan.ValidUntil),
		UserThesisID:      formatUUIDPointer(plan.UserThesisID),
		EntryTrigger:      plan.EntryTrigger,
		EntryPrice:        plan.EntryPrice,
		StopLoss:          plan.StopLoss,
		TakeProfit:        plan.TakeProfit,
		PositionSize:      plan.PositionSize,
		Invalidation:      plan.Invalidation,
		MaxRiskAmount:     plan.MaxRiskAmount,
		MaxRiskPercent:    plan.MaxRiskPercent,
		Rationale:         plan.Rationale,
		AISummary:         plan.AISummary,
		Notes:             plan.Notes,
		ExecutedJournalID: formatUUIDPointer(plan.ExecutedJournalID),
		ExecutedAt:        plan.ExecutedAt,
		CreatedAt:         plan.CreatedAt,
		UpdatedAt:         plan.UpdatedAt,
	}
}

func parsePlanDateOrToday(value string, field string) (time.Time, string) {
	text := strings.TrimSpace(value)
	if text == "" {
		text = time.Now().UTC().Format(journalDateLayout)
	}
	parsed, err := time.Parse(journalDateLayout, text)
	if err != nil {
		return time.Time{}, field + " must be a valid date"
	}
	return parsed, ""
}

func parseOptionalPlanDate(value *string, field string) (*time.Time, string) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, ""
	}
	parsed, err := time.Parse(journalDateLayout, strings.TrimSpace(*value))
	if err != nil {
		return nil, field + " must be a valid date"
	}
	return &parsed, ""
}

func validatePlanNumber(field string, value float64) string {
	if value <= 0 {
		return field + " must be positive"
	}
	if (field == "position_size" || field == "max_risk_percent") && value > 100 {
		return field + " must be 100 or less"
	}
	return ""
}

func validTradePlanStatusTransition(current string, next string) bool {
	if current == next {
		return true
	}
	switch current {
	case models.TradePlanStatusDraft:
		return next == models.TradePlanStatusWaiting || next == models.TradePlanStatusCancelled || next == models.TradePlanStatusExpired
	case models.TradePlanStatusWaiting:
		return next == models.TradePlanStatusCancelled || next == models.TradePlanStatusExpired
	default:
		return false
	}
}

func decodeRequiredPatchString(raw map[string]json.RawMessage, field string) (string, bool, error) {
	value, ok := raw[field]
	if !ok {
		return "", false, nil
	}
	if isJSONNull(value) {
		return "", true, errors.New(field + " must be a string")
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", true, errors.New(field + " must be a string")
	}
	return text, true, nil
}

func decodeCleanPatchString(raw map[string]json.RawMessage, field string) (interface{}, bool, error) {
	value, present, err := decodeOptionalString(raw, field)
	if err != nil || !present || value == nil {
		return value, present, err
	}
	text, ok := value.(string)
	if !ok {
		return value, present, nil
	}
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return nil, present, nil
	}
	return cleaned, present, nil
}

func decodeRequiredPatchDate(raw map[string]json.RawMessage, field string) (time.Time, bool, error) {
	value, ok := raw[field]
	if !ok {
		return time.Time{}, false, nil
	}
	if isJSONNull(value) {
		return time.Time{}, true, errors.New(field + " must be a valid date")
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return time.Time{}, true, errors.New(field + " must be a valid date")
	}
	parsed, err := time.Parse(journalDateLayout, strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, true, errors.New(field + " must be a valid date")
	}
	return parsed, true, nil
}

func decodeNullablePatchDate(raw map[string]json.RawMessage, field string) (interface{}, bool, error) {
	value, ok := raw[field]
	if !ok {
		return nil, false, nil
	}
	if isJSONNull(value) {
		return nil, true, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return nil, true, errors.New(field + " must be a valid date")
	}
	if strings.TrimSpace(text) == "" {
		return nil, true, nil
	}
	parsed, err := time.Parse(journalDateLayout, strings.TrimSpace(text))
	if err != nil {
		return nil, true, errors.New(field + " must be a valid date")
	}
	return parsed, true, nil
}

func firstFloat(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func formatDatePointer(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(journalDateLayout)
	return &formatted
}

func formatUUIDPointer(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	formatted := value.String()
	return &formatted
}
