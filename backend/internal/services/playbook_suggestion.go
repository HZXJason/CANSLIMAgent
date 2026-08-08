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

type PlaybookSuggestionInput struct {
	LLMProvider  string
	Model        string
	BackendURL   string
	APIKey       string
	Reviews      []models.TradeReview
	Observations []models.BehaviorObservation
}

type PlaybookSuggestion struct {
	Name              string    `json:"name"`
	Markets           []string  `json:"markets"`
	Horizons          []string  `json:"horizons"`
	MarketContext     string    `json:"market_context"`
	EntryRules        string    `json:"entry_rules"`
	InvalidationRules string    `json:"invalidation_rules"`
	StopRules         string    `json:"stop_rules"`
	TargetRules       string    `json:"target_rules"`
	PositionRules     string    `json:"position_rules"`
	NoTradeRules      string    `json:"no_trade_rules"`
	Notes             string    `json:"notes"`
	EvidenceReviewIDs []string  `json:"evidence_review_ids"`
	EvidenceCount     int       `json:"evidence_count"`
	GeneratedAt       time.Time `json:"generated_at"`
}

type PlaybookSuggester interface {
	Suggest(context.Context, PlaybookSuggestionInput) ([]PlaybookSuggestion, error)
}

type HTTPPlaybookSuggester struct {
	endpoint string
	client   *http.Client
	secret   string
}

func NewHTTPPlaybookSuggester(upstreamURL string, timeout time.Duration, sharedSecret string) *HTTPPlaybookSuggester {
	base := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &HTTPPlaybookSuggester{endpoint: base + "/profile/playbooks/suggest", client: &http.Client{Timeout: timeout}, secret: strings.TrimSpace(sharedSecret)}
}

func (s *HTTPPlaybookSuggester) Suggest(ctx context.Context, input PlaybookSuggestionInput) ([]PlaybookSuggestion, error) {
	wire := map[string]any{"llm_provider": input.LLMProvider, "model": input.Model, "backend_url": input.BackendURL, "api_key": input.APIKey, "reviews": input.Reviews, "observations": input.Observations}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if s.secret != "" {
		request.Header.Set("X-Sidecar-Token", s.secret)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Suggestions []PlaybookSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, errors.New("playbook suggestion response is invalid")
	}
	if result.Suggestions == nil {
		return nil, errors.New("playbook suggestion response is incomplete")
	}
	for _, item := range result.Suggestions {
		if strings.TrimSpace(item.Name) == "" || item.EvidenceCount != len(item.EvidenceReviewIDs) || item.GeneratedAt.IsZero() {
			return nil, errors.New("playbook suggestion is missing evidence metadata")
		}
	}
	return result.Suggestions, nil
}
