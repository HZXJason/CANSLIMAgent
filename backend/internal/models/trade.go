package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	TradeDirectionLong  = "long"
	TradeDirectionShort = "short"

	TradeStatusOpen      = "open"
	TradeStatusClosed    = "closed"
	TradeStatusCancelled = "cancelled"

	TradeQuadrantOpen          = "open"
	TradeQuadrantModelWin      = "model_win"
	TradeQuadrantGoodLoss      = "good_loss"
	TradeQuadrantUnforcedError = "unforced_error"
	TradeQuadrantToxicWin      = "toxic_win"
)

// Trade represents one complete position lifecycle. Individual broker fills
// remain immutable TradeJournalEntry records linked through TradeID.
type Trade struct {
	ID                 uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	UserID             uuid.UUID  `json:"user_id" gorm:"type:uuid;not null;index:idx_trades_user_status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	TradePlanID        *uuid.UUID `json:"trade_plan_id" gorm:"type:uuid;index"`
	PlaybookID         *uuid.UUID `json:"playbook_id" gorm:"type:uuid;index"`
	Symbol             string     `json:"symbol" gorm:"type:text;not null;index"`
	Name               string     `json:"name" gorm:"type:text;not null;default:''"`
	Market             string     `json:"market" gorm:"type:text;not null;index"`
	Currency           string     `json:"currency" gorm:"type:text;not null;default:'USD'"`
	Direction          string     `json:"direction" gorm:"type:text;not null"`
	Status             string     `json:"status" gorm:"type:text;not null;index:idx_trades_user_status"`
	OpenedAt           time.Time  `json:"opened_at" gorm:"not null;index"`
	ClosedAt           *time.Time `json:"closed_at" gorm:"index"`
	OpenQuantity       float64    `json:"open_quantity" gorm:"type:numeric(20,8);not null;default:0"`
	TotalBuyQuantity   float64    `json:"total_buy_quantity" gorm:"type:numeric(20,8);not null;default:0"`
	TotalSellQuantity  float64    `json:"total_sell_quantity" gorm:"type:numeric(20,8);not null;default:0"`
	AvgEntryPrice      float64    `json:"avg_entry_price" gorm:"type:numeric(20,8);not null;default:0"`
	AvgExitPrice       *float64   `json:"avg_exit_price" gorm:"type:numeric(20,8)"`
	CurrentStopLoss    *float64   `json:"current_stop_loss" gorm:"type:numeric(20,8)"`
	TotalFees          float64    `json:"total_fees" gorm:"type:numeric(20,8);not null;default:0"`
	PlannedEntryPrice  *float64   `json:"planned_entry_price" gorm:"type:numeric(20,8)"`
	PlannedStopLoss    *float64   `json:"planned_stop_loss" gorm:"type:numeric(20,8)"`
	InitialRiskAmount  *float64   `json:"initial_risk_amount" gorm:"type:numeric(24,8)"`
	OpenRiskAmount     float64    `json:"open_risk_amount" gorm:"type:numeric(24,8);not null;default:0"`
	RealizedPnL        *float64   `json:"realized_pnl" gorm:"type:numeric(24,8)"`
	RealizedPnLPercent *float64   `json:"realized_pnl_percent" gorm:"type:numeric(12,6)"`
	RealizedR          *float64   `json:"realized_r" gorm:"type:numeric(12,4)"`
	EntrySlippageR     *float64   `json:"entry_slippage_r" gorm:"type:numeric(12,4)"`
	HoldingDays        *float64   `json:"holding_days" gorm:"type:numeric(10,2)"`
	Quadrant           string     `json:"quadrant" gorm:"type:text;not null;default:'open'"`
	AutoViolations     []string   `json:"auto_violations" gorm:"serializer:json;type:jsonb;not null"`
}

func (trade *Trade) BeforeCreate(_ *gorm.DB) error {
	if trade.ID == uuid.Nil {
		trade.ID = uuid.New()
	}
	ensureDefaultUserID(&trade.UserID)
	if trade.Currency == "" {
		trade.Currency = "USD"
	}
	if trade.Quadrant == "" {
		trade.Quadrant = TradeQuadrantOpen
	}
	if trade.AutoViolations == nil {
		trade.AutoViolations = []string{}
	}
	return nil
}
