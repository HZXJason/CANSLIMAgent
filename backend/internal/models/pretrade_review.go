package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PreTradeAIStatusNotRequested = "not_requested"
	PreTradeAIStatusCompleted    = "completed"
	PreTradeAIStatusUnavailable  = "unavailable"
	PreTradeAIStatusFailed       = "failed"
)

type PreTradeFinding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Category string         `json:"category"`
	Message  string         `json:"message"`
	Field    *string        `json:"field,omitempty"`
	Evidence map[string]any `json:"evidence"`
}

type PreTradeSource struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	URL      *string `json:"url,omitempty"`
	DataAsOf *string `json:"data_as_of,omitempty"`
	Status   string  `json:"status"`
}

type PreTradeRiskReview struct {
	ID                   uuid.UUID             `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt            time.Time             `json:"created_at" gorm:"index:idx_pretrade_reviews_created_at,sort:desc"`
	TradePlanID          uuid.UUID             `json:"trade_plan_id" gorm:"type:uuid;not null;index:idx_pretrade_reviews_plan_version,unique"`
	Version              int                   `json:"version" gorm:"not null;index:idx_pretrade_reviews_plan_version,unique"`
	PlanSnapshot         map[string]any        `json:"plan_snapshot" gorm:"serializer:json;type:jsonb;not null"`
	HardConflicts        []PreTradeFinding     `json:"hard_conflicts" gorm:"serializer:json;type:jsonb;not null"`
	RiskWarnings         []PreTradeFinding     `json:"risk_warnings" gorm:"serializer:json;type:jsonb;not null"`
	BehaviorReminders    []PreTradeFinding     `json:"behavior_reminders" gorm:"serializer:json;type:jsonb;not null"`
	UnresolvedQuestions  []PreTradeFinding     `json:"unresolved_questions" gorm:"serializer:json;type:jsonb;not null"`
	ChangesSinceResearch []PreTradeFinding     `json:"changes_since_research" gorm:"serializer:json;type:jsonb;not null"`
	Freshness            map[string]any        `json:"freshness" gorm:"serializer:json;type:jsonb;not null"`
	Sources              []PreTradeSource      `json:"sources" gorm:"serializer:json;type:jsonb;not null"`
	AIStatus             string                `json:"ai_status" gorm:"type:text;not null"`
	AIProvider           *string               `json:"ai_provider" gorm:"type:text"`
	AIModel              *string               `json:"ai_model" gorm:"type:text"`
	Acknowledgements     []RiskAcknowledgement `json:"acknowledgements" gorm:"-"`
}

func (review *PreTradeRiskReview) BeforeCreate(_ *gorm.DB) error {
	if review.ID == uuid.Nil {
		review.ID = uuid.New()
	}
	return nil
}

type RiskAcknowledgement struct {
	ID          uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt   time.Time `json:"created_at"`
	ReviewID    uuid.UUID `json:"review_id" gorm:"type:uuid;not null;index"`
	FindingCode string    `json:"finding_code" gorm:"type:text;not null"`
	Reason      *string   `json:"reason" gorm:"type:text"`
}

func (acknowledgement *RiskAcknowledgement) BeforeCreate(_ *gorm.DB) error {
	if acknowledgement.ID == uuid.Nil {
		acknowledgement.ID = uuid.New()
	}
	return nil
}
