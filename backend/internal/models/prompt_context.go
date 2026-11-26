package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PromptContextSnapshot struct {
	ID             uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID      `json:"user_id" gorm:"type:uuid;not null;index"`
	CreatedAt      time.Time      `json:"created_at" gorm:"index"`
	Purpose        string         `json:"purpose" gorm:"type:text;not null;index"`
	ContextVersion string         `json:"context_version" gorm:"type:text;not null"`
	TradePlanID    *uuid.UUID     `json:"trade_plan_id" gorm:"type:uuid;index"`
	ContextHash    string         `json:"context_hash" gorm:"type:text;not null;index"`
	Payload        map[string]any `json:"payload" gorm:"serializer:json;type:jsonb;not null"`
}

func (snapshot *PromptContextSnapshot) BeforeCreate(_ *gorm.DB) error {
	if snapshot.ID == uuid.Nil {
		snapshot.ID = uuid.New()
	}
	ensureDefaultUserID(&snapshot.UserID)
	if snapshot.Payload == nil {
		snapshot.Payload = map[string]any{}
	}
	return nil
}
