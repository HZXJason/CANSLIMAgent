package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PrincipleCategoryNoTrade  = "no_trade"
	PrincipleCategoryPreTrade = "pretrade_check"
	PrincipleCategoryRiskExit = "risk_exit"

	PlaybookStatusDraft    = "draft"
	PlaybookStatusActive   = "active"
	PlaybookStatusArchived = "archived"
)

type TradingProfile struct {
	ID                     uint      `json:"-" gorm:"primaryKey"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	Configured             bool      `json:"configured" gorm:"not null;default:false"`
	PrimaryMarkets         []string  `json:"primary_markets" gorm:"serializer:json;type:jsonb;not null"`
	TradingHorizons        []string  `json:"trading_horizons" gorm:"serializer:json;type:jsonb;not null"`
	MaxRiskPerTradePercent *float64  `json:"max_risk_per_trade_percent" gorm:"type:numeric(8,4)"`
	MaxDailyLossPercent    *float64  `json:"max_daily_loss_percent" gorm:"type:numeric(8,4)"`
	MaxOpenPositions       *int      `json:"max_open_positions"`
}

type TradingPrinciple struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Category  string    `json:"category" gorm:"type:text;not null;index"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	Enabled   bool      `json:"enabled" gorm:"not null"`
	AIEnabled bool      `json:"ai_enabled" gorm:"not null"`
	SortOrder int       `json:"sort_order" gorm:"not null;default:0"`
}

func (principle *TradingPrinciple) BeforeCreate(_ *gorm.DB) error {
	if principle.ID == uuid.Nil {
		principle.ID = uuid.New()
	}
	return nil
}

type TradingPlaybook struct {
	ID                uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Name              string    `json:"name" gorm:"type:text;not null"`
	Markets           []string  `json:"markets" gorm:"serializer:json;type:jsonb;not null"`
	Horizons          []string  `json:"horizons" gorm:"serializer:json;type:jsonb;not null"`
	MarketContext     *string   `json:"market_context" gorm:"type:text"`
	EntryRules        *string   `json:"entry_rules" gorm:"type:text"`
	InvalidationRules *string   `json:"invalidation_rules" gorm:"type:text"`
	StopRules         *string   `json:"stop_rules" gorm:"type:text"`
	TargetRules       *string   `json:"target_rules" gorm:"type:text"`
	PositionRules     *string   `json:"position_rules" gorm:"type:text"`
	NoTradeRules      *string   `json:"no_trade_rules" gorm:"type:text"`
	Notes             *string   `json:"notes" gorm:"type:text"`
	Status            string    `json:"status" gorm:"type:text;not null;index"`
	Source            string    `json:"source" gorm:"type:text;not null"`
	EvidenceReviewIDs []string  `json:"evidence_review_ids" gorm:"serializer:json;type:jsonb;not null"`
	EvidenceCount     int       `json:"evidence_count" gorm:"not null;default:0"`
	AIEnabled         bool      `json:"ai_enabled" gorm:"not null"`
}

func (playbook *TradingPlaybook) BeforeCreate(_ *gorm.DB) error {
	if playbook.ID == uuid.Nil {
		playbook.ID = uuid.New()
	}
	return nil
}

type AIMemoryPreference struct {
	ID                  uint      `json:"-" gorm:"primaryKey"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	PrinciplesEnabled   bool      `json:"principles_enabled" gorm:"not null"`
	ObservationsEnabled bool      `json:"observations_enabled" gorm:"not null"`
	PlaybooksEnabled    bool      `json:"playbooks_enabled" gorm:"not null"`
}

func ValidPrincipleCategory(value string) bool {
	return value == PrincipleCategoryNoTrade || value == PrincipleCategoryPreTrade || value == PrincipleCategoryRiskExit
}

func ValidPlaybookStatus(value string) bool {
	return value == PlaybookStatusDraft || value == PlaybookStatusActive || value == PlaybookStatusArchived
}
