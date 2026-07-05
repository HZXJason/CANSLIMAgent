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
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt         time.Time `gorm:"index:idx_trade_plans_created_at,sort:desc"`
	UpdatedAt         time.Time
	Ticker            string     `gorm:"type:text;not null;index:idx_trade_plans_ticker_status"`
	Direction         string     `gorm:"type:text;not null;check:direction IN ('buy','sell','watch')"`
	Status            string     `gorm:"type:text;not null;default:'draft';index:idx_trade_plans_ticker_status;check:status IN ('draft','waiting','executed','cancelled','expired')"`
	PlanDate          time.Time  `gorm:"type:date;not null"`
	ValidUntil        *time.Time `gorm:"type:date"`
	EntryTrigger      *string    `gorm:"type:text"`
	EntryPrice        *float64   `gorm:"type:numeric(12,4)"`
	StopLoss          *float64   `gorm:"type:numeric(12,4)"`
	TakeProfit        *float64   `gorm:"type:numeric(12,4)"`
	PositionSize      *float64   `gorm:"type:numeric(6,2)"`
	Rationale         *string    `gorm:"type:text"`
	AISummary         *string    `gorm:"column:ai_summary;type:text"`
	Notes             *string    `gorm:"type:text"`
	ExecutedJournalID *uuid.UUID `gorm:"type:uuid;index"`
	ExecutedAt        *time.Time
}

func (plan *TradePlan) BeforeCreate(_ *gorm.DB) error {
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
