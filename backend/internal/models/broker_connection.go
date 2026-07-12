package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	BrokerLongPort = "longport"
	BrokerTiger    = "tiger"
	BrokerFutu     = "futu"
	BrokerIBKR     = "ibkr"
)

// BrokerConnection stores only metadata in plaintext. The credential columns
// contain AES-GCM encrypted data and must never be included in an API response.
type BrokerConnection struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Broker               string     `gorm:"type:text;not null;uniqueIndex"`
	Label                string     `gorm:"type:text;not null;default:''"`
	CredentialCiphertext []byte     `gorm:"type:bytea;not null"`
	CredentialNonce      []byte     `gorm:"type:bytea;not null"`
	LastSyncedAt         *time.Time `gorm:"index"`
}

func (connection *BrokerConnection) BeforeCreate(_ *gorm.DB) error {
	if connection.ID == uuid.Nil {
		connection.ID = uuid.New()
	}
	return nil
}

func ValidBroker(value string) bool {
	switch value {
	case BrokerLongPort, BrokerTiger, BrokerFutu, BrokerIBKR:
		return true
	default:
		return false
	}
}
