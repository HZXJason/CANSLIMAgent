package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AICalibrationStatusRunning  = "running"
	AICalibrationStatusComplete = "complete"
	AICalibrationStatusFailed   = "failed"
)

// AICalibration stores one reusable point-in-time decision replay Run Card.
// ConfigHash includes the model pair, analyst set and decision-policy version.
type AICalibration struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Symbol          string    `gorm:"type:text;not null;uniqueIndex:idx_ai_calibrations_symbol_config"`
	ConfigHash      string    `gorm:"type:text;not null;uniqueIndex:idx_ai_calibrations_symbol_config"`
	Provider        string    `gorm:"type:text;not null"`
	DeepThinkLLM    string    `gorm:"type:text;not null"`
	QuickThinkLLM   string    `gorm:"type:text;not null"`
	StrategyVersion string    `gorm:"type:text;not null"`
	Status          string    `gorm:"type:text;not null;index"`
	TotalNodes      int       `gorm:"not null;default:10"`
	CompletedNodes  int       `gorm:"not null;default:0"`
	CachedNodes     int       `gorm:"not null;default:0"`
	FailedNodes     int       `gorm:"not null;default:0"`
	SampleCount     int       `gorm:"not null;default:0"`
	CorrectCount    int       `gorm:"not null;default:0"`
	Accuracy        float64   `gorm:"type:numeric(8,6);not null;default:0"`
	Confidence      string    `gorm:"type:text;not null;default:'INSUFFICIENT'"`
	HorizonDays     int       `gorm:"not null;default:20"`
	StartedAt       time.Time `gorm:"not null"`
	CompletedAt     *time.Time
	Error           *string        `gorm:"type:text"`
	Payload         map[string]any `gorm:"serializer:json;type:jsonb;not null"`
}

func (calibration *AICalibration) BeforeCreate(_ *gorm.DB) error {
	if calibration.ID == uuid.Nil {
		calibration.ID = uuid.New()
	}
	return nil
}
