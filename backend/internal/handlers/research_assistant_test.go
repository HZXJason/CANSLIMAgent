package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type capturingResearchAssistant struct {
	input services.ResearchAssistantInput
}

func (assistant *capturingResearchAssistant) Ask(_ context.Context, input services.ResearchAssistantInput) (services.ResearchAssistantOutput, error) {
	assistant.input = input
	return services.ResearchAssistantOutput{Answer: "CPI 会改变利率预期。", Model: input.Model}, nil
}

func TestResearchAssistantHandlerValidatesAndForwardsQuestion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assistant := &capturingResearchAssistant{}
	router := gin.New()
	router.POST("/api/v1/assistant/ask", NewResearchAssistantHandler(assistant, nil).Ask)

	bad := httptest.NewRecorder()
	router.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/assistant/ask", strings.NewReader(`{"question":"","market":"US","llm_provider":"openai","model":"gpt-test"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error, got %d", bad.Code)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assistant/ask", strings.NewReader(`{"question":"CPI 有什么影响？","market":"US","llm_provider":"openai","model":"gpt-test"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || assistant.input.Question != "CPI 有什么影响？" || assistant.input.Model != "gpt-test" {
		t.Fatalf("unexpected response/input: %d %s %+v", response.Code, response.Body.String(), assistant.input)
	}
}
