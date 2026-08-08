package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTradeReviewTest(t *testing.T) (*gorm.DB, *TradeReviewHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.TradePlan{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.BehaviorObservation{}); err != nil {
		t.Fatal(err)
	}
	handler := NewTradeReviewHandler(database)
	router := gin.New()
	RegisterTradeReviewRoutes(router.Group("/api/v1"), handler)
	return database, handler, router
}

func TestTradeReviewCreatesLinkedAndOffPlanSnapshots(t *testing.T) {
	database, _, router := setupTradeReviewTest(t)
	entryPrice := 100.0
	plan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: time.Now().UTC(), EntryPrice: &entryPrice}
	if err := database.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	entry := models.TradeJournalEntry{TradePlanID: &plan.ID, Symbol: "NVDA", Side: models.TradeSideBuy, Price: 102, Quantity: 10, Amount: 1020, Market: models.TradeMarketUS, Source: "trade_plan", ExecutedAt: time.Now().UTC()}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+entry.ID.String()+"/review", nil))
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"plan_link_status":"linked"`) || !strings.Contains(create.Body.String(), `"version":1`) {
		t.Fatalf("expected linked review, got %d: %s", create.Code, create.Body.String())
	}
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/journal/"+entry.ID.String()+"/review", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"entry_slippage_percent":2`) {
		t.Fatalf("expected persisted latest review, got %d: %s", get.Code, get.Body.String())
	}

	planless := models.TradeJournalEntry{Symbol: "AAPL", Side: models.TradeSideBuy, Price: 190, Quantity: 5, Amount: 950, Market: models.TradeMarketUS, Source: "broker", ExecutedAt: time.Now().UTC()}
	if err := database.Create(&planless).Error; err != nil {
		t.Fatal(err)
	}
	offPlan := httptest.NewRecorder()
	router.ServeHTTP(offPlan, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+planless.ID.String()+"/review", nil))
	if offPlan.Code != http.StatusCreated || !strings.Contains(offPlan.Body.String(), `"plan_link_status":"off_plan"`) {
		t.Fatalf("expected explicit off-plan review, got %d: %s", offPlan.Code, offPlan.Body.String())
	}
}

func TestTradeReviewExplicitPlanLinkRejectsMismatchAndAppendsVersion(t *testing.T) {
	database, _, router := setupTradeReviewTest(t)
	entry := models.TradeJournalEntry{Symbol: "NVDA", Side: models.TradeSideBuy, Price: 102, Quantity: 10, Amount: 1020, Market: models.TradeMarketUS, Source: "broker", ExecutedAt: time.Now().UTC()}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+entry.ID.String()+"/review", nil))

	badPlan := models.TradePlan{Ticker: "AAPL", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusWaiting, PlanDate: time.Now().UTC()}
	goodPlan := models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusWaiting, PlanDate: time.Now().UTC()}
	if err := database.Create(&badPlan).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&goodPlan).Error; err != nil {
		t.Fatal(err)
	}
	mismatch := httptest.NewRecorder()
	router.ServeHTTP(mismatch, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+entry.ID.String()+"/link-plan", strings.NewReader(`{"trade_plan_id":"`+badPlan.ID.String()+`"}`)))
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("expected symbol mismatch rejection, got %d: %s", mismatch.Code, mismatch.Body.String())
	}

	candidates := httptest.NewRecorder()
	router.ServeHTTP(candidates, httptest.NewRequest(http.MethodGet, "/api/v1/journal/"+entry.ID.String()+"/plan-candidates", nil))
	if candidates.Code != http.StatusOK || !strings.Contains(candidates.Body.String(), goodPlan.ID.String()) || strings.Contains(candidates.Body.String(), badPlan.ID.String()) {
		t.Fatalf("unexpected plan candidates, got %d: %s", candidates.Code, candidates.Body.String())
	}

	linked := httptest.NewRecorder()
	router.ServeHTTP(linked, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+entry.ID.String()+"/link-plan", strings.NewReader(`{"trade_plan_id":"`+goodPlan.ID.String()+`"}`)))
	if linked.Code != http.StatusCreated || !strings.Contains(linked.Body.String(), `"plan_link_status":"linked"`) || !strings.Contains(linked.Body.String(), `"version":2`) {
		t.Fatalf("expected linked version 2, got %d: %s", linked.Code, linked.Body.String())
	}
	var stored models.TradeJournalEntry
	if err := database.First(&stored, "id = ?", entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TradePlanID == nil || *stored.TradePlanID != goodPlan.ID {
		t.Fatalf("expected explicit plan link, got %+v", stored.TradePlanID)
	}
}

