package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/gin-gonic/gin"
)

func TestProductionAndRemoteBindRequireSecrets(t *testing.T) {
	if err := validateRuntimeSecurity(config.Config{Host: "0.0.0.0", AppEnv: "development"}); err == nil {
		t.Fatal("remote bind without an API token was accepted")
	}
	if err := validateRuntimeSecurity(config.Config{
		Host:                "127.0.0.1",
		AppEnv:              "production",
		APIAuthToken:        strings.Repeat("a", 32),
		SidecarSharedSecret: strings.Repeat("s", 32),
	}); err != nil {
		t.Fatalf("valid production security config was rejected: %v", err)
	}
}

func TestConfiguredOriginMatcherRejectsLookalikeDevelopmentOrigin(t *testing.T) {
	allowed := configuredOriginAllowed([]string{"http://127.0.0.1:5173"})
	if !allowed("http://127.0.0.1:5173") {
		t.Fatal("configured origin was rejected")
	}
	for _, origin := range []string{
		"http://evil.example:5173",
		"http://127.0.0.1:5174",
		"null",
		"https://127.0.0.1:5173",
	} {
		if allowed(origin) {
			t.Fatalf("unconfigured origin %q was accepted", origin)
		}
	}
}

func TestInternalAPIAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(internalAPIAuth("a-secret-long-enough-for-testing"))
	router.GET("/api/v1/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/private", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/api/v1/brokers/oauth/:broker/callback", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, path := range []string{"/api/v1/health", "/api/v1/brokers/oauth/longport/callback"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code >= http.StatusBadRequest {
			t.Fatalf("public route %s returned %d", path, response.Code)
		}
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/private", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing token returned %d, want 401", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/private", nil)
	authorizedRequest.Header.Set(internalAPITokenHeader, "a-secret-long-enough-for-testing")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("valid token returned %d, want 204", authorized.Code)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(limitRequestBody(4))
	router.POST("/", func(c *gin.Context) {
		_, _ = c.GetRawData()
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345")))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request returned %d, want 413", response.Code)
	}
}

func TestSafeAccessLoggerOmitsOAuthCallbackQueryAndLogsOtherRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousWriter := gin.DefaultWriter
	var accessLogs bytes.Buffer
	gin.DefaultWriter = &accessLogs
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
	})

	router := gin.New()
	router.Use(safeAccessLogger())
	router.GET("/api/v1/brokers/oauth/:broker/callback", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/v1/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	callbackURL := "/api/v1/brokers/oauth/longport/callback?code=authorization-code-secret&state=oauth-state-secret"
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, callbackURL, nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health?probe=normal", nil))

	logs := accessLogs.String()
	for _, forbidden := range []string{callbackURL, "code=", "state=", "authorization-code-secret", "oauth-state-secret"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("access logs leaked OAuth callback query data %q: %s", forbidden, logs)
		}
	}
	if !strings.Contains(logs, `"path":"/api/v1/health"`) {
		t.Fatalf("normal request was not access logged: %s", logs)
	}
	if strings.Contains(logs, "probe=normal") {
		t.Fatalf("access logs included query parameters: %s", logs)
	}
}
