package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	TradePlanDirectionBuy   = "buy"
	TradePlanDirectionSell  = "sell"
	TradePlanDirectionWatch = "watch"

	TradePlanStatusDraft     = "draft"
	TradePlanStatusWaiting   = "waiting"
	TradePlanStatusExecuted  = "executed"
	TradePlanStatusCancelled = "cancelled"
	TradePlanStatusExpired   = "expired"
)

type TradePlan struct {
	ID                  uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID              uuid.UUID  `json:"user_id" gorm:"type:uuid;not null;index"`
	CreatedAt           time.Time  `json:"created_at" gorm:"index:idx_trade_plans_created_at,sort:desc"`
	UpdatedAt           time.Time  `json:"updated_at"`
	Ticker              string     `json:"ticker" gorm:"type:text;not null;index:idx_trade_plans_ticker_status"`
	Direction           string     `json:"direction" gorm:"type:text;not null;check:direction IN ('buy','sell','watch')"`
	Status              string     `json:"status" gorm:"type:text;not null;default:'draft';index:idx_trade_plans_ticker_status;check:status IN ('draft','waiting','executed','cancelled','expired')"`
	PlanDate            time.Time  `json:"plan_date" gorm:"type:date;not null"`
	ValidUntil          *time.Time `json:"valid_until" gorm:"type:date"`
	UserThesisID        *uuid.UUID `json:"user_thesis_id" gorm:"type:uuid;index"`
	PlaybookID          *uuid.UUID `json:"playbook_id" gorm:"type:uuid;index"`
	EntryTrigger        *string    `json:"entry_trigger" gorm:"type:text"`
	EntryPrice          *float64   `json:"entry_price" gorm:"type:numeric(12,4)"`
	StopLoss            *float64   `json:"stop_loss" gorm:"type:numeric(12,4)"`
	TakeProfit          *float64   `json:"take_profit" gorm:"type:numeric(12,4)"`
	PositionSize        *float64   `json:"position_size" gorm:"type:numeric(6,2)"`
	PlannedQuantity     *float64   `json:"planned_quantity" gorm:"type:numeric(20,4)"`
	RiskMultiple        *float64   `json:"risk_multiple" gorm:"type:numeric(6,2)"`
	InitialRiskPerShare *float64   `json:"initial_risk_per_share" gorm:"type:numeric(12,4)"`
	OneRAmount          *float64   `json:"one_r_amount" gorm:"type:numeric(20,4)"`
	Invalidation        *string    `json:"invalidation" gorm:"type:text"`
	MaxRiskAmount       *float64   `json:"max_risk_amount" gorm:"type:numeric(20,4)"`
	MaxRiskPercent      *float64   `json:"max_risk_percent" gorm:"type:numeric(8,4)"`
	Rationale           *string    `json:"rationale" gorm:"type:text"`
	AISummary           *string    `json:"ai_summary" gorm:"column:ai_summary;type:text"`
	Notes               *string    `json:"notes" gorm:"type:text"`
	ExecutedJournalID   *uuid.UUID `json:"executed_journal_id" gorm:"type:uuid;index"`
	ExecutedAt          *time.Time `json:"executed_at"`
}

func (plan *TradePlan) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&plan.UserID)
	if plan.ID == uuid.Nil {
		plan.ID = uuid.New()
	}
	return nil
}

func ValidTradePlanDirection(direction string) bool {
	switch direction {
	case TradePlanDirectionBuy, TradePlanDirectionSell, TradePlanDirectionWatch:
		return true
	default:
		return false
	}
}

func ValidTradePlanStatus(status string) bool {
	switch status {
	case TradePlanStatusDraft, TradePlanStatusWaiting, TradePlanStatusExecuted, TradePlanStatusCancelled, TradePlanStatusExpired:
		return true
	default:
		return false
	}
}
