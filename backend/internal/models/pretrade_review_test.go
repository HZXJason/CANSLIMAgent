package models

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPreTradeReviewsAppendVersionsAndPreserveAcknowledgements(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PreTradeRiskReview{}, &RiskAcknowledgement{}); err != nil {
		t.Fatal(err)
	}
	planID := uuid.New()
	first := PreTradeRiskReview{
		TradePlanID: planID, Version: 1, PlanSnapshot: map[string]any{"ticker": "NVDA"},
		HardConflicts: []PreTradeFinding{{Code: "missing_stop_loss", Severity: "high", Evidence: map[string]any{}}},
		RiskWarnings:  []PreTradeFinding{}, BehaviorReminders: []PreTradeFinding{}, UnresolvedQuestions: []PreTradeFinding{}, ChangesSinceResearch: []PreTradeFinding{},
		Freshness: map[string]any{}, Sources: []PreTradeSource{}, AIStatus: PreTradeAIStatusUnavailable,
	}
	second := first
	second.ID = uuid.Nil
	second.Version = 2
	second.HardConflicts = []PreTradeFinding{}
	if err := database.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	acknowledgement := RiskAcknowledgement{ReviewID: first.ID, FindingCode: "missing_stop_loss"}
	if err := database.Create(&acknowledgement).Error; err != nil {
		t.Fatal(err)
	}

	var reviews []PreTradeRiskReview
	if err := database.Where("trade_plan_id = ?", planID).Order("version ASC").Find(&reviews).Error; err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 2 || reviews[0].Version != 1 || reviews[1].Version != 2 {
		t.Fatalf("expected two immutable versions, got %+v", reviews)
	}
	if len(reviews[0].HardConflicts) != 1 || reviews[0].HardConflicts[0].Code != "missing_stop_loss" {
		t.Fatalf("expected version 1 findings to remain, got %+v", reviews[0].HardConflicts)
	}
	if acknowledgement.ID == uuid.Nil || acknowledgement.ReviewID != reviews[0].ID {
		t.Fatalf("expected stable acknowledgement identity, got %+v", acknowledgement)
	}
}
