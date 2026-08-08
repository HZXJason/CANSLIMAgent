package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCreateTradePlanRequestBuildsPlan(t *testing.T) {
	entryPrice := 125.5
	stopLoss := 119.25
	positionSize := 8.5
	status := models.TradePlanStatusWaiting
	validUntil := "2026-08-15"
	trigger := "突破 125 后成交量放大"
	thesisID := uuid.New()
	thesisIDText := thesisID.String()
	invalidation := "跌破 119 且无法收回"
	maxRiskAmount := 800.0
	maxRiskPercent := 0.8

	plan, validationErr := createTradePlanRequest{
		Ticker:         " nvda ",
		Direction:      "BUY",
		Status:         &status,
		PlanDate:       "2026-07-05",
		ValidUntil:     &validUntil,
		UserThesisID:   &thesisIDText,
		EntryTrigger:   &trigger,
		EntryPrice:     &entryPrice,
		StopLoss:       &stopLoss,
		PositionSize:   &positionSize,
		Invalidation:   &invalidation,
		MaxRiskAmount:  &maxRiskAmount,
		MaxRiskPercent: &maxRiskPercent,
	}.toModel()

	if validationErr != "" {
		t.Fatalf("unexpected validation error: %s", validationErr)
	}
	if plan.Ticker != "NVDA" {
		t.Fatalf("expected ticker to be normalized, got %q", plan.Ticker)
	}
	if plan.Direction != models.TradePlanDirectionBuy {
		t.Fatalf("unexpected direction: %s", plan.Direction)
	}
	if plan.Status != models.TradePlanStatusWaiting {
		t.Fatalf("unexpected status: %s", plan.Status)
	}
	if plan.ValidUntil == nil || plan.ValidUntil.Format(journalDateLayout) != validUntil {
		t.Fatalf("expected valid_until to be parsed, got %+v", plan.ValidUntil)
	}
	if plan.UserThesisID == nil || *plan.UserThesisID != thesisID {
		t.Fatalf("expected thesis link %s, got %+v", thesisID, plan.UserThesisID)
	}
	if plan.Invalidation == nil || *plan.Invalidation != invalidation {
		t.Fatalf("expected invalidation to be preserved, got %+v", plan.Invalidation)
	}
	if plan.MaxRiskAmount == nil || *plan.MaxRiskAmount != maxRiskAmount || plan.MaxRiskPercent == nil || *plan.MaxRiskPercent != maxRiskPercent {
		t.Fatalf("expected risk budget to be preserved, got amount=%+v percent=%+v", plan.MaxRiskAmount, plan.MaxRiskPercent)
	}
}

func TestCreateTradePlanRejectsInvalidPositionSize(t *testing.T) {
	positionSize := 120.0
	_, validationErr := createTradePlanRequest{
		Ticker:       "AAPL",
		Direction:    models.TradePlanDirectionBuy,
		PlanDate:     "2026-07-05",
		PositionSize: &positionSize,
	}.toModel()

	if validationErr != "position_size must be 100 or less" {
		t.Fatalf("expected position size validation, got %q", validationErr)
	}
}

func TestCreateTradePlanRejectsThesisForDifferentTicker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.UserThesis{}, &models.TradePlan{}); err != nil {
		t.Fatal(err)
	}
	thesis := models.UserThesis{
		ResearchCaseID: uuid.New(),
		Symbol:         "NVDA",
		Version:        1,
		Stance:         "long",
		Summary:        "需求持续但财报波动风险较高",
	}
	if err := database.Create(&thesis).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewTradePlanHandler(database)
	router := gin.New()
	router.POST("/api/v1/trade-plans", handler.Create)
	response := httptest.NewRecorder()
	body := `{"ticker":"AAPL","direction":"buy","user_thesis_id":"` + thesis.ID.String() + `"}`
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans", strings.NewReader(body)))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched thesis, got %d: %s", response.Code, response.Body.String())
	}
}

func TestExecuteTradePlanUsesPlanDefaults(t *testing.T) {
	entryPrice := 125.5
	quantity := 12.0
	planID := uuid.New()
	plan := models.TradePlan{
		ID:         planID,
		Ticker:     "NVDA",
		Direction:  models.TradePlanDirectionBuy,
		EntryPrice: &entryPrice,
	}

	entry, validationErr := executeTradePlanRequest{Datetime: "2026-07-06T14:30", Quantity: &quantity}.toJournalEntry(plan)

	if validationErr != "" {
		t.Fatalf("unexpected validation error: %s", validationErr)
	}
	if entry.Symbol != "NVDA" || entry.Side != models.TradeSideBuy {
		t.Fatalf("unexpected journal identity: %+v", entry)
	}
	if entry.Price != entryPrice || entry.Quantity != quantity || entry.Amount != entryPrice*quantity {
		t.Fatalf("expected journal values from plan defaults, got %+v", entry)
	}
	if entry.Market != models.TradeMarketUS || entry.Source != "trade_plan" {
		t.Fatalf("expected canonical market/source fields, got %+v", entry)
	}
	if entry.TradePlanID == nil || *entry.TradePlanID != planID {
		t.Fatalf("expected journal entry to link plan %s, got %+v", planID, entry.TradePlanID)
	}
}

func TestTradePlanStatusTransitions(t *testing.T) {
	if !validTradePlanStatusTransition(models.TradePlanStatusDraft, models.TradePlanStatusWaiting) {
		t.Fatalf("expected draft to waiting to be valid")
	}
	if validTradePlanStatusTransition(models.TradePlanStatusExecuted, models.TradePlanStatusDraft) {
		t.Fatalf("expected executed plan to stay terminal")
	}
	if validTradePlanStatusTransition(models.TradePlanStatusWaiting, models.TradePlanStatusExecuted) {
		t.Fatalf("execute endpoint should own the executed transition")
	}
}

func TestTradePlanExecutionRequiresLatestHighRiskAcknowledgements(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.PreTradeRiskReview{}, &models.RiskAcknowledgement{}); err != nil {
		t.Fatal(err)
	}
	planID := uuid.New()
	review := models.PreTradeRiskReview{
		TradePlanID: planID, Version: 1, PlanSnapshot: map[string]any{},
		HardConflicts: []models.PreTradeFinding{{Code: "missing_stop_loss", Severity: "high", Category: "deterministic", Message: "缺少止损", Evidence: map[string]any{}}},
		RiskWarnings:  []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{},
		Freshness: map[string]any{}, Sources: []models.PreTradeSource{}, AIStatus: models.PreTradeAIStatusUnavailable,
	}
	if err := database.Create(&review).Error; err != nil {
		t.Fatal(err)
	}

	codes, err := findUnacknowledgedHighRiskCodes(database, planID)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1 || codes[0] != "missing_stop_loss" {
		t.Fatalf("expected high-risk acknowledgement gate, got %+v", codes)
	}
	if err := database.Create(&models.RiskAcknowledgement{ReviewID: review.ID, FindingCode: "missing_stop_loss"}).Error; err != nil {
		t.Fatal(err)
	}
	codes, err = findUnacknowledgedHighRiskCodes(database, planID)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 0 {
		t.Fatalf("expected acknowledged risk to stop blocking execution, got %+v", codes)
	}
}
