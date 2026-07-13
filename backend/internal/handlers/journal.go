package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const journalDateLayout = "2006-01-02"

type JournalHandler struct {
	db                 *gorm.DB
	reflectionUpstream services.JournalReflectionUpstream
}

type createJournalEntryRequest struct {
	Datetime string   `json:"datetime"`
	Symbol   string   `json:"symbol"`
	Name     string   `json:"name"`
	Side     string   `json:"side"`
	Quantity *float64 `json:"quantity"`
	Price    *float64 `json:"price"`
	Amount   *float64 `json:"amount"`
	Fee      *float64 `json:"fee"`
	Market   string   `json:"market"`
	Currency string   `json:"currency"`
	OrderID  *string  `json:"order_id"`
	Notes    *string  `json:"notes"`
}

type journalEntryResponse struct {
	ID         string    `json:"id"`
	Datetime   string    `json:"datetime"`
	Symbol     string    `json:"symbol"`
	Name       string    `json:"name"`
	Side       string    `json:"side"`
	Quantity   float64   `json:"quantity"`
	Price      float64   `json:"price"`
	Amount     float64   `json:"amount"`
	Fee        float64   `json:"fee"`
	Market     string    `json:"market"`
	Currency   string    `json:"currency"`
	OrderID    *string   `json:"order_id"`
	Source     string    `json:"source"`
	Notes      *string   `json:"notes"`
	Reflection *string   `json:"reflection"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// pythonTradeRecord is the exact Vibe-Trading TradeRecord contract.
type pythonTradeRecord struct {
	Datetime string  `json:"datetime"`
	Symbol   string  `json:"symbol"`
	Name     string  `json:"name"`
	Side     string  `json:"side"`
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price"`
	Amount   float64 `json:"amount"`
	Fee      float64 `json:"fee"`
	Market   string  `json:"market"`
}

func NewJournalHandler(database *gorm.DB) *JournalHandler {
	return &JournalHandler{db: database}
}

func NewJournalHandlerWithReflection(database *gorm.DB, upstream services.JournalReflectionUpstream) *JournalHandler {
	return &JournalHandler{db: database, reflectionUpstream: upstream}
}

func (h *JournalHandler) List(c *gin.Context) {
	entries, err := h.listEntries(c, 1000)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal entries could not be loaded")
		return
	}

	responses := make([]journalEntryResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, newJournalEntryResponse(entry))
	}
	OK(c, responses)
}

func (h *JournalHandler) Export(c *gin.Context) {
	entries, err := h.listEntries(c, 10000)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal entries could not be exported")
		return
	}

	records := make([]pythonTradeRecord, 0, len(entries))
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Quantity <= 0 || entry.Price <= 0 {
			continue
		}
		records = append(records, newPythonTradeRecord(entry))
	}
	OK(c, gin.H{
		"schema_version": "vibe.trade_record.v1",
		"records":        records,
	})
}

func (h *JournalHandler) Reflect(c *gin.Context) {
	if h.reflectionUpstream == nil {
		Fail(c, http.StatusServiceUnavailable, "journal reflection service is not configured")
		return
	}
	entries, err := h.listEntries(c, 2000)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal entries could not be loaded for reflection")
		return
	}
	records := make([]services.JournalTradeRecord, 0, len(entries))
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Quantity <= 0 || entry.Price <= 0 {
			continue
		}
		records = append(records, services.JournalTradeRecord{
			Datetime: entry.ExecutedAt.Format("2006-01-02 15:04:05"),
			Symbol:   entry.Symbol,
			Name:     entry.Name,
			Side:     entry.Side,
			Quantity: entry.Quantity,
			Price:    entry.Price,
			Amount:   entry.Amount,
			Fee:      entry.Fee,
			Market:   entry.Market,
		})
	}
	if len(records) == 0 {
		Fail(c, http.StatusBadRequest, "no structured trade records are available for reflection")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	result, err := h.reflectionUpstream.Reflect(ctx, services.JournalReflectionRequest{
		SchemaVersion: "vibe.trade_record.v1",
		Records:       records,
	})
	if err != nil {
		Fail(c, http.StatusBadGateway, "journal reflection failed: "+err.Error())
		return
	}
	scope := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	if scope == "" {
		scope = "all"
	}
	engine, _ := result["engine"].(string)
	if engine == "" {
		engine = "vibe-trading"
	}
	reflection := models.TradeJournalReflection{
		Scope:       scope,
		RecordCount: len(records),
		Engine:      engine,
		Payload:     map[string]any(result),
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&reflection).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal reflection could not be saved")
		return
	}
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: newJournalReflectionResponse(reflection), Error: nil})
}

func (h *JournalHandler) LatestReflection(c *gin.Context) {
	scope := strings.ToUpper(strings.TrimSpace(c.Query("symbol")))
	if scope == "" {
		scope = "all"
	}
	var reflection models.TradeJournalReflection
	err := h.db.WithContext(c.Request.Context()).Where("scope = ?", scope).Order("created_at DESC").First(&reflection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "journal reflection could not be loaded")
		return
	}
	OK(c, newJournalReflectionResponse(reflection))
}

