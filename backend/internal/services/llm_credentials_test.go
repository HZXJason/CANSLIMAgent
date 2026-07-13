package services

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLLMCredentialStoreEncryptsAndScopesKeys(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.LLMCredential{}); err != nil {
		t.Fatal(err)
	}
	vault, err := NewCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := NewLLMCredentialStore(database, vault)
	ctx := context.Background()
	status, err := store.Save(ctx, "openai_compatible", "https://gateway.example.com/v1", "sk-server-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.BackendHost != "gateway.example.com" {
		t.Fatalf("unexpected status: %+v", status)
	}
	var row models.LLMCredential
	if err := database.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(row.CredentialCiphertext), "sk-server-secret") {
		t.Fatal("plaintext API key reached the database")
	}
	resolved, err := store.Resolve(ctx, "openai_compatible", "https://gateway.example.com/chat/completions")
	if err != nil || resolved != "sk-server-secret" {
		t.Fatalf("resolve = %q / %v", resolved, err)
	}
	other, err := store.Resolve(ctx, "openai_compatible", "https://other.example.com/v1")
	if err != nil || other != "" {
		t.Fatalf("credential crossed gateway scope: %q / %v", other, err)
	}
	if err := store.Delete(ctx, "openai_compatible", "https://gateway.example.com/v1"); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := store.Status(ctx, "openai_compatible", "https://gateway.example.com/v1")
	if err != nil || afterDelete.Configured {
		t.Fatalf("credential still configured: %+v / %v", afterDelete, err)
	}
}

func TestNormalizeLLMCredentialScopeRejectsUnsafeOrUnknownTargets(t *testing.T) {
	for _, test := range []struct{ provider, backendURL string }{
		{"openai_compatible", "http://gateway.example.com/v1"},
		{"openai_compatible", "https://user:pass@gateway.example.com/v1"},
		{"unknown", "https://gateway.example.com/v1"},
	} {
		if _, _, err := normalizeLLMCredentialScope(test.provider, test.backendURL); err == nil {
			t.Fatalf("accepted unsafe scope: %+v", test)
		}
	}
}
