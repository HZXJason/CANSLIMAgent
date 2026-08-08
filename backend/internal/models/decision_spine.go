package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CandidateSnapshot preserves why a symbol entered the research queue at a
// particular point in time. It is intentionally immutable after creation.
type CandidateSnapshot struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey"`
	CreatedAt          time.Time      `gorm:"index:idx_candidate_snapshots_created_at,sort:desc"`
	Symbol             string         `gorm:"type:text;not null;index:idx_candidate_snapshots_symbol_created"`
	Market             string         `gorm:"type:text;not null"`
	ScreenName         string         `gorm:"type:text;not null"`
	ScreenDefinitionID *uuid.UUID     `gorm:"type:uuid;index"`
	InclusionReasons   []string       `gorm:"serializer:json;type:jsonb;not null"`
	ExclusionRisks     []string       `gorm:"serializer:json;type:jsonb;not null"`
	ContextSnapshot    map[string]any `gorm:"serializer:json;type:jsonb;not null"`
	DataAsOf           *time.Time
}

func (snapshot *CandidateSnapshot) BeforeCreate(_ *gorm.DB) error {
	if snapshot.ID == uuid.Nil {
		snapshot.ID = uuid.New()
	}
	return nil
}

// ResearchCase binds one research session to the candidate evidence that was
// available when the session began.
type ResearchCase struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt           time.Time `gorm:"index:idx_research_cases_created_at,sort:desc"`
	UpdatedAt           time.Time
	CandidateSnapshotID uuid.UUID      `gorm:"type:uuid;not null;index"`
	Symbol              string         `gorm:"type:text;not null;index:idx_research_cases_symbol_updated"`
	Market              string         `gorm:"type:text;not null"`
	AnalysisRunID       *string        `gorm:"type:text;index"`
	EvidenceSnapshot    map[string]any `gorm:"serializer:json;type:jsonb;not null"`
}

func (research *ResearchCase) BeforeCreate(_ *gorm.DB) error {
	if research.ID == uuid.Nil {
		research.ID = uuid.New()
	}
	return nil
}

// UserThesis is append-only: revisions create a new row linked by ParentID.
type UserThesis struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey"`
	CreatedAt           time.Time  `gorm:"index:idx_user_theses_created_at,sort:desc"`
	ResearchCaseID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	ParentID            *uuid.UUID `gorm:"type:uuid;index"`
	Symbol              string     `gorm:"type:text;not null;index"`
	Version             int        `gorm:"not null"`
	Stance              string     `gorm:"type:text;not null"`
	Summary             string     `gorm:"type:text;not null"`
	AcceptedArguments   []string   `gorm:"serializer:json;type:jsonb;not null"`
	RejectedArguments   []string   `gorm:"serializer:json;type:jsonb;not null"`
	UnresolvedQuestions []string   `gorm:"serializer:json;type:jsonb;not null"`
	Falsifiers          []string   `gorm:"serializer:json;type:jsonb;not null"`
}

func (thesis *UserThesis) BeforeCreate(_ *gorm.DB) error {
	if thesis.ID == uuid.Nil {
		thesis.ID = uuid.New()
	}
	return nil
}
