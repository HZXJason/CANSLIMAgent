package services

import (
	"fmt"
	"math"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

const MinimumBehaviorReviewSample = 10

func BuildBehaviorObservations(reviews []models.TradeReview) []models.BehaviorObservation {
	complete := make([]models.TradeReview, 0, len(reviews))
	for _, review := range reviews {
		if review.CompletedAt != nil && review.TradePlanID != nil && review.Diff.PlanLinkStatus == "linked" {
			complete = append(complete, review)
		}
	}
	allEvidence := reviewIDs(complete)
	if len(complete) < MinimumBehaviorReviewSample {
		return []models.BehaviorObservation{{
			Key: "review_sample_readiness", Scenario: "complete_linked_trade_reviews", Status: "insufficient",
			SampleCount: len(complete), Confidence: 0,
			Message:           fmt.Sprintf("当前只有 %d 个完整且已关联计划的复盘样本；至少需要 %d 个后才生成稳定行为观察。", len(complete), MinimumBehaviorReviewSample),
			EvidenceReviewIDs: allEvidence,
			AIEnabled:         boolPointer(true),
		}}
	}

	adverse := filterReviewIDs(complete, func(review models.TradeReview) bool {
		return containsReviewDeviation(review.Diff.KnownDeviations, "adverse_entry_slippage_over_1_percent")
	})
	deviated := filterReviewIDs(complete, func(review models.TradeReview) bool { return review.ProcessQuality == "deviated" })
	confidence := math.Min(0.9, 0.5+float64(len(complete))/100)
	return []models.BehaviorObservation{
		{
			Key: "adverse_entry_slippage", Scenario: "complete_linked_trade_reviews", Status: "eligible",
			SampleCount: len(complete), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 个完整样本中，有 %d 次入场价格相对计划出现超过 1%% 的不利偏差。", len(complete), len(adverse)),
			EvidenceReviewIDs: adverse,
			AIEnabled:         boolPointer(true),
		},
		{
			Key: "self_reported_plan_deviation", Scenario: "complete_linked_trade_reviews", Status: "eligible",
			SampleCount: len(complete), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 个完整样本中，有 %d 次被用户记录为偏离原计划。", len(complete), len(deviated)),
			EvidenceReviewIDs: deviated,
			AIEnabled:         boolPointer(true),
		},
	}
}

func boolPointer(value bool) *bool { return &value }

func reviewIDs(reviews []models.TradeReview) []string {
	ids := make([]string, 0, len(reviews))
	for _, review := range reviews {
		ids = append(ids, review.ID.String())
	}
	return ids
}

func filterReviewIDs(reviews []models.TradeReview, predicate func(models.TradeReview) bool) []string {
	ids := []string{}
	for _, review := range reviews {
		if predicate(review) {
			ids = append(ids, review.ID.String())
		}
	}
	return ids
}

func containsReviewDeviation(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func BuildTradeBehaviorObservations(trades []models.Trade) []models.BehaviorObservation {
	closed := make([]models.Trade, 0, len(trades))
	for _, tr := range trades {
		if tr.Status == models.TradeStatusClosed {
			closed = append(closed, tr)
		}
	}
	if len(closed) < MinimumBehaviorReviewSample {
		return nil
	}
	confidence := math.Min(0.9, 0.5+float64(len(closed))/100)
	stopViolations, chaseEntries, toxicWins, offPlans := []string{}, []string{}, []string{}, []string{}
	for _, tr := range closed {
		id := tr.ID.String()
		if hasViolationCode(tr.AutoViolations, "stop_loss_violated") {
			stopViolations = append(stopViolations, id)
		}
		if hasViolationCode(tr.AutoViolations, "entry_chase_over_0_25r") {
			chaseEntries = append(chaseEntries, id)
		}
		if hasViolationCode(tr.AutoViolations, "off_plan_unlinked") {
			offPlans = append(offPlans, id)
		}
		if tr.Quadrant == models.TradeQuadrantToxicWin {
			toxicWins = append(toxicWins, id)
		}
	}
	return []models.BehaviorObservation{
		{
			Key: "stop_loss_violation_rate", Scenario: "closed_trades_r_lifecycle", Status: "eligible",
			SampleCount: len(closed), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 笔完结仓位中，有 %d 笔出现扛单超损（Realized R < -1.15R）。", len(closed), len(stopViolations)),
			EvidenceReviewIDs: stopViolations,
			AIEnabled:         boolPointer(true),
		},
		{
			Key: "entry_chase_over_0_25r", Scenario: "closed_trades_r_lifecycle", Status: "eligible",
			SampleCount: len(closed), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 笔完结仓位中，有 %d 笔入场滑点超过 0.25R（追高入场）。", len(closed), len(chaseEntries)),
			EvidenceReviewIDs: chaseEntries,
			AIEnabled:         boolPointer(true),
		},
		{
			Key: "toxic_win_warning", Scenario: "closed_trades_r_lifecycle", Status: "eligible",
			SampleCount: len(closed), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 笔完结仓位中，有 %d 笔属于“坏过程但盈利”的毒性盈利（Toxic Win），警惕强化错误习惯。", len(closed), len(toxicWins)),
			EvidenceReviewIDs: toxicWins,
			AIEnabled:         boolPointer(true),
		},
		{
			Key: "off_plan_execution_rate", Scenario: "closed_trades_r_lifecycle", Status: "eligible",
			SampleCount: len(closed), Confidence: confidence,
			Message:           fmt.Sprintf("在 %d 笔完结仓位中，有 %d 笔未关联交易计划（计划外开仓）。", len(closed), len(offPlans)),
			EvidenceReviewIDs: offPlans,
			AIEnabled:         boolPointer(true),
		},
	}
}
