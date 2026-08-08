package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TradeExecutionDiff struct {
	PlanLinkStatus       string   `json:"plan_link_status"`
	SymbolMatch          *bool    `json:"symbol_match"`
	DirectionMatch       *bool    `json:"direction_match"`
	EntrySlippagePercent *float64 `json:"entry_slippage_percent"`
	TimingStatus         string   `json:"timing_status"`
	KnownDeviations      []string `json:"known_deviations"`
	UnknownComparisons   []string `json:"unknown_comparisons"`
}

type TradeReview struct {
	ID                uuid.UUID          `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	JournalEntryID    uuid.UUID          `json:"journal_entry_id" gorm:"type:uuid;not null;index:idx_trade_reviews_entry_version,unique"`
	Version           int                `json:"version" gorm:"not null;index:idx_trade_reviews_entry_version,unique"`
	TradePlanID       *uuid.UUID         `json:"trade_plan_id" gorm:"type:uuid;index"`
	PlanSnapshot      map[string]any     `json:"plan_snapshot" gorm:"serializer:json;type:jsonb;not null"`
	ExecutionSnapshot map[string]any     `json:"execution_snapshot" gorm:"serializer:json;type:jsonb;not null"`
	Diff              TradeExecutionDiff `json:"diff" gorm:"serializer:json;type:jsonb;not null"`
	ProcessQuality    string             `json:"process_quality" gorm:"type:text;not null;default:'unreviewed'"`
	DeviationReason   *string            `json:"deviation_reason" gorm:"type:text"`
	EmotionBefore     *string            `json:"emotion_before" gorm:"type:text"`
	EmotionAfter      *string            `json:"emotion_after" gorm:"type:text"`
	OutcomeNotes      *string            `json:"outcome_notes" gorm:"type:text"`
	Lesson            *string            `json:"lesson" gorm:"type:text"`
	CompletedAt       *time.Time         `json:"completed_at"`
}

func (review *TradeReview) BeforeCreate(_ *gorm.DB) error {
	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}
	return nil
}

type BehaviorObservation struct {
	ID                uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Key               string     `json:"key" gorm:"type:text;not null;uniqueIndex"`
	Scenario          string     `json:"scenario" gorm:"type:text;not null"`
	Status            string     `json:"status" gorm:"type:text;not null"`
	SampleCount       int        `json:"sample_count" gorm:"not null"`
	Confidence        float64    `json:"confidence" gorm:"type:numeric(6,4);not null"`
	Message           string     `json:"message" gorm:"type:text;not null"`
	EvidenceReviewIDs []string   `json:"evidence_review_ids" gorm:"serializer:json;type:jsonb;not null"`
	Feedback          *string    `json:"feedback" gorm:"type:text"`
	FeedbackAt        *time.Time `json:"feedback_at"`
	UserCorrection    *string    `json:"user_correction" gorm:"type:text"`
	AIEnabled         *bool      `json:"ai_enabled" gorm:"not null;default:true"`
}

func (observation *BehaviorObservation) BeforeCreate(_ *gorm.DB) error {
	if observation.ID == uuid.Nil {
		observation.ID = uuid.New()
	}
	return nil
}
