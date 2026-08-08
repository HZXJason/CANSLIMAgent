package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupProfileTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.TradingProfile{}, &models.TradingPrinciple{}, &models.TradingPlaybook{}, &models.AIMemoryPreference{},
		&models.BehaviorObservation{}, &models.TradePlan{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.PreTradeRiskReview{},
	); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterProfileRoutes(router.Group("/api/v1"), NewProfileHandler(database, nil))
	return database, router
}

func performJSON(router *gin.Engine, method, path string, payload any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func TestProfilePersistsExplicitTradingSystemWithoutInventingDefaults(t *testing.T) {
	_, router := setupProfileTest(t)
	empty := performJSON(router, http.MethodGet, "/api/v1/profile", nil)
	if empty.Code != http.StatusOK || !bytes.Contains(empty.Body.Bytes(), []byte(`"configured":false`)) {
		t.Fatalf("empty profile = %d %s", empty.Code, empty.Body.String())
	}

	saved := performJSON(router, http.MethodPut, "/api/v1/profile", map[string]any{
		"primary_markets": []string{"US", "CN"}, "trading_horizons": []string{"intraday", "swing"},
		"max_risk_per_trade_percent": 1.25, "max_daily_loss_percent": 3.0, "max_open_positions": 4,
	})
	if saved.Code != http.StatusOK || !bytes.Contains(saved.Body.Bytes(), []byte(`"configured":true`)) || !bytes.Contains(saved.Body.Bytes(), []byte(`"max_open_positions":4`)) {
		t.Fatalf("saved profile = %d %s", saved.Code, saved.Body.String())
	}
}

func TestPrinciplesAndPlaybooksAreEditableAndArchivedRatherThanDeleted(t *testing.T) {
	_, router := setupProfileTest(t)
	principle := performJSON(router, http.MethodPost, "/api/v1/profile/principles", map[string]any{"category": "no_trade", "content": "重大数据公布前不追价"})
	if principle.Code != http.StatusCreated || !bytes.Contains(principle.Body.Bytes(), []byte(`"ai_enabled":true`)) {
		t.Fatalf("principle = %d %s", principle.Code, principle.Body.String())
	}

	playbook := performJSON(router, http.MethodPost, "/api/v1/profile/playbooks", map[string]any{
		"name": "放量突破", "markets": []string{"US"}, "horizons": []string{"swing"}, "entry_rules": "收盘突破且成交量确认",
	})
	if playbook.Code != http.StatusCreated {
		t.Fatalf("playbook = %d %s", playbook.Code, playbook.Body.String())
	}
	var envelope struct {
		Data models.TradingPlaybook `json:"data"`
	}
	if err := json.Unmarshal(playbook.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	archived := performJSON(router, http.MethodPatch, "/api/v1/profile/playbooks/"+envelope.Data.ID.String(), map[string]any{"status": "archived"})
	if archived.Code != http.StatusOK || !bytes.Contains(archived.Body.Bytes(), []byte(`"status":"archived"`)) {
		t.Fatalf("archived = %d %s", archived.Code, archived.Body.String())
	}
}

func TestProfileDashboardSeparatesProcessMetricsAndCurrencyOutcomes(t *testing.T) {
	database, router := setupProfileTest(t)
	now := time.Now().UTC()
	plan := models.TradePlan{ID: uuid.New(), Ticker: "AAPL", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: now, ExecutedAt: &now}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	entry := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now, Symbol: "AAPL", Side: models.TradeSideBuy, Quantity: 10, Price: 100, Amount: 1000, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &plan.ID, Source: "manual"}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	review := models.TradeReview{ID: uuid.New(), JournalEntryID: entry.ID, Version: 1, TradePlanID: &plan.ID, PlanSnapshot: map[string]any{}, ExecutionSnapshot: map[string]any{}, Diff: models.TradeExecutionDiff{PlanLinkStatus: "linked", KnownDeviations: []string{}, UnknownComparisons: []string{}}, ProcessQuality: "followed", CompletedAt: &now}
	if err := database.Create(&review).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.PreTradeRiskReview{ID: uuid.New(), TradePlanID: plan.ID, Version: 1, PlanSnapshot: map[string]any{}, HardConflicts: []models.PreTradeFinding{}, RiskWarnings: []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{}, AIStatus: models.PreTradeAIStatusNotRequested, CreatedAt: now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}

	response := performJSON(router, http.MethodGet, "/api/v1/profile/dashboard?range=30d", nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"planned_execution_rate":1`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"pretrade_review_rate":1`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"review_completion_rate":1`)) {
		t.Fatalf("dashboard = %d %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"currency":"USD"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"open_quantity":10`)) {
		t.Fatalf("outcomes missing coverage = %s", response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"trends"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"planned_execution_rate":1`)) {
		t.Fatalf("trend evidence missing = %s", response.Body.String())
	}
}

func TestProfileDashboardDoesNotCountRiskReviewCreatedAfterExecution(t *testing.T) {
	database, router := setupProfileTest(t)
	now := time.Now().UTC()
	plan := models.TradePlan{ID: uuid.New(), Ticker: "MSFT", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: now, ExecutedAt: &now}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	entry := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now, Symbol: "MSFT", Side: models.TradeSideBuy, Quantity: 1, Price: 100, Amount: 100, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &plan.ID, Source: "manual"}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	late := now.Add(time.Hour)
	if err := database.Create(&models.PreTradeRiskReview{ID: uuid.New(), CreatedAt: late, TradePlanID: plan.ID, Version: 1, PlanSnapshot: map[string]any{}, HardConflicts: []models.PreTradeFinding{}, RiskWarnings: []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{}, AIStatus: models.PreTradeAIStatusNotRequested}).Error; err != nil {
		t.Fatal(err)
	}
	response := performJSON(router, http.MethodGet, "/api/v1/profile/dashboard?range=30d", nil)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"pretrade_reviewed":0`)) {
		t.Fatalf("late review counted as pretrade: %s", response.Body.String())
	}
}

func TestAIMemoryCategorySwitchesAndObservationCorrectionPersist(t *testing.T) {
	database, router := setupProfileTest(t)
	aiEnabled := true
	observation := models.BehaviorObservation{Key: "sample", Scenario: "tests", Status: "eligible", SampleCount: 10, Confidence: .6, Message: "样本中出现重复偏差", EvidenceReviewIDs: []string{}, AIEnabled: &aiEnabled}
	if err := database.Create(&observation).Error; err != nil {
		t.Fatal(err)
	}
	preferences := performJSON(router, http.MethodPatch, "/api/v1/profile/ai-memory", map[string]any{"principles_enabled": false, "observations_enabled": true, "playbooks_enabled": false})
	if preferences.Code != http.StatusOK || !bytes.Contains(preferences.Body.Bytes(), []byte(`"principles_enabled":false`)) {
		t.Fatalf("preferences = %d %s", preferences.Code, preferences.Body.String())
	}
	corrected := performJSON(router, http.MethodPatch, "/api/v1/profile/behavior-observations/"+observation.ID.String(), map[string]any{"user_correction": "仅在追涨场景出现", "ai_enabled": false})
	if corrected.Code != http.StatusOK || !bytes.Contains(corrected.Body.Bytes(), []byte(`"user_correction":"仅在追涨场景出现"`)) || !bytes.Contains(corrected.Body.Bytes(), []byte(`"ai_enabled":false`)) {
		t.Fatalf("corrected = %d %s", corrected.Code, corrected.Body.String())
	}
}
