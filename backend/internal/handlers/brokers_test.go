package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConnectOAuthReturnsRedirectWithoutEchoingCredentials(t *testing.T) {
	router, _ := setupBrokerRouter(t, &fakeOAuthAuthorizer{start: services.BrokerOAuthStart{
		AuthorizationURL:   "https://broker.example/authorize?state=browser-only-state",
		StateHash:          []byte("state-hash"),
		VerifierCiphertext: []byte("encrypted-verifier"),
		VerifierNonce:      []byte("nonce"),
		ExpiresAt:          time.Now().Add(10 * time.Minute),
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/brokers/longport/connect", strings.NewReader(`{"credentials":{"access_token":"must-not-be-read"}}`))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "must-not-be-read") {
		t.Fatal(res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"action":"redirect"`) {
		t.Fatal("missing redirect action")
	}
}

func TestOAuthCallbackRejectsExpiredState(t *testing.T) {
	router, database := setupBrokerRouter(t, &fakeOAuthAuthorizer{})
	rawState := "expired-state"
	digest := sha256.Sum256([]byte(rawState))
	if err := database.Create(&models.BrokerAuthorizationSession{Broker: models.BrokerLongPort, StateHash: digest[:], PKCEVerifierCiphertext: []byte("unused"), PKCEVerifierNonce: []byte("unused"), ExpiresAt: time.Now().Add(-time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/brokers/oauth/longport/callback?code=code&state="+rawState, nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("got %d", res.Code)
	}
}

func TestOAuthCallbackCompletesOnceAndStoresOnlyEncryptedCredentials(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{identity: services.BrokerAuthorizationIdentity{
		AccountName: "LongPort 1234",
		Credentials: map[string]string{"access_token": "server-only-token"},
	}}
	router, database := setupBrokerRouter(t, authorizer)
	verifierCiphertext, verifierNonce, err := testBrokerVault(t).Encrypt(map[string]string{"pkce_verifier": "server-only-verifier"})
	if err != nil {
		t.Fatal(err)
	}
	rawState := "valid-state"
	digest := sha256.Sum256([]byte(rawState))
	if err := database.Create(&models.BrokerAuthorizationSession{
		Broker:                 models.BrokerLongPort,
		StateHash:              digest[:],
		PKCEVerifierCiphertext: verifierCiphertext,
		PKCEVerifierNonce:      verifierNonce,
		ExpiresAt:              time.Now().Add(time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/brokers/oauth/longport/callback?code=code&state="+rawState, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != "/settings?broker=longport" {
		t.Fatalf("Location = %q", got)
	}
	if authorizer.verifier != "server-only-verifier" {
		t.Fatalf("CompleteOAuth verifier = %q", authorizer.verifier)
	}

	var connection models.BrokerConnection
	if err := database.Where("broker = ?", models.BrokerLongPort).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if connection.ConnectionStatus != models.BrokerConnectionStatusConnected || connection.AccountName != "LongPort 1234" || connection.ConnectionMethod != "oauth" {
		t.Fatalf("unexpected connection: %#v", connection)
	}
	if strings.Contains(string(connection.CredentialCiphertext), "server-only-token") {
		t.Fatal("credentials were stored in plaintext")
	}

	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, "/api/v1/brokers/oauth/longport/callback?code=code&state="+rawState, nil))
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("replay got %d", replay.Code)
	}
}

func TestOAuthCallbackDenialClearsPendingConnectionAndRedirects(t *testing.T) {
	router, database := setupBrokerRouter(t, &fakeOAuthAuthorizer{})
	if err := database.Create(&models.BrokerConnection{
		Broker:               models.BrokerTiger,
		Label:                brokerDisplayName(models.BrokerTiger),
		CredentialCiphertext: []byte{},
		CredentialNonce:      []byte{},
		ConnectionMethod:     "oauth",
		ConnectionStatus:     models.BrokerConnectionStatusPending,
	}).Error; err != nil {
		t.Fatal(err)
	}
	rawState := "denied-state"
	digest := sha256.Sum256([]byte(rawState))
	if err := database.Create(&models.BrokerAuthorizationSession{
		Broker:                 models.BrokerTiger,
		StateHash:              digest[:],
		PKCEVerifierCiphertext: []byte("unused"),
		PKCEVerifierNonce:      make([]byte, 12),
		ExpiresAt:              time.Now().Add(time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/brokers/oauth/tiger/callback?state="+rawState+"&error=access_denied", nil))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/settings?broker=tiger&connection_error=access_denied" {
		t.Fatalf("got %d Location=%q", response.Code, response.Header().Get("Location"))
	}
	var connection models.BrokerConnection
	if err := database.Where("broker = ?", models.BrokerTiger).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if connection.ConnectionStatus != models.BrokerConnectionStatusUnconnected || connection.AccountName != "" || connection.ConnectionMethod != "" {
		t.Fatalf("pending metadata was not cleared: %#v", connection)
	}
}

func TestConnectOAuthDisabledReturnsConfigurationConflict(t *testing.T) {
	router, _ := setupBrokerRouter(t, &fakeOAuthAuthorizer{startErr: errors.New("OAuth for broker \"longport\" is not configured")})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/brokers/longport/connect", nil))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "OAuth 开发者应用尚未配置") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestGatewayDetectionStoresStateOnlyConnection(t *testing.T) {
	authorizer := &fakeOAuthAuthorizer{gateway: services.BrokerGatewayIdentity{
		AccountName: "Futu 12345678",
		Credentials: map[string]string{"host": "127.0.0.1", "port": "11111"},
	}}
	router, database := setupBrokerRouter(t, authorizer)
	connect := httptest.NewRecorder()
	router.ServeHTTP(connect, httptest.NewRequest(http.MethodPost, "/api/v1/brokers/futu/connect", nil))
	if connect.Code != http.StatusOK || !strings.Contains(connect.Body.String(), `"action":"gateway_detection"`) {
		t.Fatalf("connect: %d %s", connect.Code, connect.Body.String())
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/brokers/futu/detect", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "127.0.0.1") || !strings.Contains(response.Body.String(), `"account_name":"Futu 12345678"`) {
		t.Fatalf("detect: %d %s", response.Code, response.Body.String())
	}
	var connection models.BrokerConnection
	if err := database.Where("broker = ?", models.BrokerFutu).First(&connection).Error; err != nil {
		t.Fatal(err)
	}
	if connection.ConnectionStatus != models.BrokerConnectionStatusConnected || connection.ConnectionMethod != "gateway" {
		t.Fatalf("unexpected gateway connection: %#v", connection)
	}
}

func setupBrokerRouter(t *testing.T, authorizer services.BrokerAuthorizer) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.BrokerConnection{}, &models.BrokerAuthorizationSession{}); err != nil {
		t.Fatal(err)
	}
	handler := NewBrokerHandler(database, testBrokerVault(t), fakeBrokerSyncer{}, authorizer)
	router := gin.New()
	api := router.Group("/api/v1")
	api.GET("/brokers", handler.List)
	api.POST("/brokers/:broker/connect", handler.Connect)
	api.GET("/brokers/oauth/:broker/callback", handler.OAuthCallback)
	api.POST("/brokers/:broker/detect", handler.Detect)
	return router, database
}

func testBrokerVault(t *testing.T) *services.BrokerCredentialVault {
	t.Helper()
	vault, err := services.NewBrokerCredentialVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return vault
}

type fakeOAuthAuthorizer struct {
	start      services.BrokerOAuthStart
	startErr   error
	identity   services.BrokerAuthorizationIdentity
	complete   error
	gateway    services.BrokerGatewayIdentity
	gatewayErr error
	verifier   string
}

func (f *fakeOAuthAuthorizer) BeginOAuth(context.Context, string) (services.BrokerOAuthStart, error) {
	return f.start, f.startErr
}

func (f *fakeOAuthAuthorizer) CompleteOAuth(_ context.Context, _ string, _ string, verifier string) (services.BrokerAuthorizationIdentity, error) {
	f.verifier = verifier
	return f.identity, f.complete
}

func (f *fakeOAuthAuthorizer) DetectGateway(context.Context, string) (services.BrokerGatewayIdentity, error) {
	return f.gateway, f.gatewayErr
}

type fakeBrokerSyncer struct{}

func (fakeBrokerSyncer) Sync(context.Context, services.BrokerSyncRequest) (services.BrokerSyncResult, error) {
	return services.BrokerSyncResult{}, nil
}
