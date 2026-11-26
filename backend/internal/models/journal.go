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
	ID          uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      uuid.UUID  `json:"user_id" gorm:"type:uuid;not null;index"`
	CreatedAt   time.Time  `json:"created_at" gorm:"index:idx_trade_journal_created_at,sort:desc"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ExecutedAt  time.Time  `json:"executed_at" gorm:"column:executed_at;not null;default:CURRENT_TIMESTAMP;index:idx_trade_journal_executed_at,sort:desc"`
	Symbol      string     `json:"symbol" gorm:"type:text;not null;default:'';index:idx_trade_journal_symbol"`
	Name        string     `json:"name" gorm:"type:text;not null;default:''"`
	Side        string     `json:"side" gorm:"type:text;not null;default:'buy';check:side IN ('buy','sell')"`
	Quantity    float64    `json:"quantity" gorm:"type:numeric(20,8);not null;default:0"`
	Price       float64    `json:"price" gorm:"type:numeric(20,8);not null;default:0"`
	Amount      float64    `json:"amount" gorm:"type:numeric(24,8);not null;default:0"`
	Fee         float64    `json:"fee" gorm:"type:numeric(20,8);not null;default:0"`
	Market      string     `json:"market" gorm:"type:text;not null;default:'other';index:idx_trade_journal_market"`
	Currency    string     `json:"currency" gorm:"type:text;not null;default:''"`
	OrderID     *string    `json:"order_id" gorm:"column:order_id;type:text;index"`
	TradeID     *uuid.UUID `json:"trade_id" gorm:"type:uuid;index"`
	TradePlanID *uuid.UUID `json:"trade_plan_id" gorm:"type:uuid;index"`
	Source      string     `json:"source" gorm:"type:text;not null;default:'manual'"`
	Notes       *string    `json:"notes" gorm:"type:text"`
	Reflection  *string    `json:"reflection" gorm:"type:text"`
}

func (entry *TradeJournalEntry) BeforeCreate(_ *gorm.DB) error {
	ensureDefaultUserID(&entry.UserID)
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
