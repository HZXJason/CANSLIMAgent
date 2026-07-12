package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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
	if !strings.Contains(logs, "/api/v1/health?probe=normal") {
		t.Fatalf("normal request was not access logged: %s", logs)
	}
}
