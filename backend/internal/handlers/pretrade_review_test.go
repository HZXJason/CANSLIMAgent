package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type recordingPreTradeReviewer struct {
	input services.PreTradeAIReviewInput
}

func (reviewer *recordingPreTradeReviewer) Review(_ context.Context, input services.PreTradeAIReviewInput) (services.PreTradeAIReviewOutput, error) {
	reviewer.input = input
	return services.PreTradeAIReviewOutput{
		RiskWarnings: []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{},
	}, nil
}

func TestPreTradeReviewReturnsDeterministicConflictsWhenAIUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.TradePlan{}, &models.PreTradeRiskReview{}, &models.RiskAcknowledgement{}); err != nil {
		t.Fatal(err)
	}
	entry := 100.0
	stop := 105.0
	plan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusDraft, PlanDate: time.Now().UTC(), EntryPrice: &entry, StopLoss: &stop}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewPreTradeReviewHandler(database, nil, nil)
	router := gin.New()
	router.POST("/api/v1/trade-plans/:id/risk-reviews", handler.Create)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews", strings.NewReader(`{"run_ai":true,"llm_provider":"openai","model":"gpt-test"}`)))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			ID            string                   `json:"id"`
			Version       int                      `json:"version"`
			AIStatus      string                   `json:"ai_status"`
			HardConflicts []models.PreTradeFinding `json:"hard_conflicts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ID == "" || payload.Data.Version != 1 || payload.Data.AIStatus != models.PreTradeAIStatusUnavailable {
		t.Fatalf("unexpected review identity/status: %+v", payload.Data)
	}
	if !findingCodesForHandler(payload.Data.HardConflicts)["buy_stop_not_below_entry"] {
		t.Fatalf("expected deterministic conflict, got %+v", payload.Data.HardConflicts)
	}
}

func TestPreTradeReviewListsLatestAndAcknowledgesKnownFinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.TradePlan{}, &models.PreTradeRiskReview{}, &models.RiskAcknowledgement{}); err != nil {
		t.Fatal(err)
	}
	plan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusDraft, PlanDate: time.Now().UTC()}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	finding := models.PreTradeFinding{Code: "missing_stop_loss", Severity: "high", Category: "deterministic", Message: "missing", Evidence: map[string]any{}}
	base := models.PreTradeRiskReview{
		TradePlanID: plan.ID, Version: 1, PlanSnapshot: map[string]any{"ticker": "NVDA"}, HardConflicts: []models.PreTradeFinding{finding},
		RiskWarnings: []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{}, AIStatus: models.PreTradeAIStatusUnavailable,
	}
	if err := database.Create(&base).Error; err != nil {
		t.Fatal(err)
	}
	latest := base
	latest.ID = [16]byte{}
	latest.Version = 2
	if err := database.Create(&latest).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewPreTradeReviewHandler(database, nil, nil)
	router := gin.New()
	router.GET("/api/v1/trade-plans/:id/risk-reviews", handler.List)
	router.GET("/api/v1/trade-plans/:id/risk-reviews/latest", handler.Latest)
	router.POST("/api/v1/trade-plans/:id/risk-reviews/:reviewId/acknowledgements", handler.Acknowledge)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"version":1`) || !strings.Contains(list.Body.String(), `"version":2`) {
		t.Fatalf("expected both review versions, got %d: %s", list.Code, list.Body.String())
	}
	latestResponse := httptest.NewRecorder()
	router.ServeHTTP(latestResponse, httptest.NewRequest(http.MethodGet, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews/latest", nil))
	if latestResponse.Code != http.StatusOK || !strings.Contains(latestResponse.Body.String(), `"version":2`) {
		t.Fatalf("expected latest version 2, got %d: %s", latestResponse.Code, latestResponse.Body.String())
	}
	ack := httptest.NewRecorder()
	ackBody := `{"finding_code":"missing_stop_loss","reason":"我会在外部下单前设置保护止损"}`
	router.ServeHTTP(ack, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews/"+latest.ID.String()+"/acknowledgements", strings.NewReader(ackBody)))
	if ack.Code != http.StatusCreated || !strings.Contains(ack.Body.String(), `"finding_code":"missing_stop_loss"`) {
		t.Fatalf("expected acknowledgement, got %d: %s", ack.Code, ack.Body.String())
	}
	otherPlan := models.TradePlan{Ticker: "AAPL", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusDraft, PlanDate: time.Now().UTC()}
	if err := database.Create(&otherPlan).Error; err != nil {
		t.Fatal(err)
	}
	mismatch := httptest.NewRecorder()
	router.ServeHTTP(mismatch, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans/"+otherPlan.ID.String()+"/risk-reviews/"+latest.ID.String()+"/acknowledgements", strings.NewReader(ackBody)))
	if mismatch.Code != http.StatusNotFound {
		t.Fatalf("expected review/plan mismatch to return 404, got %d: %s", mismatch.Code, mismatch.Body.String())
	}
}

func findingCodesForHandler(findings []models.PreTradeFinding) map[string]bool {
	result := make(map[string]bool, len(findings))
	for _, finding := range findings {
		result[finding.Code] = true
	}
	return result
}

func TestPreTradeReviewSendsOnlyEligibleBehaviorEvidenceToAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.TradePlan{}, &models.PreTradeRiskReview{}, &models.RiskAcknowledgement{}, &models.TradeReview{}, &models.BehaviorObservation{}, &models.TradingProfile{}, &models.TradingPrinciple{}, &models.TradingPlaybook{}, &models.AIMemoryPreference{}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		completedAt := time.Now().UTC()
		planID := uuid.New()
		deviations := []string{}
		if index < 4 {
			deviations = append(deviations, "adverse_entry_slippage_over_1_percent")
		}
		review := models.TradeReview{
			JournalEntryID: uuid.New(), Version: 1, TradePlanID: &planID, PlanSnapshot: map[string]any{}, ExecutionSnapshot: map[string]any{},
			Diff: models.TradeExecutionDiff{PlanLinkStatus: "linked", KnownDeviations: deviations, UnknownComparisons: []string{}}, ProcessQuality: "followed", CompletedAt: &completedAt,
		}
		if err := database.Create(&review).Error; err != nil {
			t.Fatal(err)
		}
	}
	plan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusDraft, PlanDate: time.Now().UTC()}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	reviewer := &recordingPreTradeReviewer{}
	handler := NewPreTradeReviewHandler(database, reviewer, nil)
	router := gin.New()
	router.POST("/api/v1/trade-plans/:id/risk-reviews", handler.Create)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews", strings.NewReader(`{"run_ai":true,"llm_provider":"openai","model":"gpt-test"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if len(reviewer.input.BehaviorObservations) != 2 || reviewer.input.BehaviorObservations[0]["sample_count"] != 10 {
		t.Fatalf("expected eligible behavior observations, got %+v", reviewer.input.BehaviorObservations)
	}
}

