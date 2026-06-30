package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	TradeDirectionBuy  = "buy"
	TradeDirectionSell = "sell"
)

type TradeJournalEntry struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt    time.Time `gorm:"index:idx_trade_journal_created_at,sort:desc"`
	UpdatedAt    time.Time
	Ticker       string    `gorm:"type:text;not null"`
	Direction    string    `gorm:"type:text;not null;check:direction IN ('buy','sell')"`
	TradeDate    time.Time `gorm:"type:date;not null"`
	Price        float64   `gorm:"type:numeric(12,4);not null"`
	PositionSize float64   `gorm:"type:numeric(6,2);not null"`
	StopLoss     float64   `gorm:"type:numeric(12,4);not null"`
	Notes        *string   `gorm:"type:text"`
	ExitPrice    *float64  `gorm:"type:numeric(12,4)"`
	PnLPct       *float64  `gorm:"column:pnl_pct;type:numeric(8,4)"`
	Reflection   *string   `gorm:"type:text"`
	Phase1Locked bool      `gorm:"not null;default:false"`
}

func (entry *TradeJournalEntry) BeforeCreate(_ *gorm.DB) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	return nil
}

func ValidTradeDirection(direction string) bool {
	return direction == TradeDirectionBuy || direction == TradeDirectionSell
}
