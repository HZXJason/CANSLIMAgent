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

func TestCompleteOAuthSupportsLongbridgePublicPKCEClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/token"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if got := r.PostForm.Get("client_secret"); got != "" {
			t.Fatalf("public client sent client_secret %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-token"})
	}))
	defer server.Close()

	authorizer := newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "public-client-id", AuthorizeURL: server.URL + "/authorize",
		TokenURL: server.URL + "/token", RedirectURL: "http://127.0.0.1:8080/api/v1/brokers/oauth/longport/callback",
		Scopes: []string{"3"},
	})

	identity, err := authorizer.CompleteOAuth(context.Background(), models.BrokerLongPort, "authorization-code", "pkce-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := identity.AccountName, "长桥账户（已授权）"; got != want {
		t.Fatalf("account name = %q, want %q", got, want)
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
		if got, want := payload.Credentials, map[string]string{"host": "127.0.0.1", "port": "11111", "account": "FUTU-001"}; !equalStringMap(got, want) {
			t.Fatalf("credentials = %#v, want %#v", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account_name": "Futu 12345678",
			"credentials": map[string]string{
				"host": "sidecar.example.com",
				"port": "443",
			},
		})
	}))
	defer server.Close()

	client := NewHTTPBrokerGatewayClient(server.URL, map[string]config.BrokerGatewayConfig{
		models.BrokerFutu: {Host: "127.0.0.1", Port: "11111", Account: "FUTU-001"},
	})
	identity, err := client.DetectGateway(context.Background(), models.BrokerFutu)
	if err != nil {
		t.Fatalf("detect gateway: %v", err)
	}
	if got, want := identity.AccountName, "Futu 12345678"; got != want {
		t.Fatalf("account name = %q, want %q", got, want)
	}
	if got, want := identity.Credentials, map[string]string{"host": "127.0.0.1", "port": "11111", "account": "FUTU-001"}; !equalStringMap(got, want) {
		t.Fatalf("identity credentials = %#v, want original server settings %#v", got, want)
	}
}

func TestDetectGatewayDoesNotFollowRedirectToRemoteTarget(t *testing.T) {
	remoteRequests := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteRequests <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer remote.Close()

	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, remote.URL, http.StatusFound)
	}))
	defer sidecar.Close()

	client := NewHTTPBrokerGatewayClient(sidecar.URL, map[string]config.BrokerGatewayConfig{
		models.BrokerFutu: {Host: "127.0.0.1", Port: "11111"},
	})
	_, err := client.DetectGateway(context.Background(), models.BrokerFutu)
	if err == nil {
		t.Fatal("redirected gateway detection must fail")
	}

	select {
	case <-remoteRequests:
		t.Fatal("redirect target received a request")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGatewaySettingsPassesConfiguredIBKRAccountToSidecar(t *testing.T) {
	client := NewHTTPBrokerGatewayClient("http://127.0.0.1:8765", map[string]config.BrokerGatewayConfig{
		models.BrokerIBKR: {Host: "127.0.0.1", Port: "7497", ClientID: "9901", Account: "DU123456"},
	})

	settings, err := client.gatewaySettings(models.BrokerIBKR)
	if err != nil {
		t.Fatalf("gateway settings: %v", err)
	}
	if got, want := settings, map[string]string{"host": "127.0.0.1", "port": "7497", "client_id": "9901", "account": "DU123456"}; !equalStringMap(got, want) {
		t.Fatalf("settings = %#v, want %#v", got, want)
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

func TestBeginOAuthRejectsWriteScopeBeforeCreatingAuthorizationURL(t *testing.T) {
	authorizer := newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		AuthorizeURL: "://invalid", TokenURL: "https://oauth.example/token",
		AccountURL: "https://oauth.example/account", RedirectURL: "http://127.0.0.1:5173/settings",
		Scopes: []string{"account.read", "orders.write"},
	})

	_, err := authorizer.BeginOAuth(context.Background(), models.BrokerLongPort)
	if err == nil || !strings.Contains(err.Error(), "orders.write") {
		t.Fatalf("expected rejected write scope error, got %v", err)
	}
}

func TestBeginOAuthRejectsOrderPlacementScopeBeforeCreatingAuthorizationURL(t *testing.T) {
	authorizer := newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		AuthorizeURL: "://invalid", TokenURL: "https://oauth.example/token",
		AccountURL: "https://oauth.example/account", RedirectURL: "http://127.0.0.1:5173/settings",
		Scopes: []string{"account.read", "order-placement"},
	})

	_, err := authorizer.BeginOAuth(context.Background(), models.BrokerLongPort)
	if err == nil || !strings.Contains(err.Error(), "order-placement") {
		t.Fatalf("expected rejected order-placement scope error, got %v", err)
	}
}

func TestBeginOAuthRejectsEmptyScopesBeforeCreatingAuthorizationURL(t *testing.T) {
	authorizer := newTestAuthorizerWithConfig(t, config.BrokerOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret",
		AuthorizeURL: "://invalid", TokenURL: "https://oauth.example/token",
		AccountURL: "https://oauth.example/account", RedirectURL: "http://127.0.0.1:5173/settings",
		Scopes: []string{},
	})

	_, err := authorizer.BeginOAuth(context.Background(), models.BrokerLongPort)
	if err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected empty scopes error, got %v", err)
	}
}
