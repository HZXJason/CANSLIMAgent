package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TradeJournalReflection stores one reproducible Vibe-powered behavior review.
type TradeJournalReflection struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	CreatedAt   time.Time      `gorm:"index:idx_trade_journal_reflections_created_at,sort:desc"`
	Scope       string         `gorm:"type:text;not null;default:'all';index:idx_trade_journal_reflections_scope"`
	RecordCount int            `gorm:"not null"`
	Engine      string         `gorm:"type:text;not null"`
	Payload     map[string]any `gorm:"serializer:json;type:jsonb;not null"`
}

func (reflection *TradeJournalReflection) BeforeCreate(_ *gorm.DB) error {
	if reflection.ID == uuid.Nil {
		reflection.ID = uuid.New()
	}
	return nil
}
