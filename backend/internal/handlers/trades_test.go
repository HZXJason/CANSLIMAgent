package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTradeHandlerTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.User{}, &models.TradingProfile{}, &models.TradePlan{}, &models.Trade{},
		&models.TradeJournalEntry{}, &models.TradeReview{}, &models.PreTradeRiskReview{},
	); err != nil {
		t.Fatal(err)
	}
	user := models.DefaultUser()
	_ = database.Create(&user).Error
	equity := 100000.0
	stdRisk := 1.0
	_ = database.Create(&models.TradingProfile{
		UserID: models.DefaultUserID, Configured: true, PrimaryMarkets: []string{"US"}, TradingHorizons: []string{"swing"},
		AccountEquity: &equity, AccountCurrency: "USD", StandardRiskPercent: &stdRisk,
	}).Error

	router := gin.New()
	RegisterTradeRoutes(router.Group("/api/v1"), NewTradeHandler(database, nil))
	return database, router
}

func TestPositionSizerAndTradeLifecycleHandlers(t *testing.T) {
	database, router := setupTradeHandlerTest(t)

	// 1. Position sizer uses stored 1R profile
	sized := performJSON(router, http.MethodPost, "/api/v1/position-sizer", map[string]any{
		"symbol": "NVDA", "direction": "buy", "entry_price": 100, "stop_loss": 95, "risk_multiple": 1.0,
	})
	if sized.Code != http.StatusOK || !bytes.Contains(sized.Body.Bytes(), []byte(`"suggested_quantity":200`)) {
		t.Fatalf("position sizer = %d %s", sized.Code, sized.Body.String())
	}

	// 2. Seed an open fill & test stop-loss update + link-plan + review
	now := time.Now().UTC()
	entry := 100.0
	stop := 95.0
	plan := models.TradePlan{ID: uuid.New(), Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusWaiting, PlanDate: now, EntryPrice: &entry, StopLoss: &stop}
	_ = database.Create(&plan).Error
	fill := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now, Symbol: "NVDA", Side: models.TradeSideBuy, Quantity: 100, Price: 100, Amount: 10000, Market: models.TradeMarketUS, Currency: "USD", Source: "manual"}
	_ = database.Create(&fill).Error

	listResp := performJSON(router, http.MethodGet, "/api/v1/trades?status=open&market=US", nil)
	if listResp.Code != http.StatusOK || !bytes.Contains(listResp.Body.Bytes(), []byte(`"symbol":"NVDA"`)) {
		t.Fatalf("trades list = %d %s", listResp.Code, listResp.Body.String())
	}
	var listEnv struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(listResp.Body.Bytes(), &listEnv)
	tradeID := listEnv.Data[0].ID

	linked := performJSON(router, http.MethodPost, "/api/v1/trades/"+tradeID+"/link-plan", map[string]any{"trade_plan_id": plan.ID.String()})
	if linked.Code != http.StatusOK || !bytes.Contains(linked.Body.Bytes(), []byte(plan.ID.String())) {
		t.Fatalf("link-plan = %d %s", linked.Code, linked.Body.String())
	}

	trailed := performJSON(router, http.MethodPatch, "/api/v1/trades/"+tradeID+"/stop-loss", map[string]any{"stop_loss": 100.0})
	if trailed.Code != http.StatusOK || !bytes.Contains(trailed.Body.Bytes(), []byte(`"open_risk_amount":0`)) {
		t.Fatalf("trail stop = %d %s", trailed.Code, trailed.Body.String())
	}

	reviewed := performJSON(router, http.MethodPut, "/api/v1/trades/"+tradeID+"/review", map[string]any{"process_quality": "followed", "lesson": "严格移至保本"})
	if reviewed.Code != http.StatusOK || !bytes.Contains(reviewed.Body.Bytes(), []byte(`"process_quality":"followed"`)) {
		t.Fatalf("upsert review = %d %s", reviewed.Code, reviewed.Body.String())
	}
}
