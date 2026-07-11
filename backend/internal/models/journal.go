package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	TradeSideBuy  = "buy"
	TradeSideSell = "sell"

	TradeMarketUS     = "us"
	TradeMarketHK     = "hk"
	TradeMarketChinaA = "china_a"
	TradeMarketCrypto = "crypto"
	TradeMarketOther  = "other"
)

// TradeJournalEntry is one immutable execution/fill. Its public JSON contract
// mirrors Vibe-Trading's TradeRecord instead of mixing plans with executions.
type TradeJournalEntry struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt  time.Time `gorm:"index:idx_trade_journal_created_at,sort:desc"`
	UpdatedAt  time.Time
	ExecutedAt time.Time `gorm:"column:executed_at;not null;default:CURRENT_TIMESTAMP;index:idx_trade_journal_executed_at,sort:desc"`
	Symbol     string    `gorm:"type:text;not null;default:'';index:idx_trade_journal_symbol"`
	Name       string    `gorm:"type:text;not null;default:''"`
	Side       string    `gorm:"type:text;not null;default:'buy';check:side IN ('buy','sell')"`
	Quantity   float64   `gorm:"type:numeric(20,8);not null;default:0"`
	Price      float64   `gorm:"type:numeric(20,8);not null;default:0"`
	Amount     float64   `gorm:"type:numeric(24,8);not null;default:0"`
	Fee        float64   `gorm:"type:numeric(20,8);not null;default:0"`
	Market     string    `gorm:"type:text;not null;default:'other';index:idx_trade_journal_market"`
	Currency   string    `gorm:"type:text;not null;default:''"`
	OrderID    *string   `gorm:"column:order_id;type:text;index"`
	Source     string    `gorm:"type:text;not null;default:'manual'"`
	Notes      *string   `gorm:"type:text"`
	Reflection *string   `gorm:"type:text"`
}

func (entry *TradeJournalEntry) BeforeCreate(_ *gorm.DB) error {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	return nil
}

func ValidTradeSide(side string) bool {
	return side == TradeSideBuy || side == TradeSideSell
}

func ValidTradeMarket(market string) bool {
	switch market {
	case TradeMarketUS, TradeMarketHK, TradeMarketChinaA, TradeMarketCrypto, TradeMarketOther:
		return true
	default:
		return false
	}
}
