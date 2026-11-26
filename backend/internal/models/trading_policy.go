package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PolicyKindRule       = "rule"
	PolicyKindDiscipline = "discipline"

	PolicyStrengthHard     = "hard"
	PolicyStrengthSoft     = "soft"
	PolicyStrengthReminder = "reminder"

	PolicyOriginManual      = "manual"
	PolicyOriginAISuggested = "ai_suggested"
	PolicyOriginSystem      = "system"

	PolicyStatusDraft    = "draft"
	PolicyStatusActive   = "active"
	PolicyStatusArchived = "archived"
)

// TradingPolicy is the shared storage model for user rules and disciplines.
// Origin describes authorship without deciding which authoring flows the UI exposes.
type TradingPolicy struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	UserID        uuid.UUID      `json:"user_id" gorm:"type:uuid;not null;index:idx_trading_policies_user_status"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Kind          string         `json:"kind" gorm:"type:text;not null;index"`
	Category      string         `json:"category" gorm:"type:text;not null;index"`
	Title         string         `json:"title" gorm:"type:text;not null"`
	Statement     string         `json:"statement" gorm:"type:text;not null"`
	Condition     map[string]any `json:"condition" gorm:"serializer:json;type:jsonb;not null"`
	Action        map[string]any `json:"action" gorm:"serializer:json;type:jsonb;not null"`
	Scope         map[string]any `json:"scope" gorm:"serializer:json;type:jsonb;not null"`
	Strength      string         `json:"strength" gorm:"type:text;not null"`
	Origin        string         `json:"origin" gorm:"type:text;not null"`
	Status        string         `json:"status" gorm:"type:text;not null;index:idx_trading_policies_user_status"`
	PromptEnabled bool           `json:"prompt_enabled" gorm:"not null"`
	Version       int            `json:"version" gorm:"not null"`
	EffectiveFrom *time.Time     `json:"effective_from"`
	EffectiveTo   *time.Time     `json:"effective_to"`
}

func (policy *TradingPolicy) BeforeCreate(_ *gorm.DB) error {
	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}
	ensureDefaultUserID(&policy.UserID)
	if policy.Condition == nil {
		policy.Condition = map[string]any{}
	}
	if policy.Action == nil {
		policy.Action = map[string]any{}
	}
	if policy.Scope == nil {
		policy.Scope = map[string]any{}
	}
	if policy.Version < 1 {
		policy.Version = 1
	}
	if policy.Origin == PolicyOriginAISuggested {
		policy.Status = PolicyStatusDraft
	}
	return nil
}

func (policy TradingPolicy) PromptEligible() bool {
	return policy.Status == PolicyStatusActive && policy.PromptEnabled
}

func ValidPolicyKind(value string) bool {
	return value == PolicyKindRule || value == PolicyKindDiscipline
}

func ValidPolicyStrength(value string) bool {
	return value == PolicyStrengthHard || value == PolicyStrengthSoft || value == PolicyStrengthReminder
}

func ValidPolicyOrigin(value string) bool {
	return value == PolicyOriginManual || value == PolicyOriginAISuggested || value == PolicyOriginSystem
}

func ValidPolicyStatus(value string) bool {
	return value == PolicyStatusDraft || value == PolicyStatusActive || value == PolicyStatusArchived
}
