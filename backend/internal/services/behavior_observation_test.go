package services

import (
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/google/uuid"
)

func TestBuildBehaviorObservationsRequiresTenCompleteLinkedReviews(t *testing.T) {
	reviews := completeLinkedReviews(9, 4)
	observations := BuildBehaviorObservations(reviews)
	if len(observations) != 1 || observations[0].Status != "insufficient" || observations[0].SampleCount != 9 {
		t.Fatalf("expected insufficient sample observation, got %+v", observations)
	}
	if strings.Contains(observations[0].Message, "冲动型") || strings.Contains(observations[0].Message, "交易风格") {
		t.Fatalf("insufficient samples must not become a stable label: %s", observations[0].Message)
	}
}

func TestBuildBehaviorObservationsUsesEvidenceAtTenSamples(t *testing.T) {
	reviews := completeLinkedReviews(10, 4)
	observations := BuildBehaviorObservations(reviews)
	var slippage *models.BehaviorObservation
	for index := range observations {
		if observations[index].Key == "adverse_entry_slippage" {
			slippage = &observations[index]
		}
	}
	if slippage == nil || slippage.Status != "eligible" || slippage.SampleCount != 10 || len(slippage.EvidenceReviewIDs) != 4 || slippage.Confidence <= 0 {
		t.Fatalf("expected evidence-backed slippage observation, got %+v", slippage)
	}
	if strings.Contains(slippage.Message, "冲动型") {
		t.Fatalf("behavior evidence must not become a personality label: %s", slippage.Message)
	}
}

func completeLinkedReviews(count, adverseCount int) []models.TradeReview {
	reviews := make([]models.TradeReview, 0, count)
	for index := 0; index < count; index++ {
		completedAt := time.Now().UTC()
		deviations := []string{}
		if index < adverseCount {
			deviations = append(deviations, "adverse_entry_slippage_over_1_percent")
		}
		reviews = append(reviews, models.TradeReview{
			ID: uuid.New(), TradePlanID: uuidPointerForTest(uuid.New()), CompletedAt: &completedAt, ProcessQuality: "followed",
			Diff: models.TradeExecutionDiff{PlanLinkStatus: "linked", KnownDeviations: deviations, UnknownComparisons: []string{}},
		})
	}
	return reviews
}

func uuidPointerForTest(value uuid.UUID) *uuid.UUID { return &value }