func TestTradeReviewUpdatesReflectionWithoutChangingSnapshots(t *testing.T) {
	database, _, router := setupTradeReviewTest(t)
	entry := models.TradeJournalEntry{Symbol: "NVDA", Side: models.TradeSideBuy, Price: 102, Quantity: 10, Amount: 1020, Market: models.TradeMarketUS, Source: "broker", ExecutedAt: time.Now().UTC()}
	if err := database.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/journal/"+entry.ID.String()+"/review", nil))

	patch := httptest.NewRecorder()
	body := `{"process_quality":"deviated","deviation_reason":"害怕错过而追价","emotion_before":"焦虑","emotion_after":"后悔","outcome_notes":"结果尚未闭环","lesson":"等待触发条件","complete":true}`
	router.ServeHTTP(patch, httptest.NewRequest(http.MethodPatch, "/api/v1/journal/"+entry.ID.String()+"/review", strings.NewReader(body)))
	if patch.Code != http.StatusOK || !strings.Contains(patch.Body.String(), `"process_quality":"deviated"`) || !strings.Contains(patch.Body.String(), `"outcome_notes":"结果尚未闭环"`) || !strings.Contains(patch.Body.String(), `"completed_at"`) {
		t.Fatalf("expected separate process/outcome reflection, got %d: %s", patch.Code, patch.Body.String())
	}
	if !strings.Contains(patch.Body.String(), `"price":102`) || !strings.Contains(patch.Body.String(), `"plan_link_status":"off_plan"`) {
		t.Fatalf("expected immutable execution snapshot and diff, got %s", patch.Body.String())
	}
}

func TestBehaviorObservationsRequireMinimumSampleAndPreserveFeedback(t *testing.T) {
	database, _, router := setupTradeReviewTest(t)
	createCompletedReview := func(adverse bool) {
		completedAt := time.Now().UTC()
		planID := uuid.New()
		deviations := []string{}
		if adverse {
			deviations = append(deviations, "adverse_entry_slippage_over_1_percent")
		}
		review := models.TradeReview{
			JournalEntryID: uuid.New(), Version: 1, TradePlanID: &planID, PlanSnapshot: map[string]any{}, ExecutionSnapshot: map[string]any{},
			Diff:           models.TradeExecutionDiff{PlanLinkStatus: "linked", KnownDeviations: deviations, UnknownComparisons: []string{}},
			ProcessQuality: "followed", CompletedAt: &completedAt,
		}
		if err := database.Create(&review).Error; err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 9; index++ {
		createCompletedReview(index < 3)
	}
	insufficient := httptest.NewRecorder()
	router.ServeHTTP(insufficient, httptest.NewRequest(http.MethodGet, "/api/v1/behavior-observations", nil))
	if insufficient.Code != http.StatusOK || !strings.Contains(insufficient.Body.String(), `"status":"insufficient"`) || !strings.Contains(insufficient.Body.String(), `"sample_count":9`) {
		t.Fatalf("expected insufficient observation, got %d: %s", insufficient.Code, insufficient.Body.String())
	}

	createCompletedReview(true)
	eligible := httptest.NewRecorder()
	router.ServeHTTP(eligible, httptest.NewRequest(http.MethodGet, "/api/v1/behavior-observations", nil))
	if eligible.Code != http.StatusOK || !strings.Contains(eligible.Body.String(), `"key":"adverse_entry_slippage"`) || !strings.Contains(eligible.Body.String(), `"sample_count":10`) {
		t.Fatalf("expected eligible evidence-backed observation, got %d: %s", eligible.Code, eligible.Body.String())
	}
	var observation models.BehaviorObservation
	if err := database.First(&observation, "key = ?", "adverse_entry_slippage").Error; err != nil {
		t.Fatal(err)
	}
	feedback := httptest.NewRecorder()
	router.ServeHTTP(feedback, httptest.NewRequest(http.MethodPost, "/api/v1/behavior-observations/"+observation.ID.String()+"/feedback", strings.NewReader(`{"feedback":"not_applicable"}`)))
	if feedback.Code != http.StatusOK || !strings.Contains(feedback.Body.String(), `"feedback":"not_applicable"`) || !strings.Contains(feedback.Body.String(), `"evidence_review_ids"`) {
		t.Fatalf("expected feedback without evidence deletion, got %d: %s", feedback.Code, feedback.Body.String())
	}
}
