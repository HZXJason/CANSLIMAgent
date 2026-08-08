package models

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTradeReviewPersistsImmutableSnapshotsAndBehaviorEvidence(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&TradeReview{}, &BehaviorObservation{}); err != nil {
		t.Fatal(err)
	}
	planID := uuid.New()
	review := TradeReview{
		JournalEntryID: uuid.New(), Version: 1, TradePlanID: &planID,
		PlanSnapshot: map[string]any{"entry_price": 100.0}, ExecutionSnapshot: map[string]any{"price": 102.0},
		Diff:           TradeExecutionDiff{PlanLinkStatus: "linked", UnknownComparisons: []string{"actual_exit"}, KnownDeviations: []string{}},
		ProcessQuality: "unreviewed",
	}
	if err := database.Create(&review).Error; err != nil {
		t.Fatal(err)
	}
	observation := BehaviorObservation{
		Key: "adverse_entry_slippage", Scenario: "linked_entries", Status: "eligible", SampleCount: 10, Confidence: 0.7,
		Message: "10 个完整样本中存在多次不利滑点。", EvidenceReviewIDs: []string{review.ID.String()},
	}
	if err := database.Create(&observation).Error; err != nil {
		t.Fatal(err)
	}
	if review.ID == uuid.Nil || observation.ID == uuid.Nil || observation.EvidenceReviewIDs[0] != review.ID.String() {
		t.Fatalf("expected stable review and evidence identities: review=%+v observation=%+v", review, observation)
	}
}
