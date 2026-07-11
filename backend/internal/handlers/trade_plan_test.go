package handlers

import (
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestCreateTradePlanRequestBuildsPlan(t *testing.T) {
	entryPrice := 125.5
	stopLoss := 119.25
	positionSize := 8.5
	status := models.TradePlanStatusWaiting
	validUntil := "2026-08-15"
	trigger := "突破 125 后成交量放大"

	plan, validationErr := createTradePlanRequest{
		Ticker:       " nvda ",
		Direction:    "BUY",
		Status:       &status,
		PlanDate:     "2026-07-05",
		ValidUntil:   &validUntil,
		EntryTrigger: &trigger,
		EntryPrice:   &entryPrice,
		StopLoss:     &stopLoss,
		PositionSize: &positionSize,
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

func TestExecuteTradePlanUsesPlanDefaults(t *testing.T) {
	entryPrice := 125.5
	quantity := 12.0
	plan := models.TradePlan{
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
