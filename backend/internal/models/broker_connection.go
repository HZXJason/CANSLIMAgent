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

type BrokerConnectionStatus string

const (
	BrokerConnectionStatusUnconnected        BrokerConnectionStatus = "unconnected"
	BrokerConnectionStatusPending            BrokerConnectionStatus = "pending_authorization"
	BrokerConnectionStatusConnected          BrokerConnectionStatus = "connected"
	BrokerConnectionStatusReconnectRequired  BrokerConnectionStatus = "reconnect_required"
	BrokerConnectionStatusGatewayUnavailable BrokerConnectionStatus = "gateway_unavailable"
)

// BrokerConnection stores only metadata in plaintext. The credential columns
// contain AES-GCM encrypted data and must never be included in an API response.
type BrokerConnection struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Broker                 string                 `gorm:"type:text;not null;uniqueIndex"`
	Label                  string                 `gorm:"type:text;not null;default:''"`
	CredentialCiphertext   []byte                 `gorm:"type:bytea;not null"`
	CredentialNonce        []byte                 `gorm:"type:bytea;not null"`
	AccountName            string                 `gorm:"type:text;not null;default:''"`
	ConnectionMethod       string                 `gorm:"type:text;not null;default:''"`
	ConnectionStatus       BrokerConnectionStatus `gorm:"type:text;not null;default:'unconnected';index"`
	AuthorizationExpiresAt *time.Time             `gorm:"index"`
	LastSyncedAt           *time.Time             `gorm:"index"`
}

// BrokerAuthorizationSession tracks a one-time OAuth authorization attempt.
// StateHash is SHA-256(state), so the raw browser value is never persisted.
type BrokerAuthorizationSession struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Broker                 string     `gorm:"type:text;not null;index"`
	StateHash              []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	PKCEVerifierCiphertext []byte     `gorm:"type:bytea;not null"`
	PKCEVerifierNonce      []byte     `gorm:"type:bytea;not null"`
	ExpiresAt              time.Time  `gorm:"not null;index"`
	ConsumedAt             *time.Time `gorm:"index"`
}

func (connection *BrokerConnection) BeforeCreate(_ *gorm.DB) error {
	if connection.ID == uuid.Nil {
		connection.ID = uuid.New()
	}
	return nil
}

func (session *BrokerAuthorizationSession) BeforeCreate(_ *gorm.DB) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
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

func ValidBrokerConnectionStatus(value BrokerConnectionStatus) bool {
	switch value {
	case BrokerConnectionStatusUnconnected,
		BrokerConnectionStatusPending,
		BrokerConnectionStatusConnected,
		BrokerConnectionStatusReconnectRequired,
		BrokerConnectionStatusGatewayUnavailable:
		return true
	default:
		return false
	}
}
