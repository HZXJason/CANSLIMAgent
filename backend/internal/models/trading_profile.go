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
	ID                               uint      `json:"-" gorm:"primaryKey"`
	UserID                           uuid.UUID `json:"user_id" gorm:"type:uuid;not null;uniqueIndex"`
	CreatedAt                        time.Time `json:"created_at"`
	UpdatedAt                        time.Time `json:"updated_at"`
	Configured                       bool      `json:"configured" gorm:"not null;default:false"`
	PrimaryMarkets                   []string  `json:"primary_markets" gorm:"serializer:json;type:jsonb;not null"`
	AssetClasses                     []string  `json:"asset_classes" gorm:"serializer:json;type:jsonb;not null"`
	TradingHorizons                  []string  `json:"trading_horizons" gorm:"serializer:json;type:jsonb;not null"`
	StrategyPreferences              []string  `json:"strategy_preferences" gorm:"serializer:json;type:jsonb;not null"`
	InvestmentObjectives             []string  `json:"investment_objectives" gorm:"serializer:json;type:jsonb;not null"`
	RiskTolerance                    *string   `json:"risk_tolerance" gorm:"type:text"`
	MaxRiskPerTradePercent           *float64  `json:"max_risk_per_trade_percent" gorm:"type:numeric(8,4)"`
	MaxDailyLossPercent              *float64  `json:"max_daily_loss_percent" gorm:"type:numeric(8,4)"`
	MaxOpenPositions                 *int      `json:"max_open_positions"`
	MaxPortfolioConcentrationPercent *float64  `json:"max_portfolio_concentration_percent" gorm:"type:numeric(8,4)"`
	LeverageAllowed                  *bool     `json:"leverage_allowed"`
	AccountEquity                    *float64  `json:"account_equity" gorm:"type:numeric(20,4)"`
	AccountCurrency                  string    `json:"account_currency" gorm:"type:text;not null;default:'USD'"`
	StandardRiskPercent              *float64  `json:"standard_risk_percent" gorm:"type:numeric(8,4)"`
	MaxPortfolioHeatPercent          *float64  `json:"max_portfolio_heat_percent" gorm:"type:numeric(8,4)"`
}

func (profile *TradingProfile) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&profile.UserID)
	if profile.AccountCurrency == "" {
		profile.AccountCurrency = "USD"
	}
	return nil
}

type TradingPrinciple struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Category  string    `json:"category" gorm:"type:text;not null;index"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	Enabled   bool      `json:"enabled" gorm:"not null"`
	AIEnabled bool      `json:"ai_enabled" gorm:"not null"`
	SortOrder int       `json:"sort_order" gorm:"not null;default:0"`
}

func (principle *TradingPrinciple) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&principle.UserID)
	if principle.ID == uuid.Nil {
		principle.ID = uuid.New()
	}
	return nil
}

type TradingPlaybook struct {
	ID                     uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	UserID                 uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	Name                   string    `json:"name" gorm:"type:text;not null"`
	SetupType              string    `json:"setup_type" gorm:"type:text;not null;default:'custom'"`
	Markets                []string  `json:"markets" gorm:"serializer:json;type:jsonb;not null"`
	Horizons               []string  `json:"horizons" gorm:"serializer:json;type:jsonb;not null"`
	MarketContext          *string   `json:"market_context" gorm:"type:text"`
	EntryRules             *string   `json:"entry_rules" gorm:"type:text"`
	InvalidationRules      *string   `json:"invalidation_rules" gorm:"type:text"`
	StopRules              *string   `json:"stop_rules" gorm:"type:text"`
	TargetRules            *string   `json:"target_rules" gorm:"type:text"`
	PositionRules          *string   `json:"position_rules" gorm:"type:text"`
	NoTradeRules           *string   `json:"no_trade_rules" gorm:"type:text"`
	Notes                  *string   `json:"notes" gorm:"type:text"`
	DefaultRiskMultiple    *float64  `json:"default_risk_multiple" gorm:"type:numeric(6,2)"`
	DefaultStopATRMultiple *float64  `json:"default_stop_atr_multiple" gorm:"type:numeric(6,2)"`
	Status                 string    `json:"status" gorm:"type:text;not null;index"`
	Source                 string    `json:"source" gorm:"type:text;not null"`
	EvidenceReviewIDs      []string  `json:"evidence_review_ids" gorm:"serializer:json;type:jsonb;not null"`
	EvidenceCount          int       `json:"evidence_count" gorm:"not null;default:0"`
	AIEnabled              bool      `json:"ai_enabled" gorm:"not null"`
}

func (playbook *TradingPlaybook) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&playbook.UserID)
	if playbook.ID == uuid.Nil {
		playbook.ID = uuid.New()
	}
	if playbook.SetupType == "" {
		playbook.SetupType = "custom"
	}
	if playbook.Markets == nil {
		playbook.Markets = []string{}
	}
	if playbook.Horizons == nil {
		playbook.Horizons = []string{}
	}
	if playbook.EvidenceReviewIDs == nil {
		playbook.EvidenceReviewIDs = []string{}
	}
	return nil
}

type AIMemoryPreference struct {
	ID                  uint      `json:"-" gorm:"primaryKey"`
	UserID              uuid.UUID `json:"user_id" gorm:"type:uuid;not null;uniqueIndex"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	PrinciplesEnabled   bool      `json:"principles_enabled" gorm:"not null"`
	ObservationsEnabled bool      `json:"observations_enabled" gorm:"not null"`
	PlaybooksEnabled    bool      `json:"playbooks_enabled" gorm:"not null"`
}

func (preference *AIMemoryPreference) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&preference.UserID)
	return nil
}

func ValidPrincipleCategory(value string) bool {
	return value == PrincipleCategoryNoTrade || value == PrincipleCategoryPreTrade || value == PrincipleCategoryRiskExit
}

func ValidPlaybookStatus(value string) bool {
	return value == PlaybookStatusDraft || value == PlaybookStatusActive || value == PlaybookStatusArchived
}
