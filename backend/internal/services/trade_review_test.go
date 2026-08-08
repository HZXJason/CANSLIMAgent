package services

import (
	"math"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestBuildTradeExecutionDiffSeparatesKnownAndUnknownComparisons(t *testing.T) {
	entryPrice := 100.0
	stop := 95.0
	target := 112.0
	planDate := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	validUntil := planDate.AddDate(0, 0, 1)
	plan := models.TradePlan{
		Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, PlanDate: planDate, ValidUntil: &validUntil,
		EntryPrice: &entryPrice, StopLoss: &stop, TakeProfit: &target,
	}
	entry := models.TradeJournalEntry{
		Symbol: "NVDA", Side: models.TradeSideBuy, Price: 102, Quantity: 10,
		ExecutedAt: time.Date(2026, 8, 4, 14, 30, 0, 0, time.UTC),
	}

	diff := BuildTradeExecutionDiff(&plan, entry)
	if diff.PlanLinkStatus != "linked" || diff.DirectionMatch == nil || !*diff.DirectionMatch {
		t.Fatalf("unexpected identity comparison: %+v", diff)
	}
	if diff.EntrySlippagePercent == nil || math.Abs(*diff.EntrySlippagePercent-2) > 0.0001 {
		t.Fatalf("expected 2%% adverse slippage, got %+v", diff.EntrySlippagePercent)
	}
	if diff.TimingStatus != "within_plan_window" {
		t.Fatalf("expected execution within plan window, got %q", diff.TimingStatus)
	}
	for _, field := range []string{"planned_quantity", "actual_exit", "stop_execution"} {
		if !containsString(diff.UnknownComparisons, field) {
			t.Fatalf("expected %q to remain unknown, got %+v", field, diff.UnknownComparisons)
		}
	}
}

func TestBuildTradeExecutionDiffMarksPlanlessExecutions(t *testing.T) {
	entry := models.TradeJournalEntry{Symbol: "NVDA", Side: models.TradeSideBuy, Price: 102, Quantity: 10, ExecutedAt: time.Now().UTC()}
	diff := BuildTradeExecutionDiff(nil, entry)
	if diff.PlanLinkStatus != "off_plan" || diff.DirectionMatch != nil || diff.EntrySlippagePercent != nil {
		t.Fatalf("expected explicit off-plan unknowns, got %+v", diff)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
