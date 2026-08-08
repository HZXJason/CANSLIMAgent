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

	"github.com/alphalens/canslimagent/backend/internal/models"
)

const maxPreTradeReviewResponseBytes = 2 << 20

type HTTPPreTradeAIReviewer struct {
	endpoint string
	client   *http.Client
	secret   string
}

func NewHTTPPreTradeAIReviewer(upstreamURL string, timeout time.Duration, sharedSecret string) *HTTPPreTradeAIReviewer {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	base := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if base == "" {
		base = DefaultAIAnalysisUpstreamURL
	}
	if !strings.HasSuffix(base, "/trade-plans/review") {
		base += "/trade-plans/review"
	}
	return &HTTPPreTradeAIReviewer{endpoint: base, client: &http.Client{Timeout: timeout}, secret: strings.TrimSpace(sharedSecret)}
}

func (reviewer *HTTPPreTradeAIReviewer) Review(ctx context.Context, input PreTradeAIReviewInput) (PreTradeAIReviewOutput, error) {
	wire := struct {
		Ticker               string           `json:"ticker"`
		LLMProvider          string           `json:"llm_provider"`
		Model                string           `json:"model"`
		BackendURL           string           `json:"backend_url,omitempty"`
		APIKey               string           `json:"api_key,omitempty"`
		PlanSnapshot         map[string]any   `json:"plan_snapshot"`
		ResearchEvidence     map[string]any   `json:"research_evidence"`
		CurrentEvidence      map[string]any   `json:"current_evidence"`
		BehaviorObservations []map[string]any `json:"behavior_observations"`
		TradingProfile       map[string]any   `json:"trading_profile"`
		TradingPrinciples    []map[string]any `json:"trading_principles"`
		TradingPlaybooks     []map[string]any `json:"trading_playbooks"`
	}{
		Ticker: input.Ticker, LLMProvider: input.Provider, Model: input.Model, BackendURL: input.BackendURL, APIKey: input.APIKey,
		PlanSnapshot: input.PlanSnapshot, ResearchEvidence: input.ResearchEvidence, CurrentEvidence: input.CurrentEvidence, BehaviorObservations: input.BehaviorObservations,
		TradingProfile: input.TradingProfile, TradingPrinciples: input.TradingPrinciples, TradingPlaybooks: input.TradingPlaybooks,
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return PreTradeAIReviewOutput{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, reviewer.endpoint, bytes.NewReader(body))
	if err != nil {
		return PreTradeAIReviewOutput{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if reviewer.secret != "" {
		request.Header.Set("X-Sidecar-Token", reviewer.secret)
	}
	response, err := reviewer.client.Do(request)
	if err != nil {
		return PreTradeAIReviewOutput{}, err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxPreTradeReviewResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return PreTradeAIReviewOutput{}, err
	}
	if len(responseBody) > maxPreTradeReviewResponseBytes {
		return PreTradeAIReviewOutput{}, errors.New("pre-trade review response exceeds size limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(responseBody, &failure)
		if strings.TrimSpace(failure.Detail) == "" {
			failure.Detail = fmt.Sprintf("pre-trade review upstream returned HTTP %d", response.StatusCode)
		}
		return PreTradeAIReviewOutput{}, errors.New(strings.TrimSpace(failure.Detail))
	}
	var result PreTradeAIReviewOutput
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return PreTradeAIReviewOutput{}, errors.New("pre-trade review response is invalid")
	}
	if err := validatePreTradeAIReviewOutput(result); err != nil {
		return PreTradeAIReviewOutput{}, err
	}
	return result, nil
}

func validatePreTradeAIReviewOutput(result PreTradeAIReviewOutput) error {
	if result.RiskWarnings == nil || result.BehaviorReminders == nil || result.UnresolvedQuestions == nil || result.ChangesSinceResearch == nil || result.Freshness == nil || result.Sources == nil {
		return errors.New("pre-trade review response is incomplete")
	}
	sourceIDs := make(map[string]bool, len(result.Sources))
	for _, source := range result.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Label) == "" {
			return errors.New("pre-trade review source is invalid")
		}
		sourceIDs[source.ID] = true
	}
	groups := [][]models.PreTradeFinding{result.RiskWarnings, result.BehaviorReminders, result.UnresolvedQuestions, result.ChangesSinceResearch}
	for _, findings := range groups {
		if len(findings) > 20 {
			return errors.New("pre-trade review contains too many findings")
		}
		for _, finding := range findings {
			if strings.TrimSpace(finding.Code) == "" || strings.TrimSpace(finding.Message) == "" || strings.TrimSpace(finding.Category) == "" {
				return errors.New("pre-trade review finding is invalid")
			}
			if containsProhibitedTradeDecisionLanguage(finding.Message) {
				return errors.New("pre-trade review contains prohibited decision language")
			}
			if finding.Severity == "high" {
				rawIDs, ok := finding.Evidence["source_ids"].([]any)
				if !ok || len(rawIDs) == 0 {
					return errors.New("high-risk finding is missing source citations")
				}
				for _, rawID := range rawIDs {
					id, ok := rawID.(string)
					if !ok || !sourceIDs[id] {
						return errors.New("finding references an unknown source")
					}
				}
			}
		}
	}
	return nil
}

func containsProhibitedTradeDecisionLanguage(message string) bool {
	lower := strings.ToLower(message)
	for _, phrase := range []string{"批准", "计划通过", "交易通过", "可以买", "可以卖", "建议买入", "建议卖出", " buy ", " sell ", " hold "} {
		if strings.Contains(" "+lower+" ", phrase) {
			return true
		}
	}
	return false
}
