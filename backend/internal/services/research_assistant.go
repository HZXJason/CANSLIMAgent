package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResearchAssistantResponseBytes = 64 << 10

type ResearchAssistantInput struct {
	Question   string
	Market     string
	Provider   string
	Model      string
	BackendURL string
	APIKey     string
}

type ResearchAssistantOutput struct {
	Answer string `json:"answer"`
	Model  string `json:"model"`
}

type ResearchAssistant interface {
	Ask(context.Context, ResearchAssistantInput) (ResearchAssistantOutput, error)
}

type HTTPResearchAssistant struct {
	endpoint string
	client   *http.Client
	secret   string
}

func NewHTTPResearchAssistant(upstreamURL string, timeout time.Duration, sharedSecret string) *HTTPResearchAssistant {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	base := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if base == "" {
		base = DefaultAIAnalysisUpstreamURL
	}
	return &HTTPResearchAssistant{endpoint: base + "/assistant/ask", client: &http.Client{Timeout: timeout}, secret: strings.TrimSpace(sharedSecret)}
}

func (assistant *HTTPResearchAssistant) Ask(ctx context.Context, input ResearchAssistantInput) (ResearchAssistantOutput, error) {
	body, err := json.Marshal(map[string]string{"question": input.Question, "market": input.Market, "llm_provider": input.Provider, "model": input.Model, "backend_url": input.BackendURL, "api_key": input.APIKey})
	if err != nil {
		return ResearchAssistantOutput{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, assistant.endpoint, bytes.NewReader(body))
	if err != nil {
		return ResearchAssistantOutput{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if assistant.secret != "" {
		request.Header.Set("X-Sidecar-Token", assistant.secret)
	}
	response, err := assistant.client.Do(request)
	if err != nil {
		return ResearchAssistantOutput{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResearchAssistantResponseBytes+1))
	if err != nil {
		return ResearchAssistantOutput{}, err
	}
	if len(responseBody) > maxResearchAssistantResponseBytes {
		return ResearchAssistantOutput{}, errors.New("research assistant response exceeds size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ResearchAssistantOutput{}, fmt.Errorf("research assistant upstream returned HTTP %d", response.StatusCode)
	}
	var result ResearchAssistantOutput
	if err := json.Unmarshal(responseBody, &result); err != nil || strings.TrimSpace(result.Answer) == "" || strings.TrimSpace(result.Model) == "" {
		return ResearchAssistantOutput{}, errors.New("research assistant response is invalid")
	}
	return result, nil
}
