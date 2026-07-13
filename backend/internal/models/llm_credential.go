package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LLMCredential stores only the lookup scope in plaintext. The API key is
// AES-GCM encrypted by the backend and is never included in an API response.
type LLMCredential struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Provider             string `gorm:"type:text;not null;uniqueIndex:idx_llm_credential_scope"`
	BackendHost          string `gorm:"type:text;not null;default:'';uniqueIndex:idx_llm_credential_scope"`
	CredentialCiphertext []byte `gorm:"type:bytea;not null"`
	CredentialNonce      []byte `gorm:"type:bytea;not null"`
}

func (credential *LLMCredential) BeforeCreate(_ *gorm.DB) error {
	if credential.ID == uuid.Nil {
		credential.ID = uuid.New()
	}
	return nil
}
