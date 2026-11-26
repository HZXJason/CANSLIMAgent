package models

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAISuggestedTradingPolicyCannotActivateOnCreate(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&TradingPolicy{}); err != nil {
		t.Fatal(err)
	}
	policy := TradingPolicy{
		Kind: PolicyKindDiscipline, Category: "process", Title: "停止交易",
		Statement: "连续亏损两笔后停止当日交易", Strength: PolicyStrengthHard,
		Origin: PolicyOriginAISuggested, Status: PolicyStatusActive, PromptEnabled: true,
		Scope: map[string]any{}, Condition: map[string]any{}, Action: map[string]any{},
	}
	if err := database.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if policy.UserID != DefaultUserID || policy.Status != PolicyStatusDraft || policy.Version != 1 {
		t.Fatalf("unexpected AI policy defaults: %+v", policy)
	}
}

func TestTradingPolicyPromptEligibilityRequiresActiveEnabledPolicy(t *testing.T) {
	policy := TradingPolicy{Status: PolicyStatusActive, PromptEnabled: true}
	if !policy.PromptEligible() {
		t.Fatal("active enabled policy should be prompt eligible")
	}
	policy.Status = PolicyStatusDraft
	if policy.PromptEligible() {
		t.Fatal("draft policy must not be prompt eligible")
	}
}
