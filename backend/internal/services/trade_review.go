package services

import (
	"math"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func BuildTradeExecutionDiff(plan *models.TradePlan, entry models.TradeJournalEntry) models.TradeExecutionDiff {
	diff := models.TradeExecutionDiff{
		PlanLinkStatus: "off_plan", TimingStatus: "unknown", KnownDeviations: []string{},
		UnknownComparisons: []string{"planned_quantity", "actual_exit", "stop_execution"},
	}
	if plan == nil {
		return diff
	}
	diff.PlanLinkStatus = "linked"
	symbolMatch := strings.EqualFold(strings.TrimSpace(plan.Ticker), strings.TrimSpace(entry.Symbol))
	diff.SymbolMatch = &symbolMatch
	directionMatch := (plan.Direction == models.TradePlanDirectionBuy && entry.Side == models.TradeSideBuy) ||
		(plan.Direction == models.TradePlanDirectionSell && entry.Side == models.TradeSideSell)
	diff.DirectionMatch = &directionMatch
	if !symbolMatch {
		diff.KnownDeviations = append(diff.KnownDeviations, "symbol_mismatch")
	}
	if !directionMatch {
		diff.KnownDeviations = append(diff.KnownDeviations, "direction_mismatch")
	}
	if plan.EntryPrice != nil && *plan.EntryPrice > 0 && entry.Price > 0 {
		slippage := (entry.Price - *plan.EntryPrice) / *plan.EntryPrice * 100
		if plan.Direction == models.TradePlanDirectionSell {
			slippage = (*plan.EntryPrice - entry.Price) / *plan.EntryPrice * 100
		}
		slippage = math.Round(slippage*10000) / 10000
		diff.EntrySlippagePercent = &slippage
		if slippage > 1 {
			diff.KnownDeviations = append(diff.KnownDeviations, "adverse_entry_slippage_over_1_percent")
		}
	} else {
		diff.UnknownComparisons = append(diff.UnknownComparisons, "planned_entry_price")
	}
	executionDate := entry.ExecutedAt.UTC().Truncate(24 * time.Hour)
	planDate := plan.PlanDate.UTC().Truncate(24 * time.Hour)
	if executionDate.Before(planDate) {
		diff.TimingStatus = "before_plan_date"
		diff.KnownDeviations = append(diff.KnownDeviations, "execution_before_plan_date")
	} else if plan.ValidUntil != nil && executionDate.After(plan.ValidUntil.UTC().Truncate(24*time.Hour)) {
		diff.TimingStatus = "after_valid_until"
		diff.KnownDeviations = append(diff.KnownDeviations, "execution_after_valid_until")
	} else {
		diff.TimingStatus = "within_plan_window"
	}
	return diff
}
