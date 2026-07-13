package services

import (
	"encoding/base64"
	"testing"
)

func TestBrokerCredentialVaultRoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	vault, err := NewBrokerCredentialVault(key)
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	ciphertext, nonce, err := vault.Encrypt(map[string]string{"access_token": "secret"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if string(ciphertext) == "secret" {
		t.Fatal("credential was not encrypted")
	}
	values, err := vault.Decrypt(ciphertext, nonce)
	if err != nil || values["access_token"] != "secret" {
		t.Fatalf("unexpected decrypted values: %v / %v", values, err)
	}
}

func TestBrokerCredentialVaultScopedCiphertextCannotBeSwapped(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	vault, err := NewBrokerCredentialVault(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := vault.EncryptScoped("broker:ibkr", map[string]string{"access_token": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.DecryptScoped("broker:futu", ciphertext, nonce); err == nil {
		t.Fatal("scoped ciphertext decrypted under a different broker")
	}
}

func TestBrokerCredentialVaultRejectsInvalidKey(t *testing.T) {
	if _, err := NewBrokerCredentialVault("not-a-key"); err == nil {
		t.Fatal("expected invalid key to be rejected")
	}
}

func TestBrokerSyncUpstreamMustBeLoopback(t *testing.T) {
	if err := validateLoopbackBrokerUpstream("https://broker.example.com/broker/sync"); err == nil {
		t.Fatal("expected remote broker sidecar to be rejected")
	}
	if err := validateLoopbackBrokerUpstream("http://127.0.0.1:8765/broker/sync"); err != nil {
		t.Fatalf("expected loopback sidecar to be accepted: %v", err)
	}
}
