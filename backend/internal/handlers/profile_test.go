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
		&models.User{}, &models.TradingProfile{}, &models.TradingPrinciple{}, &models.TradingPlaybook{}, &models.TradingPolicy{}, &models.AIMemoryPreference{},
		&models.BehaviorObservation{},
		&models.Trade{}, &models.TradePlan{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.PreTradeRiskReview{}, &models.PromptContextSnapshot{},
	); err != nil {
		t.Fatal(err)
	}
	user := models.DefaultUser()
	if err := database.Create(&user).Error; err != nil {
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

func TestProfilePersistsDeclaredInvestmentStyleForDefaultUser(t *testing.T) {
	database, router := setupProfileTest(t)
	saved := performJSON(router, http.MethodPut, "/api/v1/profile", map[string]any{
		"primary_markets":                     []string{"US"},
		"asset_classes":                       []string{"equity", "etf"},
		"trading_horizons":                    []string{"swing"},
		"strategy_preferences":                []string{"momentum", "breakout"},
		"investment_objectives":               []string{"capital_growth"},
		"risk_tolerance":                      "medium",
		"max_portfolio_concentration_percent": 25,
		"leverage_allowed":                    false,
	})
	if saved.Code != http.StatusOK {
		t.Fatalf("saved profile = %d %s", saved.Code, saved.Body.String())
	}
	for _, fragment := range []string{
		`"user_id":"` + models.DefaultUserID.String() + `"`,
		`"asset_classes":["equity","etf"]`,
		`"strategy_preferences":["momentum","breakout"]`,
		`"investment_objectives":["capital_growth"]`,
		`"risk_tolerance":"medium"`,
		`"max_portfolio_concentration_percent":25`,
		`"leverage_allowed":false`,
	} {
		if !bytes.Contains(saved.Body.Bytes(), []byte(fragment)) {
			t.Fatalf("profile response missing %s: %s", fragment, saved.Body.String())
		}
	}
	var stored models.TradingProfile
	if err := database.Where("user_id = ?", models.DefaultUserID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UserID != models.DefaultUserID || stored.RiskTolerance == nil || *stored.RiskTolerance != "medium" {
		t.Fatalf("unexpected stored profile: %+v", stored)
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

func TestTradingPoliciesKeepRuleAndDisciplineSourcesFlexible(t *testing.T) {
	_, router := setupProfileTest(t)
	manual := performJSON(router, http.MethodPost, "/api/v1/profile/policies", map[string]any{
		"kind": "rule", "category": "entry", "title": "突破确认", "statement": "收盘突破并放量后才允许入场",
		"strength": "hard", "origin": "manual", "status": "active", "prompt_enabled": true,
		"scope": map[string]any{"markets": []string{"US"}},
	})
	if manual.Code != http.StatusCreated || !bytes.Contains(manual.Body.Bytes(), []byte(`"status":"active"`)) {
		t.Fatalf("manual policy = %d %s", manual.Code, manual.Body.String())
	}
	aiDraft := performJSON(router, http.MethodPost, "/api/v1/profile/policies", map[string]any{
		"kind": "discipline", "category": "process", "title": "连续亏损暂停", "statement": "连续亏损两笔后停止当日交易",
		"strength": "hard", "origin": "ai_suggested", "status": "active", "prompt_enabled": true,
	})
	if aiDraft.Code != http.StatusCreated || !bytes.Contains(aiDraft.Body.Bytes(), []byte(`"status":"draft"`)) {
		t.Fatalf("AI policy should remain draft = %d %s", aiDraft.Code, aiDraft.Body.String())
	}
	listed := performJSON(router, http.MethodGet, "/api/v1/profile/policies", nil)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"kind":"rule"`)) || !bytes.Contains(listed.Body.Bytes(), []byte(`"kind":"discipline"`)) {
		t.Fatalf("policies = %d %s", listed.Code, listed.Body.String())
	}
}

func TestProfileContextComposesUserOwnedDataAndExportV2(t *testing.T) {
	database, router := setupProfileTest(t)
	policy := models.TradingPolicy{Kind: models.PolicyKindRule, Category: "risk", Title: "单笔风险", Statement: "不超过预算", Strength: models.PolicyStrengthHard, Origin: models.PolicyOriginManual, Status: models.PolicyStatusActive, PromptEnabled: true}
	if err := database.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.TradePlan{Ticker: "AAPL", Direction: models.TradePlanDirectionWatch, Status: models.TradePlanStatusWaiting, PlanDate: time.Now().UTC()}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	contextResponse := performJSON(router, http.MethodGet, "/api/v1/profile/context", nil)
	if contextResponse.Code != http.StatusOK {
		t.Fatalf("context = %d %s", contextResponse.Code, contextResponse.Body.String())
	}
	for _, fragment := range []string{
		`"version":"alphalens.user-trading-context.v2"`,
		`"id":"` + models.DefaultUserID.String() + `"`,
		`"policies":[{`,
		`"trade_plans":1`,
	} {
		if !bytes.Contains(contextResponse.Body.Bytes(), []byte(fragment)) {
			t.Fatalf("context missing %s: %s", fragment, contextResponse.Body.String())
		}
	}
	exported := performJSON(router, http.MethodGet, "/api/v1/profile/export", nil)
	if exported.Code != http.StatusOK || !bytes.Contains(exported.Body.Bytes(), []byte(`"schema_version":"alphalens.personal-trading-system.v2"`)) || !bytes.Contains(exported.Body.Bytes(), []byte(`"user"`)) || !bytes.Contains(exported.Body.Bytes(), []byte(`"policies"`)) {
		t.Fatalf("export = %d %s", exported.Code, exported.Body.String())
	}
}

func TestProfileExportUsesSnakeCaseForPlansAndExecutionFills(t *testing.T) {
	database, router := setupProfileTest(t)
	now := time.Now().UTC()
	plan := models.TradePlan{Ticker: "LB", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: now}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	entry := models.TradeJournalEntry{ExecutedAt: now, Symbol: "LB", Side: models.TradeSideBuy, Quantity: 1, Price: 80, Amount: 80, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &plan.ID, Source: "trade_plan"}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}

	exported := performJSON(router, http.MethodGet, "/api/v1/profile/export", nil)
	if exported.Code != http.StatusOK {
		t.Fatalf("export = %d %s", exported.Code, exported.Body.String())
	}
	var response struct {
		Data struct {
			TradePlans     []map[string]any `json:"trade_plans"`
			ExecutionFills []map[string]any `json:"execution_fills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(exported.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.TradePlans) != 1 || response.Data.TradePlans[0]["id"] == nil || response.Data.TradePlans[0]["plan_date"] == nil {
		t.Fatalf("trade plan export is not snake_case: %#v", response.Data.TradePlans)
	}
	if len(response.Data.ExecutionFills) != 1 || response.Data.ExecutionFills[0]["id"] == nil || response.Data.ExecutionFills[0]["trade_plan_id"] == nil || response.Data.ExecutionFills[0]["executed_at"] == nil {
		t.Fatalf("execution export is not snake_case: %#v", response.Data.ExecutionFills)
	}
}

func TestProfilePersistsOneRRiskAndHeatParameters(t *testing.T) {
	_, router := setupProfileTest(t)
	saved := performJSON(router, http.MethodPut, "/api/v1/profile", map[string]any{
		"primary_markets":            []string{"US"},
		"trading_horizons":           []string{"swing"},
		"account_equity":             100000,
		"account_currency":           "USD",
		"standard_risk_percent":      1.0,
		"max_portfolio_heat_percent": 5.0,
	})
	if saved.Code != http.StatusOK || !bytes.Contains(saved.Body.Bytes(), []byte(`"account_equity":100000`)) || !bytes.Contains(saved.Body.Bytes(), []byte(`"standard_risk_percent":1`)) || !bytes.Contains(saved.Body.Bytes(), []byte(`"max_portfolio_heat_percent":5`)) {
		t.Fatalf("saved 1R profile = %d %s", saved.Code, saved.Body.String())
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
