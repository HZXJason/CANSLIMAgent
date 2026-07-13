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

// CredentialVault encrypts scoped secrets before they reach Postgres.
// Its key is intentionally supplied only by the server environment.
type CredentialVault struct {
	gcm cipher.AEAD
}

// BrokerCredentialVault preserves the existing broker-facing API while the
// same scoped vault implementation is reused for LLM credentials.
type BrokerCredentialVault = CredentialVault

var scopedCiphertextPrefix = []byte("v2:")

func NewCredentialVault(encodedKey string) (*CredentialVault, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return nil, errors.New("credential encryption key is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encodedKey)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("credential encryption key must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential GCM: %w", err)
	}
	return &CredentialVault{gcm: gcm}, nil
}

func NewBrokerCredentialVault(encodedKey string) (*BrokerCredentialVault, error) {
	return NewCredentialVault(encodedKey)
}

func (v *CredentialVault) Encrypt(values map[string]string) ([]byte, []byte, error) {
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

func (v *CredentialVault) Decrypt(ciphertext, nonce []byte) (map[string]string, error) {
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

func (v *CredentialVault) EncryptScoped(scope string, values map[string]string) ([]byte, []byte, error) {
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, nil, fmt.Errorf("encode broker credentials: %w", err)
	}
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate broker credential nonce: %w", err)
	}
	sealed := v.gcm.Seal(nil, nonce, plain, []byte(strings.TrimSpace(scope)))
	return append(append([]byte{}, scopedCiphertextPrefix...), sealed...), nonce, nil
}

func (v *CredentialVault) DecryptScoped(scope string, ciphertext, nonce []byte) (map[string]string, error) {
	if !strings.HasPrefix(string(ciphertext), string(scopedCiphertextPrefix)) {
		// Backward-compatible read for rows written before scoped AAD existed.
		return v.Decrypt(ciphertext, nonce)
	}
	plain, err := v.gcm.Open(nil, nonce, ciphertext[len(scopedCiphertextPrefix):], []byte(strings.TrimSpace(scope)))
	if err != nil {
		return nil, errors.New("broker credentials could not be decrypted")
	}
	values := map[string]string{}
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, fmt.Errorf("decode broker credentials: %w", err)
	}
	return values, nil
}