func TestPreTradeReviewExcludesDisabledMemoryAndIncludesActivePrinciplesAndPlaybooks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.TradePlan{}, &models.PreTradeRiskReview{}, &models.RiskAcknowledgement{}, &models.TradeReview{}, &models.BehaviorObservation{}, &models.TradingProfile{}, &models.TradingPrinciple{}, &models.TradingPlaybook{}, &models.AIMemoryPreference{}); err != nil {
		t.Fatal(err)
	}
	plan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusDraft, PlanDate: time.Now().UTC()}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.AIMemoryPreference{ID: 1, PrinciplesEnabled: true, ObservationsEnabled: false, PlaybooksEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.TradingPrinciple{Category: models.PrincipleCategoryNoTrade, Content: "财报前不追价", Enabled: true, AIEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.TradingPrinciple{Category: models.PrincipleCategoryNoTrade, Content: "不得发送", Enabled: true, AIEnabled: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.TradingPlaybook{Name: "放量突破", Markets: []string{"US"}, Horizons: []string{"swing"}, Status: models.PlaybookStatusActive, Source: "user", EvidenceReviewIDs: []string{}, AIEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	reviewer := &recordingPreTradeReviewer{}
	router := gin.New()
	router.POST("/api/v1/trade-plans/:id/risk-reviews", NewPreTradeReviewHandler(database, reviewer, nil).Create)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/trade-plans/"+plan.ID.String()+"/risk-reviews", strings.NewReader(`{"run_ai":true,"llm_provider":"openai","model":"gpt-test"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if len(reviewer.input.TradingPrinciples) != 1 || reviewer.input.TradingPrinciples[0]["content"] != "财报前不追价" {
		t.Fatalf("principles = %+v", reviewer.input.TradingPrinciples)
	}
	if len(reviewer.input.TradingPlaybooks) != 1 || reviewer.input.TradingPlaybooks[0]["name"] != "放量突破" {
		t.Fatalf("playbooks = %+v", reviewer.input.TradingPlaybooks)
	}
	if len(reviewer.input.BehaviorObservations) != 0 {
		t.Fatalf("observations should be disabled: %+v", reviewer.input.BehaviorObservations)
	}
}