func (h *JournalHandler) listEntries(c *gin.Context, limit int) ([]models.TradeJournalEntry, error) {
	query := h.db.WithContext(c.Request.Context()).Order("executed_at DESC")
	if symbol := strings.ToUpper(strings.TrimSpace(c.Query("symbol"))); symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var entries []models.TradeJournalEntry
	err := query.Find(&entries).Error
	return entries, err
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
		if field != "notes" && field != "reflection" {
			Fail(c, http.StatusBadRequest, "execution fields are immutable")
			return
		}
	}

	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	for _, field := range []string{"notes", "reflection"} {
		value, present, err := decodeOptionalString(raw, field)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if present {
			if text, ok := value.(string); ok {
				value = strings.TrimSpace(text)
				if value == "" {
					value = nil
				}
			}
			updates[field] = value
		}
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

// Delete removes an execution fill. Executions are immutable once recorded,
// but an invalid or accidental entry must not keep breaking downstream
// analysis forever.
func (h *JournalHandler) Delete(c *gin.Context) {
	entry, ok := h.findEntry(c)
	if !ok {
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&entry).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "journal entry could not be deleted")
		return
	}
	OK(c, gin.H{"deleted": true})
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
	executedAt, err := parseJournalDatetime(req.Datetime)
	if err != nil {
		return models.TradeJournalEntry{}, "datetime must be a valid ISO8601 timestamp"
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return models.TradeJournalEntry{}, "missing required field: symbol"
	}
	side := strings.ToLower(strings.TrimSpace(req.Side))
	if !models.ValidTradeSide(side) {
		return models.TradeJournalEntry{}, "side must be buy or sell"
	}
	if req.Quantity == nil || *req.Quantity <= 0 {
		return models.TradeJournalEntry{}, "quantity must be positive"
	}
	if req.Price == nil || *req.Price <= 0 {
		return models.TradeJournalEntry{}, "price must be positive"
	}

	amount := *req.Quantity * *req.Price
	if req.Amount != nil {
		if *req.Amount <= 0 {
			return models.TradeJournalEntry{}, "amount must be positive"
		}
		amount = *req.Amount
	}
	fee := 0.0
	if req.Fee != nil {
		if *req.Fee < 0 {
			return models.TradeJournalEntry{}, "fee must be zero or positive"
		}
		fee = *req.Fee
	}

	market := strings.ToLower(strings.TrimSpace(req.Market))
	if market == "" {
		market = inferTradeMarket(symbol)
	}
	if !models.ValidTradeMarket(market) {
		return models.TradeJournalEntry{}, "market must be us, hk, china_a, crypto, or other"
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = defaultTradeCurrency(market)
	}

	return models.TradeJournalEntry{
		ExecutedAt: executedAt,
		Symbol:     symbol,
		Name:       strings.TrimSpace(req.Name),
		Side:       side,
		Quantity:   *req.Quantity,
		Price:      *req.Price,
		Amount:     amount,
		Fee:        fee,
		Market:     market,
		Currency:   currency,
		OrderID:    cleanOptionalString(req.OrderID),
		Source:     "manual",
		Notes:      cleanOptionalString(req.Notes),
	}, ""
}

func parseJournalDatetime(value string) (time.Time, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return time.Time{}, errors.New("datetime is required")
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		journalDateLayout,
	} {
		if parsed, err := time.Parse(layout, cleaned); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("invalid datetime")
}

func inferTradeMarket(symbol string) string {
	upper := strings.ToUpper(symbol)
	switch {
	case strings.HasSuffix(upper, ".HK"):
		return models.TradeMarketHK
	case strings.HasSuffix(upper, ".SH"), strings.HasSuffix(upper, ".SZ"), strings.HasSuffix(upper, ".BJ"):
		return models.TradeMarketChinaA
	case strings.Contains(upper, "USDT"), strings.Contains(upper, "USDC"), strings.Contains(upper, "-USD"):
		return models.TradeMarketCrypto
	case isASCIIAlpha(upper):
		return models.TradeMarketUS
	default:
		return models.TradeMarketOther
	}
}

func isASCIIAlpha(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}

func defaultTradeCurrency(market string) string {
	switch market {
	case models.TradeMarketUS:
		return "USD"
	case models.TradeMarketHK:
		return "HKD"
	case models.TradeMarketChinaA:
		return "CNY"
	case models.TradeMarketCrypto:
		return "USDT"
	default:
		return ""
	}
}

func newJournalEntryResponse(entry models.TradeJournalEntry) journalEntryResponse {
	return journalEntryResponse{
		ID:         entry.ID.String(),
		Datetime:   entry.ExecutedAt.Format(time.RFC3339),
		Symbol:     entry.Symbol,
		Name:       entry.Name,
		Side:       entry.Side,
		Quantity:   entry.Quantity,
		Price:      entry.Price,
		Amount:     entry.Amount,
		Fee:        entry.Fee,
		Market:     entry.Market,
		Currency:   entry.Currency,
		OrderID:    entry.OrderID,
		Source:     entry.Source,
		Notes:      entry.Notes,
		Reflection: entry.Reflection,
		CreatedAt:  entry.CreatedAt,
		UpdatedAt:  entry.UpdatedAt,
	}
}

func newPythonTradeRecord(entry models.TradeJournalEntry) pythonTradeRecord {
	return pythonTradeRecord{
		Datetime: entry.ExecutedAt.Format("2006-01-02 15:04:05"),
		Symbol:   entry.Symbol,
		Name:     entry.Name,
		Side:     entry.Side,
		Quantity: entry.Quantity,
		Price:    entry.Price,
		Amount:   entry.Amount,
		Fee:      entry.Fee,
		Market:   entry.Market,
	}
}

func newJournalReflectionResponse(reflection models.TradeJournalReflection) gin.H {
	return gin.H{
		"id":           reflection.ID.String(),
		"created_at":   reflection.CreatedAt,
		"scope":        reflection.Scope,
		"record_count": reflection.RecordCount,
		"engine":       reflection.Engine,
		"payload":      reflection.Payload,
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
