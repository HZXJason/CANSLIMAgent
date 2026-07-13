package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/alphalens/canslimagent/backend/internal/models"
)

const (
	brokerOAuthSessionTTL     = 10 * time.Minute
	pkceVerifierCredentialKey = "pkce_verifier"
)

// BrokerAuthorizer starts and completes server-managed OAuth, or discovers a
// server-configured local broker gateway. Browser credentials never cross this
// interface.
type BrokerAuthorizer interface {
	BeginOAuth(context.Context, string) (BrokerOAuthStart, error)
	CompleteOAuth(context.Context, string, string, string) (BrokerAuthorizationIdentity, error)
	DetectGateway(context.Context, string) (BrokerGatewayIdentity, error)
}

// BrokerOAuthStart contains persistence-safe OAuth session data. The raw state
// exists only in AuthorizationURL; callers persist StateHash instead.
type BrokerOAuthStart struct {
	AuthorizationURL   string
	StateHash          []byte
	VerifierCiphertext []byte
	VerifierNonce      []byte
	ExpiresAt          time.Time
}

type BrokerAuthorizationIdentity struct {
	Credentials map[string]string
	AccountName string
	ExpiresAt   *time.Time
}

type BrokerGatewayIdentity struct {
	AccountName string
	Credentials map[string]string
}

// BrokerGatewayClient is the small local-sidecar boundary used for Futu and
// IBKR discovery.
type BrokerGatewayClient interface {
	DetectGateway(context.Context, string) (BrokerGatewayIdentity, error)
}

// HTTPBrokerAuthorizer uses configured OAuth endpoints. The vault is used only
// for the short-lived PKCE verifier returned to the handler for persistence.
type HTTPBrokerAuthorizer struct {
	oauthByBroker map[string]config.BrokerOAuthConfig
	vault         *BrokerCredentialVault
	client        *http.Client
	gateway       BrokerGatewayClient
	now           func() time.Time
}

func NewHTTPBrokerAuthorizer(oauthByBroker map[string]config.BrokerOAuthConfig, vault *BrokerCredentialVault, gateway BrokerGatewayClient, timeout time.Duration) *HTTPBrokerAuthorizer {
	configs := make(map[string]config.BrokerOAuthConfig, len(oauthByBroker))
	for broker, oauth := range oauthByBroker {
		configs[broker] = oauth
	}
	return &HTTPBrokerAuthorizer{
		oauthByBroker: configs,
		vault:         vault,
		client:        &http.Client{Timeout: timeout},
		gateway:       gateway,
		now:           time.Now,
	}
}

func (a *HTTPBrokerAuthorizer) BeginOAuth(_ context.Context, broker string) (BrokerOAuthStart, error) {
	if a.vault == nil {
		return BrokerOAuthStart{}, errors.New("broker credential vault is not configured")
	}
	oauth, err := a.oauthConfig(broker)
	if err != nil {
		return BrokerOAuthStart{}, err
	}
	if err := validateOAuthScopes(oauth.Scopes); err != nil {
		return BrokerOAuthStart{}, err
	}

	state, err := randomOAuthValue()
	if err != nil {
		return BrokerOAuthStart{}, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, err := randomOAuthValue()
	if err != nil {
		return BrokerOAuthStart{}, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifierCiphertext, verifierNonce, err := a.vault.Encrypt(map[string]string{pkceVerifierCredentialKey: verifier})
	if err != nil {
		return BrokerOAuthStart{}, fmt.Errorf("encrypt PKCE verifier: %w", err)
	}

	authorizationURL, err := buildOAuthAuthorizationURL(oauth, state, verifier)
	if err != nil {
		return BrokerOAuthStart{}, err
	}
	stateHash := sha256.Sum256([]byte(state))
	return BrokerOAuthStart{
		AuthorizationURL:   authorizationURL,
		StateHash:          stateHash[:],
		VerifierCiphertext: verifierCiphertext,
		VerifierNonce:      verifierNonce,
		ExpiresAt:          a.now().Add(brokerOAuthSessionTTL),
	}, nil
}

func (a *HTTPBrokerAuthorizer) CompleteOAuth(ctx context.Context, broker, code, verifier string) (BrokerAuthorizationIdentity, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(verifier) == "" {
		return BrokerAuthorizationIdentity{}, errors.New("OAuth code and PKCE verifier are required")
	}
	oauth, err := a.oauthConfig(broker)
	if err != nil {
		return BrokerAuthorizationIdentity{}, err
	}

	credentials, expiresAt, err := a.exchangeOAuthCode(ctx, oauth, code, verifier)
	if err != nil {
		return BrokerAuthorizationIdentity{}, err
	}
	accountName, err := a.fetchOAuthAccountName(ctx, broker, oauth, credentials["access_token"])
	if err != nil {
		return BrokerAuthorizationIdentity{}, err
	}
	return BrokerAuthorizationIdentity{Credentials: credentials, AccountName: accountName, ExpiresAt: expiresAt}, nil
}

func (a *HTTPBrokerAuthorizer) DetectGateway(ctx context.Context, broker string) (BrokerGatewayIdentity, error) {
	if a.gateway == nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: errors.New("broker gateway client is not configured")}
	}
	return a.gateway.DetectGateway(ctx, broker)
}

