package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type ResearchAssistantHandler struct {
	assistant   services.ResearchAssistant
	credentials *services.LLMCredentialStore
}

type researchAssistantRequest struct {
	Question    string `json:"question"`
	Market      string `json:"market"`
	LLMProvider string `json:"llm_provider"`
	Model       string `json:"model"`
	BackendURL  string `json:"backend_url"`
}

func NewResearchAssistantHandler(assistant services.ResearchAssistant, credentials *services.LLMCredentialStore) *ResearchAssistantHandler {
	return &ResearchAssistantHandler{assistant: assistant, credentials: credentials}
}

func (h *ResearchAssistantHandler) Ask(c *gin.Context) {
	var request researchAssistantRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}
	request.Question = strings.TrimSpace(request.Question)
	request.Market = strings.ToUpper(strings.TrimSpace(request.Market))
	request.LLMProvider = strings.ToLower(strings.TrimSpace(request.LLMProvider))
	request.Model = strings.TrimSpace(request.Model)
	request.BackendURL = strings.TrimSpace(request.BackendURL)
	if request.Question == "" || len([]rune(request.Question)) > 500 || (request.Market != "US" && request.Market != "CN") || request.LLMProvider == "" || request.Model == "" {
		Fail(c, http.StatusBadRequest, "question, market, llm_provider and model are required")
		return
	}
	if h.assistant == nil {
		Fail(c, http.StatusServiceUnavailable, "research assistant is unavailable")
		return
	}
	apiKey := ""
	var err error
	if h.credentials != nil {
		apiKey, err = h.credentials.Resolve(c.Request.Context(), request.LLMProvider, request.BackendURL)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "stored LLM credential could not be loaded")
			return
		}
	}
	result, err := h.assistant.Ask(c.Request.Context(), services.ResearchAssistantInput{Question: request.Question, Market: request.Market, Provider: request.LLMProvider, Model: request.Model, BackendURL: request.BackendURL, APIKey: apiKey})
	if err != nil {
		Fail(c, http.StatusBadGateway, "research assistant request failed")
		return
	}
	OK(c, result)
}
