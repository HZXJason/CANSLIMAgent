package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const UserTradingContextV2 = "alphalens.user-trading-context.v2"

type UserTradingContext struct {
	Version              string                       `json:"version"`
	GeneratedAt          time.Time                    `json:"generated_at"`
	Purpose              string                       `json:"purpose"`
	User                 models.User                  `json:"user"`
	DeclaredProfile      models.TradingProfile        `json:"declared_profile"`
	Policies             []models.TradingPolicy       `json:"policies"`
	Principles           []models.TradingPrinciple    `json:"principles"`
	Playbooks            []models.TradingPlaybook     `json:"playbooks"`
	BehaviorObservations []models.BehaviorObservation `json:"behavior_observations"`
	Activity             UserActivitySummary          `json:"activity"`
}

type UserActivitySummary struct {
	TradePlans       int64 `json:"trade_plans"`
	Trades           int64 `json:"trades"`
	ExecutionFills   int64 `json:"execution_fills"`
	CompletedReviews int64 `json:"completed_reviews"`
}

func BuildUserTradingContext(ctx context.Context, database *gorm.DB, userID uuid.UUID, purpose string, now time.Time) (UserTradingContext, error) {
	result := UserTradingContext{
		Version: UserTradingContextV2, GeneratedAt: now.UTC(), Purpose: purpose,
		Policies: []models.TradingPolicy{}, Principles: []models.TradingPrinciple{}, Playbooks: []models.TradingPlaybook{},
		BehaviorObservations: []models.BehaviorObservation{},
	}
	if err := database.WithContext(ctx).First(&result.User, "id = ?", userID).Error; err != nil {
		return UserTradingContext{}, err
	}
	if err := database.WithContext(ctx).Where("user_id = ?", userID).First(&result.DeclaredProfile).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return UserTradingContext{}, err
	}
	if result.DeclaredProfile.UserID == uuid.Nil {
		result.DeclaredProfile.UserID = userID
		result.DeclaredProfile.PrimaryMarkets = []string{}
		result.DeclaredProfile.AssetClasses = []string{}
		result.DeclaredProfile.TradingHorizons = []string{}
		result.DeclaredProfile.StrategyPreferences = []string{}
		result.DeclaredProfile.InvestmentObjectives = []string{}
	}
	if err := database.WithContext(ctx).Where("user_id = ? AND status = ? AND prompt_enabled = ?", userID, models.PolicyStatusActive, true).Order("created_at asc").Find(&result.Policies).Error; err != nil {
		return UserTradingContext{}, err
	}
	if err := database.WithContext(ctx).Where("user_id = ? AND enabled = ? AND ai_enabled = ?", userID, true, true).Order("sort_order asc").Find(&result.Principles).Error; err != nil {
		return UserTradingContext{}, err
	}
	if err := database.WithContext(ctx).Where("user_id = ? AND status = ? AND ai_enabled = ?", userID, models.PlaybookStatusActive, true).Order("created_at asc").Find(&result.Playbooks).Error; err != nil {
		return UserTradingContext{}, err
	}
	if err := database.WithContext(ctx).Where("user_id = ? AND status = ? AND ai_enabled = ?", userID, "eligible", true).Order("key asc").Find(&result.BehaviorObservations).Error; err != nil {
		return UserTradingContext{}, err
	}
	counts := []struct {
		model  any
		query  string
		target *int64
	}{
		{&models.TradePlan{}, "user_id = ?", &result.Activity.TradePlans},
		{&models.Trade{}, "user_id = ?", &result.Activity.Trades},
		{&models.TradeJournalEntry{}, "user_id = ?", &result.Activity.ExecutionFills},
		{&models.TradeReview{}, "user_id = ? AND completed_at IS NOT NULL", &result.Activity.CompletedReviews},
	}
	for _, item := range counts {
		if err := database.WithContext(ctx).Model(item.model).Where(item.query, userID).Count(item.target).Error; err != nil {
			return UserTradingContext{}, err
		}
	}
	return result, nil
}

func SavePromptContextSnapshot(ctx context.Context, database *gorm.DB, value UserTradingContext, tradePlanID *uuid.UUID) (models.PromptContextSnapshot, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return models.PromptContextSnapshot{}, err
	}
	digest := sha256.Sum256(payload)
	var structured map[string]any
	if err := json.Unmarshal(payload, &structured); err != nil {
		return models.PromptContextSnapshot{}, err
	}
	snapshot := models.PromptContextSnapshot{
		UserID: value.User.ID, Purpose: value.Purpose, ContextVersion: value.Version,
		TradePlanID: tradePlanID, ContextHash: hex.EncodeToString(digest[:]), Payload: structured,
	}
	return snapshot, database.WithContext(ctx).Create(&snapshot).Error
}
