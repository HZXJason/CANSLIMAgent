package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLLMCredentialHandlerSavesStatusAndDeletesWithoutEchoingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.LLMCredential{}); err != nil {
		t.Fatal(err)
	}
	store := services.NewLLMCredentialStore(database, testBrokerVault(t))
	handler := NewLLMCredentialHandler(store)
	router := gin.New()
	router.GET("/api/v1/llm/credentials/:provider", handler.Status)
	router.PUT("/api/v1/llm/credentials/:provider", handler.Save)
	router.DELETE("/api/v1/llm/credentials/:provider", handler.Delete)

	save := httptest.NewRecorder()
	saveRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/llm/credentials/openai_compatible",
		strings.NewReader(`{"backend_url":"https://gateway.example.com/v1","api_key":"sk-never-echo-this"}`),
	)
	saveRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(save, saveRequest)
	if save.Code != http.StatusOK || strings.Contains(save.Body.String(), "sk-never-echo-this") {
		t.Fatalf("unsafe save response: %d %s", save.Code, save.Body.String())
	}

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/llm/credentials/openai_compatible?backend_url=https%3A%2F%2Fgateway.example.com%2Fv1", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"configured":true`) {
		t.Fatalf("unexpected status response: %d %s", status.Code, status.Body.String())
	}
	var envelope struct {
		Data services.LLMCredentialStatus `json:"data"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &envelope); err != nil || envelope.Data.BackendHost != "gateway.example.com" {
		t.Fatalf("unexpected status payload: %+v / %v", envelope, err)
	}

	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/v1/llm/credentials/openai_compatible?backend_url=https%3A%2F%2Fgateway.example.com%2Fv1", nil))
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
}
