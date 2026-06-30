package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const journalDateLayout = "2006-01-02"

var phase1JournalFields = map[string]struct{}{
	"ticker":        {},
	"direction":     {},
	"trade_date":    {},
	"price":         {},
	"position_size": {},
	"stop_loss":     {},
	"notes":         {},
}

type JournalHandler struct {
	db *gorm.DB
}

type createJournalEntryRequest struct {
	Ticker       string   `json:"ticker"`
	Direction    string   `json:"direction"`
	TradeDate    string   `json:"trade_date"`
	Price        *float64 `json:"price"`
	PositionSize *float64 `json:"position_size"`
	StopLoss     *float64 `json:"stop_loss"`
	Notes        *string  `json:"notes"`
}

type journalEntryResponse struct {
	ID           string    `json:"id"`
	Ticker       string    `json:"ticker"`
	Direction    string    `json:"direction"`
	TradeDate    string    `json:"trade_date"`
	Price        float64   `json:"price"`
	PositionSize float64   `json:"position_size"`
	StopLoss     float64   `json:"stop_loss"`
	Notes        *string   `json:"notes"`
	ExitPrice    *float64  `json:"exit_price"`
	PnLPct       *float64  `json:"pnl_pct"`
	Reflection   *string   `json:"reflection"`
	Phase1Locked bool      `json:"phase1_locked"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func NewJournalHandler(database *gorm.DB) *JournalHandler {
	return &JournalHandler{db: database}
}

func (h *JournalHandler) List(c *gin.Context) {
	var entries []models.TradeJournalEntry
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&entries).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal entries could not be loaded")
		return
	}

	responses := make([]journalEntryResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, newJournalEntryResponse(entry))
	}
	OK(c, responses)
}

func (h *JournalHandler) Create(c *gin.Context) {
	var req createJournalEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	entry, validationErr := req.toModel()
	if validationErr != "" {
		Fail(c, http.StatusBadRequest, validationErr)
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&entry).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newJournalEntryResponse(entry), Error: nil})
}

func (h *JournalHandler) Get(c *gin.Context) {
	entry, ok := h.findEntry(c)
	if !ok {
		return
	}
	OK(c, newJournalEntryResponse(entry))
}

func (h *JournalHandler) Patch(c *gin.Context) {
	entry, ok := h.findEntry(c)
	if !ok {
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request.Body).Decode(&raw); err != nil {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	for field := range raw {
		if _, locked := phase1JournalFields[field]; locked {
			Fail(c, http.StatusBadRequest, "phase 1 fields are locked and cannot be modified")
			return
		}
	}

	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}
	if value, present, err := decodeOptionalFloat(raw, "exit_price"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		updates["exit_price"] = value
	}
	if value, present, err := decodeOptionalFloat(raw, "pnl_pct"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		updates["pnl_pct"] = value
	}
	if value, present, err := decodeOptionalString(raw, "reflection"); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	} else if present {
		updates["reflection"] = value
	}

	if err := h.db.WithContext(c.Request.Context()).Model(&entry).Updates(updates).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be updated")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).First(&entry, "id = ?", entry.ID).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be reloaded")
		return
	}
	OK(c, newJournalEntryResponse(entry))
}

func (h *JournalHandler) findEntry(c *gin.Context) (models.TradeJournalEntry, bool) {
	var entry models.TradeJournalEntry
	err := h.db.WithContext(c.Request.Context()).First(&entry, "id = ?", c.Param("id")).Error
	if err == nil {
		return entry, true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusNotFound, "entry not found")
		return entry, false
	}
	Fail(c, http.StatusInternalServerError, "journal entry could not be loaded")
	return entry, false
}

func (req createJournalEntryRequest) toModel() (models.TradeJournalEntry, string) {
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" {
		return models.TradeJournalEntry{}, "missing required field: ticker"
	}

	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction == "" {
		return models.TradeJournalEntry{}, "missing required field: direction"
	}
	if !models.ValidTradeDirection(direction) {
		return models.TradeJournalEntry{}, "direction must be buy or sell"
	}

	tradeDateText := strings.TrimSpace(req.TradeDate)
	if tradeDateText == "" {
		return models.TradeJournalEntry{}, "missing required field: trade_date"
	}
	tradeDate, err := time.Parse(journalDateLayout, tradeDateText)
	if err != nil {
		return models.TradeJournalEntry{}, "trade_date must be a valid date"
	}

	if req.Price == nil {
		return models.TradeJournalEntry{}, "missing required field: price"
	}
	if *req.Price <= 0 {
		return models.TradeJournalEntry{}, "price must be positive"
	}
	if req.PositionSize == nil {
		return models.TradeJournalEntry{}, "missing required field: position_size"
	}
	if *req.PositionSize <= 0 {
		return models.TradeJournalEntry{}, "position_size must be positive"
	}
	if req.StopLoss == nil {
		return models.TradeJournalEntry{}, "missing required field: stop_loss"
	}
	if *req.StopLoss <= 0 {
		return models.TradeJournalEntry{}, "stop_loss must be positive"
	}

	return models.TradeJournalEntry{
		Ticker:       ticker,
		Direction:    direction,
		TradeDate:    tradeDate,
		Price:        *req.Price,
		PositionSize: *req.PositionSize,
		StopLoss:     *req.StopLoss,
		Notes:        cleanOptionalString(req.Notes),
		Phase1Locked: true,
	}, ""
}

func newJournalEntryResponse(entry models.TradeJournalEntry) journalEntryResponse {
	return journalEntryResponse{
		ID:           entry.ID.String(),
		Ticker:       entry.Ticker,
		Direction:    entry.Direction,
		TradeDate:    entry.TradeDate.Format(journalDateLayout),
		Price:        entry.Price,
		PositionSize: entry.PositionSize,
		StopLoss:     entry.StopLoss,
		Notes:        entry.Notes,
		ExitPrice:    entry.ExitPrice,
		PnLPct:       entry.PnLPct,
		Reflection:   entry.Reflection,
		Phase1Locked: entry.Phase1Locked,
		CreatedAt:    entry.CreatedAt,
		UpdatedAt:    entry.UpdatedAt,
	}
}

func cleanOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func decodeOptionalFloat(raw map[string]json.RawMessage, field string) (interface{}, bool, error) {
	value, ok := raw[field]
	if !ok {
		return nil, false, nil
	}
	if isJSONNull(value) {
		return nil, true, nil
	}
	var number float64
	if err := json.Unmarshal(value, &number); err != nil {
		return nil, true, errors.New(field + " must be a number or null")
	}
	return number, true, nil
}

func decodeOptionalString(raw map[string]json.RawMessage, field string) (interface{}, bool, error) {
	value, ok := raw[field]
	if !ok {
		return nil, false, nil
	}
	if isJSONNull(value) {
		return nil, true, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return nil, true, errors.New(field + " must be a string or null")
	}
	return text, true, nil
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
