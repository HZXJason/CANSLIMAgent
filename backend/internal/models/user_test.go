package models

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDefaultUserIdentityIsStable(t *testing.T) {
	first := DefaultUser()
	second := DefaultUser()
	if first.ID == uuid.Nil || first.ID != DefaultUserID || second.ID != first.ID {
		t.Fatalf("default user identity is not stable: first=%s second=%s constant=%s", first.ID, second.ID, DefaultUserID)
	}
	if first.Status != UserStatusActive || first.Timezone != "Asia/Shanghai" || first.BaseCurrency != "USD" {
		t.Fatalf("unexpected default user: %+v", first)
	}
}

func TestPersonalRecordsDefaultToLocalUser(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&User{}, &TradingProfile{}, &TradingPrinciple{}, &BehaviorObservation{}); err != nil {
		t.Fatal(err)
	}
	profile := TradingProfile{Configured: true, PrimaryMarkets: []string{"US"}, TradingHorizons: []string{"swing"}}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	principle := TradingPrinciple{Category: PrincipleCategoryNoTrade, Content: "不追价", Enabled: true, AIEnabled: true}
	if err := database.Create(&principle).Error; err != nil {
		t.Fatal(err)
	}
	aiEnabled := true
	observation := BehaviorObservation{Key: "execution.chasing_entry", Scenario: "swing", Status: "eligible", SampleCount: 10, Confidence: .6, Message: "存在追价样本", EvidenceReviewIDs: []string{}, AIEnabled: &aiEnabled}
	if err := database.Create(&observation).Error; err != nil {
		t.Fatal(err)
	}
	if profile.UserID != DefaultUserID || principle.UserID != DefaultUserID || observation.UserID != DefaultUserID {
		t.Fatalf("records were not assigned to default user: profile=%s principle=%s observation=%s", profile.UserID, principle.UserID, observation.UserID)
	}
}