func (a *HTTPBrokerAuthorizer) oauthConfig(broker string) (config.BrokerOAuthConfig, error) {
	if broker != models.BrokerLongPort && broker != models.BrokerTiger {
		return config.BrokerOAuthConfig{}, fmt.Errorf("broker %q does not support OAuth", broker)
	}
	oauth, ok := a.oauthByBroker[broker]
	if !ok || !oauth.Enabled() {
		return config.BrokerOAuthConfig{}, fmt.Errorf("OAuth for broker %q is not configured", broker)
	}
	return oauth, nil
}

func (a *HTTPBrokerAuthorizer) exchangeOAuthCode(ctx context.Context, oauth config.BrokerOAuthConfig, code, verifier string) (map[string]string, *time.Time, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {oauth.RedirectURL},
		"client_id":     {oauth.ClientID},
	}
	if strings.TrimSpace(oauth.ClientSecret) != "" {
		form.Set("client_secret", oauth.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("exchange OAuth code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("OAuth token exchange failed: %s", resp.Status)
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, nil, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, nil, errors.New("OAuth token response did not include an access token")
	}
	credentials := map[string]string{"access_token": token.AccessToken}
	if strings.TrimSpace(token.RefreshToken) != "" {
		credentials["refresh_token"] = token.RefreshToken
	}
	var expiresAt *time.Time
	if token.ExpiresIn > 0 {
		expires := a.now().Add(time.Duration(token.ExpiresIn) * time.Second)
		expiresAt = &expires
	}
	return credentials, expiresAt, nil
}

func (a *HTTPBrokerAuthorizer) fetchOAuthAccountName(ctx context.Context, broker string, oauth config.BrokerOAuthConfig, accessToken string) (string, error) {
	if strings.TrimSpace(oauth.AccountURL) == "" {
		if broker == models.BrokerLongPort {
			return "长桥账户（已授权）", nil
		}
		return "", errors.New("OAuth account endpoint is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauth.AccountURL, nil)
	if err != nil {
		return "", fmt.Errorf("create OAuth account request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query OAuth account: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OAuth account query failed: %s", resp.Status)
	}

	var account struct {
		AccountName string `json:"account_name"`
		Account     string `json:"account"`
		Data        struct {
			AccountName string `json:"account_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return "", fmt.Errorf("decode OAuth account response: %w", err)
	}
	for _, candidate := range []string{account.AccountName, account.Account, account.Data.AccountName} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value, nil
		}
	}
	return "", errors.New("OAuth account response did not include an allowed account name")
}

func buildOAuthAuthorizationURL(oauth config.BrokerOAuthConfig, state, verifier string) (string, error) {
	parsed, err := url.Parse(oauth.AuthorizeURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("configured OAuth authorization URL is invalid")
	}
	challengeHash := sha256.Sum256([]byte(verifier))
	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", oauth.ClientID)
	query.Set("redirect_uri", oauth.RedirectURL)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challengeHash[:]))
	query.Set("code_challenge_method", "S256")
	if scopes := readOnlyScopes(oauth.Scopes); len(scopes) > 0 {
		query.Set("scope", strings.Join(scopes, " "))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func randomOAuthValue() (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func readOnlyScopes(scopes []string) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if strings.TrimSpace(scope) != "" {
			result = append(result, scope)
		}
	}
	return result
}

func validateOAuthScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("configured OAuth scopes must include at least one read-only scope")
	}
	for _, scope := range scopes {
		normalized := normalizeOAuthScope(scope)
		if normalized == "3" {
			continue
		}
		for _, forbidden := range []string{
			"write", "execute", "execution", "place_order", "order-placement",
			"order_placement", "order.write", "trade.write", "trading", "withdraw",
			"transfer", "cancel", "modify", "modification",
		} {
			if strings.Contains(normalized, forbidden) {
				return fmt.Errorf("configured OAuth scope %q grants a prohibited capability", scope)
			}
		}
		if !hasOAuthReadCapability(normalized) {
			return fmt.Errorf("configured OAuth scope %q must grant an explicit read capability", scope)
		}
	}
	return nil
}

func normalizeOAuthScope(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}

func hasOAuthReadCapability(scope string) bool {
	return scope == "read" ||
		strings.Contains(scope, ".read") ||
		strings.Contains(scope, ":read") ||
		strings.Contains(scope, "_read") ||
		strings.Contains(scope, "-read")
}

// BrokerGatewayUnavailableError marks a local broker-sidecar failure so the
// handler can persist gateway_unavailable instead of treating it as an OAuth
// authorization failure.
type BrokerGatewayUnavailableError struct {
	Broker string
	Err    error
}

func (e *BrokerGatewayUnavailableError) Error() string {
	if e.Broker == "" {
		return fmt.Sprintf("broker gateway unavailable: %v", e.Err)
	}
	return fmt.Sprintf("%s broker gateway unavailable: %v", e.Broker, e.Err)
}

func (e *BrokerGatewayUnavailableError) Unwrap() error { return e.Err }

// HTTPBrokerGatewayClient calls the local sidecar with only fixed, server-side
// gateway settings. It never accepts host, port, or client identifiers from a
// browser request.
type HTTPBrokerGatewayClient struct {
	url              string
	client           *http.Client
	gatewaysByBroker map[string]config.BrokerGatewayConfig
}

func NewHTTPBrokerGatewayClient(sidecarURL string, gatewayConfigs ...map[string]config.BrokerGatewayConfig) *HTTPBrokerGatewayClient {
	configs := map[string]config.BrokerGatewayConfig{}
	if len(gatewayConfigs) > 0 {
		for broker, gateway := range gatewayConfigs[0] {
			configs[broker] = gateway
		}
	}
	return &HTTPBrokerGatewayClient{
		url: strings.TrimRight(strings.TrimSpace(sidecarURL), "/") + "/broker/connection-status",
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		gatewaysByBroker: configs,
	}
}

func (c *HTTPBrokerGatewayClient) DetectGateway(ctx context.Context, broker string) (BrokerGatewayIdentity, error) {
	if err := validateLoopbackBrokerUpstream(c.url); err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: err}
	}
	settings, err := c.gatewaySettings(broker)
	if err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: err}
	}
	body, err := json.Marshal(struct {
		Broker      string            `json:"broker"`
		Credentials map[string]string `json:"credentials"`
	}{Broker: broker, Credentials: settings})
	if err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: fmt.Errorf("encode gateway status request: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: fmt.Errorf("create gateway status request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: fmt.Errorf("gateway status request failed: %s", resp.Status)}
	}
	var response struct {
		AccountName string `json:"account_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: fmt.Errorf("decode gateway status response: %w", err)}
	}
	if strings.TrimSpace(response.AccountName) == "" {
		return BrokerGatewayIdentity{}, &BrokerGatewayUnavailableError{Broker: broker, Err: errors.New("gateway status response did not include an account name")}
	}
	return BrokerGatewayIdentity{AccountName: strings.TrimSpace(response.AccountName), Credentials: settings}, nil
}

func (c *HTTPBrokerGatewayClient) gatewaySettings(broker string) (map[string]string, error) {
	if broker != models.BrokerFutu && broker != models.BrokerIBKR {
		return nil, fmt.Errorf("broker %q does not support a local gateway", broker)
	}
	gateway, ok := c.gatewaysByBroker[broker]
	if !ok {
		return nil, fmt.Errorf("server gateway settings for broker %q are not configured", broker)
	}
	if !isLoopbackBrokerHost(gateway.Host) {
		return nil, errors.New("broker gateway host must be loopback")
	}
	port, err := strconv.Atoi(strings.TrimSpace(gateway.Port))
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("broker gateway port is invalid")
	}
	settings := map[string]string{"host": strings.TrimSpace(gateway.Host), "port": strconv.Itoa(port)}
	if clientID := strings.TrimSpace(gateway.ClientID); clientID != "" {
		settings["client_id"] = clientID
	}
	if account := strings.TrimSpace(gateway.Account); account != "" {
		settings["account"] = account
	}
	return settings, nil
}

func isLoopbackBrokerHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
