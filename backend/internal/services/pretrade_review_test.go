package services

import (
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestBuildDeterministicPreTradeReviewFindsHardConflictsWithoutAI(t *testing.T) {
	entry := 100.0
	stop := 105.0
	target := 95.0
	planDate := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	validUntil := planDate.AddDate(0, 0, -1)
	plan := models.TradePlan{
		Ticker:     "NVDA",
		Direction:  models.TradePlanDirectionBuy,
		PlanDate:   planDate,
		ValidUntil: &validUntil,
		EntryPrice: &entry,
		StopLoss:   &stop,
		TakeProfit: &target,
	}

	result := BuildDeterministicPreTradeReview(plan)
	codes := findingCodes(result.HardConflicts)
	for _, expected := range []string{"buy_stop_not_below_entry", "buy_target_not_above_entry", "valid_until_before_plan_date"} {
		if !codes[expected] {
			t.Fatalf("expected hard conflict %q, got %+v", expected, result.HardConflicts)
		}
	}
	warningCodes := findingCodes(result.RiskWarnings)
	for _, expected := range []string{"missing_entry_trigger", "missing_invalidation", "missing_risk_budget"} {
		if !warningCodes[expected] {
			t.Fatalf("expected risk warning %q, got %+v", expected, result.RiskWarnings)
		}
	}
}

func TestBuildDeterministicPreTradeReviewFlagsWeakRewardRisk(t *testing.T) {
	entry := 100.0
	stop := 95.0
	target := 104.0
	maxRisk := 1.0
	trigger := "放量突破后回踩"
	invalidation := "跌破 95"
	plan := models.TradePlan{
		Ticker:         "NVDA",
		Direction:      models.TradePlanDirectionBuy,
		PlanDate:       time.Now().UTC(),
		EntryTrigger:   &trigger,
		EntryPrice:     &entry,
		StopLoss:       &stop,
		TakeProfit:     &target,
		Invalidation:   &invalidation,
		MaxRiskPercent: &maxRisk,
	}

	result := BuildDeterministicPreTradeReview(plan)
	if !findingCodes(result.RiskWarnings)["reward_risk_below_1_5"] {
		t.Fatalf("expected weak reward-risk warning, got %+v", result.RiskWarnings)
	}
	if len(result.HardConflicts) != 0 {
		t.Fatalf("expected no hard conflicts, got %+v", result.HardConflicts)
	}
}

func TestBuildPersonalizedPreTradeReviewRejectsRiskAboveExplicitProfileLimit(t *testing.T) {
	risk := 2.0
	limit := 1.0
	plan := models.TradePlan{MaxRiskPercent: &risk}
	profile := models.TradingProfile{Configured: true, MaxRiskPerTradePercent: &limit}
	result := BuildPersonalizedPreTradeReview(plan, profile)
	found := false
	for _, finding := range result.HardConflicts {
		if finding.Code == "personal_risk_limit_exceeded" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected personal risk conflict, got %+v", result.HardConflicts)
	}
}

func findingCodes(findings []models.PreTradeFinding) map[string]bool {
	result := make(map[string]bool, len(findings))
	for _, finding := range findings {
		result[finding.Code] = true
	}
	return result
}
