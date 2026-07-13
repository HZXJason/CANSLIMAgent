package services

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const llmAPIKeyField = "api_key"

type LLMCredentialStatus struct {
	Provider     string     `json:"provider"`
	BackendHost  string     `json:"backend_host"`
	Configured   bool       `json:"configured"`
	StorageReady bool       `json:"storage_ready"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

type LLMCredentialStore struct {
	db    *gorm.DB
	vault *CredentialVault
}

func NewLLMCredentialStore(database *gorm.DB, vault *CredentialVault) *LLMCredentialStore {
	return &LLMCredentialStore{db: database, vault: vault}
}

func (s *LLMCredentialStore) StorageReady() bool {
	return s != nil && s.db != nil && s.vault != nil
}

func (s *LLMCredentialStore) Status(ctx context.Context, provider, backendURL string) (LLMCredentialStatus, error) {
	provider, host, err := normalizeLLMCredentialScope(provider, backendURL)
	if err != nil {
		return LLMCredentialStatus{}, err
	}
	status := LLMCredentialStatus{Provider: provider, BackendHost: host, StorageReady: s.StorageReady()}
	if !s.StorageReady() {
		return status, nil
	}
	var credential models.LLMCredential
	err = s.db.WithContext(ctx).Where("provider = ? AND backend_host = ?", provider, host).First(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Configured = true
	status.UpdatedAt = &credential.UpdatedAt
	return status, nil
}

func (s *LLMCredentialStore) Save(ctx context.Context, provider, backendURL, apiKey string) (LLMCredentialStatus, error) {
	if !s.StorageReady() {
		return LLMCredentialStatus{}, errors.New("secure LLM credential storage is not configured")
	}
	provider, host, err := normalizeLLMCredentialScope(provider, backendURL)
	if err != nil {
		return LLMCredentialStatus{}, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || len(apiKey) > 8192 {
		return LLMCredentialStatus{}, errors.New("API key must be 1-8192 characters")
	}
	ciphertext, nonce, err := s.vault.EncryptScoped(llmCredentialCipherScope(provider, host), map[string]string{llmAPIKeyField: apiKey})
	if err != nil {
		return LLMCredentialStatus{}, err
	}
	credential := models.LLMCredential{
		Provider: provider, BackendHost: host,
		CredentialCiphertext: ciphertext, CredentialNonce: nonce,
	}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "backend_host"}},
		DoUpdates: clause.AssignmentColumns([]string{"credential_ciphertext", "credential_nonce", "updated_at"}),
	}).Create(&credential).Error
	if err != nil {
		return LLMCredentialStatus{}, err
	}
	return s.Status(ctx, provider, backendURL)
}

func (s *LLMCredentialStore) Delete(ctx context.Context, provider, backendURL string) error {
	if !s.StorageReady() {
		return errors.New("secure LLM credential storage is not configured")
	}
	provider, host, err := normalizeLLMCredentialScope(provider, backendURL)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Where("provider = ? AND backend_host = ?", provider, host).Delete(&models.LLMCredential{}).Error
}

func (s *LLMCredentialStore) Resolve(ctx context.Context, provider, backendURL string) (string, error) {
	if !s.StorageReady() {
		return "", nil
	}
	provider, host, err := normalizeLLMCredentialScope(provider, backendURL)
	if err != nil {
		return "", err
	}
	var credential models.LLMCredential
	err = s.db.WithContext(ctx).Where("provider = ? AND backend_host = ?", provider, host).First(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	values, err := s.vault.DecryptScoped(llmCredentialCipherScope(provider, host), credential.CredentialCiphertext, credential.CredentialNonce)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(values[llmAPIKeyField]), nil
}

func normalizeLLMCredentialScope(provider, backendURL string) (string, string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "anthropic", "openai", "google":
		return provider, "", nil
	case "openai_compatible":
		parsed, err := url.Parse(strings.TrimSpace(backendURL))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return "", "", errors.New("OpenAI-compatible credentials require an HTTPS backend_url without embedded credentials")
		}
		return provider, strings.ToLower(parsed.Host), nil
	default:
		return "", "", errors.New("unsupported LLM provider")
	}
}

func llmCredentialCipherScope(provider, backendHost string) string {
	return "llm:" + provider + ":" + backendHost
}
