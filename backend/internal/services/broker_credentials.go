package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// BrokerCredentialVault encrypts broker secrets before they reach Postgres.
// Its key is intentionally supplied only by the server environment.
type BrokerCredentialVault struct {
	gcm cipher.AEAD
}

func NewBrokerCredentialVault(encodedKey string) (*BrokerCredentialVault, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, errors.New("BROKER_CREDENTIALS_ENCRYPTION_KEY is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encodedKey)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("BROKER_CREDENTIALS_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create broker credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create broker credential GCM: %w", err)
	}
	return &BrokerCredentialVault{gcm: gcm}, nil
}

func (v *BrokerCredentialVault) Encrypt(values map[string]string) ([]byte, []byte, error) {
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, nil, fmt.Errorf("encode broker credentials: %w", err)
	}
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate broker credential nonce: %w", err)
	}
	return v.gcm.Seal(nil, nonce, plain, nil), nonce, nil
}

func (v *BrokerCredentialVault) Decrypt(ciphertext, nonce []byte) (map[string]string, error) {
	plain, err := v.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("broker credentials could not be decrypted")
	}
	values := map[string]string{}
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, fmt.Errorf("decode broker credentials: %w", err)
	}
	return values, nil
}
