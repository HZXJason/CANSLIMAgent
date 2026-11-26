package services

import (
	"context"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRebuildUserTradesComputesRAndFourQuadrants(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.TradePlan{}, &models.Trade{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.PreTradeRiskReview{}); err != nil {
		t.Fatal(err)
	}
	user := models.DefaultUser()
	_ = database.Create(&user).Error

	now := time.Now().UTC()
	entry := 100.0
	stop := 95.0
	// Plan 1: Good loss (-1.0R)
	p1 := models.TradePlan{ID: uuid.New(), Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: now, EntryPrice: &entry, StopLoss: &stop}
	_ = database.Create(&p1).Error
	_ = database.Create(&models.PreTradeRiskReview{TradePlanID: p1.ID, Version: 1, PlanSnapshot: map[string]any{}, HardConflicts: []models.PreTradeFinding{}, RiskWarnings: []models.PreTradeFinding{}, BehaviorReminders: []models.PreTradeFinding{}, UnresolvedQuestions: []models.PreTradeFinding{}, ChangesSinceResearch: []models.PreTradeFinding{}, Freshness: map[string]any{}, Sources: []models.PreTradeSource{}, AIStatus: "not_requested"}).Error

	e1Buy := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now.Add(-48 * time.Hour), Symbol: "NVDA", Side: models.TradeSideBuy, Quantity: 100, Price: 100, Amount: 10000, Fee: 0, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &p1.ID, Source: "trade_plan"}
	e1Sell := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now.Add(-24 * time.Hour), Symbol: "NVDA", Side: models.TradeSideSell, Quantity: 100, Price: 95, Amount: 9500, Fee: 0, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &p1.ID, Source: "manual"}
	_ = database.Create(&e1Buy).Error
	_ = database.Create(&e1Sell).Error
	completedAt := now
	_ = database.Create(&models.TradeReview{JournalEntryID: e1Buy.ID, Version: 1, TradePlanID: &p1.ID, PlanSnapshot: map[string]any{}, ExecutionSnapshot: map[string]any{}, Diff: models.TradeExecutionDiff{PlanLinkStatus: "linked", KnownDeviations: []string{}, UnknownComparisons: []string{}}, ProcessQuality: "followed", CompletedAt: &completedAt}).Error

	// Off-plan Toxic Win (AAPL bought without plan, closed at profit)
	e2Buy := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now.Add(-12 * time.Hour), Symbol: "AAPL", Side: models.TradeSideBuy, Quantity: 10, Price: 200, Amount: 2000, Fee: 0, Market: models.TradeMarketUS, Currency: "USD", Source: "manual"}
	e2Sell := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now.Add(-6 * time.Hour), Symbol: "AAPL", Side: models.TradeSideSell, Quantity: 10, Price: 210, Amount: 2100, Fee: 0, Market: models.TradeMarketUS, Currency: "USD", Source: "manual"}
	_ = database.Create(&e2Buy).Error
	_ = database.Create(&e2Sell).Error

	// Open position (MSFT)
	p3 := models.TradePlan{ID: uuid.New(), Ticker: "MSFT", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusExecuted, PlanDate: now, EntryPrice: &entry, StopLoss: &stop}
	_ = database.Create(&p3).Error
	e3Buy := models.TradeJournalEntry{ID: uuid.New(), ExecutedAt: now.Add(-2 * time.Hour), Symbol: "MSFT", Side: models.TradeSideBuy, Quantity: 50, Price: 100, Amount: 5000, Fee: 0, Market: models.TradeMarketUS, Currency: "USD", TradePlanID: &p3.ID, Source: "trade_plan"}
	_ = database.Create(&e3Buy).Error

	trades, err := RebuildUserTrades(context.Background(), database, models.DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 3 {
		t.Fatalf("expected 3 trades, got %d", len(trades))
	}

	var nvda, aapl, msft models.Trade
	for _, tr := range trades {
		switch tr.Symbol {
		case "NVDA":
			nvda = tr
		case "AAPL":
			aapl = tr
		case "MSFT":
			msft = tr
		}
	}
	if nvda.Quadrant != models.TradeQuadrantGoodLoss || nvda.RealizedR == nil || *nvda.RealizedR != -1.0 {
		t.Fatalf("expected NVDA good_loss -1.0R, got %+v", nvda)
	}
	if aapl.Quadrant != models.TradeQuadrantToxicWin {
		t.Fatalf("expected AAPL toxic_win, got %+v", aapl)
	}
	if msft.Status != models.TradeStatusOpen || msft.OpenRiskAmount != 250 {
		t.Fatalf("expected MSFT open risk 250, got %+v", msft)
	}

	var fills []models.TradeJournalEntry
	var reviews []models.TradeReview
	var pretrade []models.PreTradeRiskReview
	_ = database.Find(&fills).Error
	_ = database.Find(&reviews).Error
	_ = database.Find(&pretrade).Error
	eq := 100000.0
	sr := 1.0
	summary := ComputeTradesSummary(trades, models.TradingProfile{AccountEquity: &eq, StandardRiskPercent: &sr}, fills, reviews, pretrade)
	if summary.GoodLosses != 1 || summary.ToxicWins != 1 || summary.DisciplinedRLoopRate != 50 || summary.OpenPortfolioRisk != 250 {
		t.Fatalf("unexpected trades summary: %+v", summary)
	}
}
