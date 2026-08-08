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
