package services

import (
	"context"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildUserTradingContextSelectsOnlyPromptEligiblePolicies(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.User{}, &models.TradingProfile{}, &models.TradingPolicy{}, &models.TradingPrinciple{}, &models.TradingPlaybook{},
		&models.BehaviorObservation{}, &models.TradePlan{}, &models.Trade{}, &models.TradeJournalEntry{}, &models.TradeReview{},
	); err != nil {
		t.Fatal(err)
	}
	user := models.DefaultUser()
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	equity := 100000.0
	riskPct := 1.0
	profile := models.TradingProfile{
		UserID: user.ID, Configured: true, PrimaryMarkets: []string{"US"}, AssetClasses: []string{}, TradingHorizons: []string{"swing"},
		StrategyPreferences: []string{}, InvestmentObjectives: []string{}, AccountEquity: &equity, AccountCurrency: "USD", StandardRiskPercent: &riskPct,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	active := models.TradingPolicy{UserID: user.ID, Kind: models.PolicyKindRule, Category: "risk", Title: "单笔风险", Statement: "不超过风险预算", Strength: models.PolicyStrengthHard, Origin: models.PolicyOriginManual, Status: models.PolicyStatusActive, PromptEnabled: true}
	draft := models.TradingPolicy{UserID: user.ID, Kind: models.PolicyKindDiscipline, Category: "process", Title: "暂停", Statement: "连续亏损后暂停", Strength: models.PolicyStrengthReminder, Origin: models.PolicyOriginManual, Status: models.PolicyStatusDraft, PromptEnabled: true}
	if err := database.Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&draft).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contextValue, err := BuildUserTradingContext(context.Background(), database, user.ID, "pretrade_review", now)
	if err != nil {
		t.Fatal(err)
	}
	if contextValue.Version != UserTradingContextV2 || len(contextValue.Policies) != 1 || contextValue.Policies[0].ID != active.ID {
		t.Fatalf("unexpected policies in context: %+v", contextValue.Policies)
	}
	if contextValue.DeclaredProfile.AccountEquity == nil || *contextValue.DeclaredProfile.AccountEquity != 100000 {
		t.Fatalf("unexpected profile in context: %+v", contextValue.DeclaredProfile)
	}
}
