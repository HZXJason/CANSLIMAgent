package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestBeginOAuthAddsStateAndPKCEChallenge(t *testing.T) {
	start, err := newTestAuthorizer(t).BeginOAuth(context.Background(), models.BrokerLongPort)
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if u.Query().Get("state") == "" || u.Query().Get("code_challenge") == "" {
		t.Fatal("OAuth protections missing")
	}
	if got, want := u.Query().Get("code_challenge_method"), "S256"; got != want {
		t.Fatalf("code challenge method = %q, want %q", got, want)
	}
	if got, want := u.Query().Get("scope"), "account.read"; got != want {
		t.Fatalf("scope = %q, want %q", got, want)
	}
	if len(start.StateHash) != sha256.Size || len(start.VerifierCiphertext) == 0 || len(start.VerifierNonce) == 0 {
		t.Fatal("state or PKCE verifier is not safe to persist")
	}
	if string(start.StateHash) == u.Query().Get("state") {
		t.Fatal("raw OAuth state must not be returned for persistence")
	}

	vault := newTestVault(t)
	storedVerifier, err := vault.Decrypt(start.VerifierCiphertext, start.VerifierNonce)
	if err != nil {
		t.Fatalf("decrypt persisted verifier: %v", err)
	}
	verifier := storedVerifier[pkceVerifierCredentialKey]
	if verifier == "" {
		t.Fatal("missing encrypted PKCE verifier")
	}
	challengeHash := sha256.Sum256([]byte(verifier))
	if got, want := u.Query().Get("code_challenge"), base64.RawURLEncoding.EncodeToString(challengeHash[:]); got != want {
		t.Fatalf("code challenge = %q, want %q", got, want)
	}
}

func TestCompleteOAuthExchangesCodeAndReturnsAllowedAccountIdentity(t *testing.T) {
	var tokenRequest url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenRequest = r.PostForm
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"expires_in":    3600,
			})
		case "/account":
			if got, want := r.Header.Get("Authorization"), "Bearer access-token"; got != want {
				t.Fatalf("authorization = %q, want %q", got, want)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"account_name": "Longbridge · 12345678"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authorizer := newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token",
		AccountURL: server.URL + "/account", RedirectURL: "http://127.0.0.1:5173/settings",
	})

	identity, err := authorizer.CompleteOAuth(context.Background(), models.BrokerLongPort, "authorization-code", "pkce-verifier")
	if err != nil {
		t.Fatalf("complete OAuth: %v", err)
	}
	if got, want := identity.AccountName, "Longbridge · 12345678"; got != want {
		t.Fatalf("account name = %q, want %q", got, want)
	}
	if identity.Credentials["access_token"] != "access-token" || identity.Credentials["refresh_token"] != "refresh-token" {
		t.Fatalf("unexpected credentials: %#v", identity.Credentials)
	}
	if identity.ExpiresAt == nil || !identity.ExpiresAt.After(time.Now()) {
		t.Fatalf("expected future token expiry, got %v", identity.ExpiresAt)
	}
	if got, want := tokenRequest.Get("grant_type"), "authorization_code"; got != want {
		t.Fatalf("grant type = %q, want %q", got, want)
	}
	if got, want := tokenRequest.Get("code"), "authorization-code"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := tokenRequest.Get("code_verifier"), "pkce-verifier"; got != want {
		t.Fatalf("code verifier = %q, want %q", got, want)
	}
}

func TestDetectGatewayRejectsRemoteSidecar(t *testing.T) {
	_, err := NewHTTPBrokerGatewayClient("https://broker.example.com").DetectGateway(context.Background(), models.BrokerFutu)
	if err == nil {
		t.Fatal("remote gateway must be rejected")
	}
	var unavailable *BrokerGatewayUnavailableError
	if !asBrokerGatewayUnavailable(err, &unavailable) {
		t.Fatalf("expected typed unavailable error, got %T: %v", err, err)
	}
}

func TestDetectGatewayPostsOnlyConfiguredSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/broker/connection-status"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		var payload struct {
			Broker      string            `json:"broker"`
			Credentials map[string]string `json:"credentials"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got, want := payload.Broker, models.BrokerFutu; got != want {
			t.Fatalf("broker = %q, want %q", got, want)
		}
		if got, want := payload.Credentials, map[string]string{"host": "127.0.0.1", "port": "11111"}; !equalStringMap(got, want) {
			t.Fatalf("credentials = %#v, want %#v", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account_name": "Futu 12345678",
			"credentials":  payload.Credentials,
		})
	}))
	defer server.Close()

	client := NewHTTPBrokerGatewayClient(server.URL, map[string]config.BrokerGatewayConfig{
		models.BrokerFutu: {Host: "127.0.0.1", Port: "11111"},
	})
	identity, err := client.DetectGateway(context.Background(), models.BrokerFutu)
	if err != nil {
		t.Fatalf("detect gateway: %v", err)
	}
	if got, want := identity.AccountName, "Futu 12345678"; got != want {
		t.Fatalf("account name = %q, want %q", got, want)
	}
}

func newTestAuthorizer(t *testing.T) *HTTPBrokerAuthorizer {
	t.Helper()
	return newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		AuthorizeURL: "https://oauth.example/authorize", TokenURL: "https://oauth.example/token",
		AccountURL: "https://oauth.example/account", RedirectURL: "http://127.0.0.1:5173/settings",
		Scopes: []string{"account.read"},
	})
}

func newTestAuthorizerWithConfig(t *testing.T, oauth config.BrokerOAuthConfig) *HTTPBrokerAuthorizer {
	t.Helper()
	return NewHTTPBrokerAuthorizer(map[string]config.BrokerOAuthConfig{models.BrokerLongPort: oauth}, newTestVault(t), nil, time.Second)
}

func newTestVault(t *testing.T) *BrokerCredentialVault {
	t.Helper()
	vault, err := NewBrokerCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("new test vault: %v", err)
	}
	return vault
}

func equalStringMap(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			return false
		}
	}
	return true
}

func asBrokerGatewayUnavailable(err error, target **BrokerGatewayUnavailableError) bool {
	for err != nil {
		if unavailable, ok := err.(*BrokerGatewayUnavailableError); ok {
			*target = unavailable
			return true
		}
		type unwrapper interface{ Unwrap() error }
		wrapped, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = wrapped.Unwrap()
	}
	return false
}

func TestBeginOAuthRejectsUnconfiguredBroker(t *testing.T) {
	_, err := newTestAuthorizer(t).BeginOAuth(context.Background(), models.BrokerTiger)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured broker error, got %v", err)
	}
}
