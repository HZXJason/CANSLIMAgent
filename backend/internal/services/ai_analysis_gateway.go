package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultAIAnalysisUpstreamURL = "http://127.0.0.1:8765"
	DefaultAIAnalysisTimeout     = 20 * time.Minute

	AIAnalysisContractVersion = "ai-analysis.v1"
)

type AIAnalysisRequest struct {
	Ticker          string         `json:"ticker"`
	LLMProvider     string         `json:"llm_provider"`
	DeepThinkLLM    string         `json:"deep_think_llm"`
	QuickThinkLLM   string         `json:"quick_think_llm"`
	BackendURL      string         `json:"backend_url"`
	MaxDebateRounds int            `json:"max_debate_rounds,omitempty"`
	MaxRiskRounds   int            `json:"max_risk_rounds,omitempty"`
	OnlineTools     bool           `json:"online_tools"`
	Analysts        []string       `json:"analysts"`
	AnalysisContext map[string]any `json:"analysis_context,omitempty"`
	RunID           string         `json:"run_id,omitempty"`
	Resume          bool           `json:"resume,omitempty"`
	LastEventID     int            `json:"last_event_id,omitempty"`
	APIKey          string         `json:"-"`
}

type AICalibrationRequest struct {
	AIAnalysisRequest
	JobID        string                      `json:"job_id,omitempty"`
	SampleCount  int                         `json:"sample_count,omitempty"`
	HorizonDays  int                         `json:"horizon_days,omitempty"`
	ForceRefresh bool                        `json:"force_refresh,omitempty"`
	Progress     func(AICalibrationProgress) `json:"-"`
}

type AICalibrationProgress struct {
	CompletedNodes int `json:"completed_nodes"`
	TotalNodes     int `json:"total_nodes"`
	CachedNodes    int `json:"cached_nodes"`
	FailedNodes    int `json:"failed_nodes"`
}

type AICalibrationResult struct {
	SchemaVersion           string           `json:"schema_version"`
	Symbol                  string           `json:"symbol"`
	Status                  string           `json:"status"`
	SampleCount             int              `json:"sample_count"`
	TotalNodeCount          int              `json:"total_node_count"`
	UnscorableCount         int              `json:"unscorable_count"`
	Coverage                float64          `json:"coverage"`
	CorrectCount            int              `json:"correct_count"`
	Accuracy                float64          `json:"accuracy"`
	OpportunityCorrectCount int              `json:"opportunity_correct_count"`
	OpportunityAccuracy     float64          `json:"opportunity_accuracy"`
	ExecutionCorrectCount   int              `json:"execution_correct_count"`
	ExecutionAccuracy       float64          `json:"execution_accuracy"`
	Baselines               map[string]any   `json:"baselines"`
	OpportunityLift         float64          `json:"opportunity_lift_vs_best_baseline"`
	ExecutionLift           float64          `json:"execution_lift_vs_best_baseline"`
	Confidence              string           `json:"confidence"`
	HorizonDays             int              `json:"horizon_days"`
	Benchmark               string           `json:"benchmark"`
	NeutralAlphaBand        float64          `json:"neutral_alpha_band"`
	ScoringRule             string           `json:"scoring_rule"`
	NeutralCountsAsCorrect  bool             `json:"neutral_counts_as_correct"`
	PointInTimeAnalysts     []string         `json:"point_in_time_analysts"`
	ExcludedAnalysts        []string         `json:"excluded_analysts"`
	Nodes                   []map[string]any `json:"nodes"`
}

type AIAbalationRequest struct {
	AIAnalysisRequest
	JobID                string                    `json:"job_id,omitempty"`
	StartDate            string                    `json:"start_date"`
	EndDate              string                    `json:"end_date"`
	HorizonDays          int                       `json:"horizon_days"`
	MaximumNodes         int                       `json:"maximum_nodes"`
	Repeats              int                       `json:"repeats"`
	Concurrency          int                       `json:"concurrency"`
	TransactionCostBP    float64                   `json:"transaction_cost_bps"`
	MaxAttempts          int                       `json:"max_attempts"`
	AdaptiveStages       bool                      `json:"adaptive_stages"`
	StageOneNodes        int                       `json:"stage_one_nodes"`
	ReproducibilityNodes int                       `json:"reproducibility_nodes"`
	Progress             func(AIAbalationProgress) `json:"-"`
}

type AIAbalationProgress struct {
	Status           string `json:"status"`
	StudyID          string `json:"study_id"`
	CompletedRuns    int    `json:"completed_runs"`
	TotalRuns        int    `json:"total_runs"`
	CachedRuns       int    `json:"cached_runs"`
	FailedRuns       int    `json:"failed_runs"`
	IndependentNodes int    `json:"independent_nodes"`
	ScheduledRuns    int    `json:"scheduled_runs"`
	MaximumRuns      int    `json:"maximum_runs"`
	Stage            int    `json:"stage"`
}

type AIAbalationResult map[string]any

type AIAbalationUpstream interface {
	Ablate(ctx context.Context, request AIAbalationRequest) (AIAbalationResult, error)
	CancelAblation(ctx context.Context, jobID string) error
}

type AIAnalysisProviderConfig struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Models            AIAnalysisModelSets `json:"models"`
	CustomModels      bool                `json:"custom_models,omitempty"`
	DefaultBackendURL string              `json:"default_backend_url,omitempty"`
}

type AIAnalysisModelSets struct {
	Deep  []string `json:"deep"`
	Quick []string `json:"quick"`
}

type AIAnalysisAnalystConfig struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	DefaultEnabled *bool  `json:"default_enabled,omitempty"`
}

type AIAnalysisContract struct {
	Version        string   `json:"version"`
	ConfigEndpoint string   `json:"config_endpoint"`
	ModelsEndpoint string   `json:"models_endpoint"`
	StreamEndpoint string   `json:"stream_endpoint"`
	UpstreamMode   string   `json:"upstream_mode"`
	Events         []string `json:"events"`
}

type AIModelDiscoveryRequest struct {
	Provider   string `json:"provider"`
	BackendURL string `json:"backend_url,omitempty"`
	APIKey     string `json:"-"`
}

type AIModelDiscoveryResponse struct {
	Models []string `json:"models"`
	Source string   `json:"source"`
}

type AIAnalysisConfigResponse struct {
	Contract     AIAnalysisContract         `json:"contract"`
	Providers    []AIAnalysisProviderConfig `json:"providers"`
	Analysts     []AIAnalysisAnalystConfig  `json:"analysts"`
	DebateRounds []int                      `json:"debate_rounds"`
	RiskRounds   []int                      `json:"risk_rounds"`
}

type AIAnalysisStream struct {
	Body       io.ReadCloser
	StatusCode int
	Header     http.Header
}

type AIAnalysisUpstream interface {
	Config(ctx context.Context) (AIAnalysisConfigResponse, error)
	Models(ctx context.Context, request AIModelDiscoveryRequest) (AIModelDiscoveryResponse, error)
	Start(ctx context.Context, request AIAnalysisRequest) (*AIAnalysisStream, error)
	Calibrate(ctx context.Context, request AICalibrationRequest) (AICalibrationResult, error)
}

type AICalibrationCanceler interface {
	CancelCalibration(ctx context.Context, jobID string) error
}

func (u *HTTPAIAnalysisUpstream) CancelCalibration(ctx context.Context, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("calibration job id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAIAnalysisBaseURL(u.analyzeURL)+"/calibrate/"+url.PathEscape(jobID)+"/cancel", nil)
	if err != nil {
		return err
	}
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("stop sidecar calibration: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (u *HTTPAIAnalysisUpstream) CancelAblation(ctx context.Context, jobID string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("ablation job id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAIAnalysisBaseURL(u.analyzeURL)+"/ablate/"+url.PathEscape(jobID)+"/cancel", nil)
	if err != nil {
		return err
	}
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("stop sidecar ablation: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (u *HTTPAIAnalysisUpstream) Ablate(ctx context.Context, payload AIAbalationRequest) (AIAbalationResult, error) {
	result := AIAbalationResult{}
	wirePayload := struct {
		AIAbalationRequest
		APIKey string `json:"api_key,omitempty"`
	}{AIAbalationRequest: payload, APIKey: payload.APIKey}
	body, err := json.Marshal(wirePayload)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAIAnalysisBaseURL(u.analyzeURL)+"/ablate", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson, application/json")
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}
	client := *u.client
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(body))
		var failure struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(body, &failure) == nil && strings.TrimSpace(failure.Detail) != "" {
			detail = strings.TrimSpace(failure.Detail)
		}
		return result, fmt.Errorf("%s", detail)
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 128<<20))
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	for scanner.Scan() {
		var event struct {
			Type     string              `json:"type"`
			Progress AIAbalationProgress `json:"progress"`
			Result   AIAbalationResult   `json:"result"`
			Error    string              `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return result, fmt.Errorf("decode ablation progress: %w", err)
		}
		switch event.Type {
		case "progress":
			if payload.Progress != nil {
				payload.Progress(event.Progress)
			}
		case "result":
			return event.Result, nil
		case "error":
			if strings.TrimSpace(event.Error) == "" {
				event.Error = "sidecar ablation failed"
			}
			return result, fmt.Errorf("%s", event.Error)
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read ablation progress: %w", err)
	}
	return result, fmt.Errorf("sidecar ablation stream ended without a result")
}

func (u *HTTPAIAnalysisUpstream) Calibrate(ctx context.Context, payload AICalibrationRequest) (AICalibrationResult, error) {
	var result AICalibrationResult
	wirePayload := struct {
		AICalibrationRequest
		APIKey string `json:"api_key,omitempty"`
	}{AICalibrationRequest: payload, APIKey: payload.APIKey}
	body, err := json.Marshal(wirePayload)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAIAnalysisBaseURL(u.analyzeURL)+"/calibrate", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson, application/json")
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}
	// A ten-node replay can legitimately outlive the ordinary analysis timeout.
	// The detached handler context owns the four-hour calibration deadline.
	client := *u.client
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		detail := strings.TrimSpace(string(body))
		var failure struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(body, &failure) == nil && strings.TrimSpace(failure.Detail) != "" {
			detail = strings.TrimSpace(failure.Detail)
		}
		if detail == "" {
			detail = "sidecar calibration failed"
		}
		return result, fmt.Errorf("%s", detail)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "application/x-ndjson") {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, 16<<20))
		scanner.Buffer(make([]byte, 64*1024), 4<<20)
		for scanner.Scan() {
			var event struct {
				Type     string                `json:"type"`
				Progress AICalibrationProgress `json:"progress"`
				Result   AICalibrationResult   `json:"result"`
				Error    string                `json:"error"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				return result, fmt.Errorf("decode calibration progress: %w", err)
			}
			switch event.Type {
			case "progress":
				if payload.Progress != nil {
					payload.Progress(event.Progress)
				}
			case "result":
				return event.Result, nil
			case "error":
				if strings.TrimSpace(event.Error) == "" {
					event.Error = "sidecar calibration failed"
				}
				return result, fmt.Errorf("%s", event.Error)
			}
		}
		if err := scanner.Err(); err != nil {
			return result, fmt.Errorf("read calibration progress: %w", err)
		}
		return result, fmt.Errorf("sidecar calibration stream ended without a result")
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return result, fmt.Errorf("decode calibration response: %w", err)
	}
	return result, nil
}

type HTTPAIAnalysisUpstream struct {
	analyzeURL string
	client     *http.Client
	secret     string
}

func NewHTTPAIAnalysisUpstream(upstreamURL string, timeout time.Duration, sharedSecret ...string) *HTTPAIAnalysisUpstream {
	if timeout <= 0 {
		timeout = DefaultAIAnalysisTimeout
	}
	upstream := &HTTPAIAnalysisUpstream{
		analyzeURL: resolveAIAnalysisURL(upstreamURL),
		client:     &http.Client{Timeout: timeout + 30*time.Second},
	}
	if len(sharedSecret) > 0 {
		upstream.secret = strings.TrimSpace(sharedSecret[0])
	}
	return upstream
}

func (u *HTTPAIAnalysisUpstream) Config(_ context.Context) (AIAnalysisConfigResponse, error) {
	return DefaultAIAnalysisConfig(), nil
}

func (u *HTTPAIAnalysisUpstream) Models(ctx context.Context, payload AIModelDiscoveryRequest) (AIModelDiscoveryResponse, error) {
	var result AIModelDiscoveryResponse
	wirePayload := struct {
		AIModelDiscoveryRequest
		APIKey string `json:"api_key,omitempty"`
	}{AIModelDiscoveryRequest: payload, APIKey: payload.APIKey}
	body, err := json.Marshal(wirePayload)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveAIAnalysisBaseURL(u.analyzeURL)+"/models", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var failure struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&failure)
		if strings.TrimSpace(failure.Detail) == "" {
			failure.Detail = fmt.Sprintf("sidecar returned HTTP %d", resp.StatusCode)
		}
		return result, fmt.Errorf("%s", failure.Detail)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return result, fmt.Errorf("decode model discovery response: %w", err)
	}
	return result, nil
}

func (u *HTTPAIAnalysisUpstream) Start(ctx context.Context, payload AIAnalysisRequest) (*AIAnalysisStream, error) {
	wirePayload := struct {
		AIAnalysisRequest
		APIKey string `json:"api_key,omitempty"`
	}{AIAnalysisRequest: payload, APIKey: payload.APIKey}
	body, err := json.Marshal(wirePayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.analyzeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if u.secret != "" {
		req.Header.Set("X-Sidecar-Token", u.secret)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}

	return &AIAnalysisStream{
		Body:       resp.Body,
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
	}, nil
}

func DefaultAIAnalysisConfig() AIAnalysisConfigResponse {
	defaultEnabledFalse := false
	return AIAnalysisConfigResponse{
		Contract: AIAnalysisContract{
			Version:        AIAnalysisContractVersion,
			ConfigEndpoint: "/api/v1/analysis/ai/config",
			ModelsEndpoint: "/api/v1/analysis/ai/models",
			StreamEndpoint: "/api/v1/analysis/{symbol}/ai",
			UpstreamMode:   "sse",
			Events: []string{
				"heartbeat",
				"progress",
				"narration_delta",
				"report_start",
				"report_delta",
				"result",
				"thought",
				"report",
				"decision",
				"done",
				"error",
			},
		},
		Providers: []AIAnalysisProviderConfig{
			{
				ID:   "anthropic",
				Name: "Anthropic",
				Models: AIAnalysisModelSets{
					Deep:  []string{"claude-opus-4-6", "claude-sonnet-4-6"},
					Quick: []string{"claude-sonnet-4-6", "claude-haiku-4-5-20251001"},
				},
			},
			{
				ID:   "openai",
				Name: "OpenAI",
				Models: AIAnalysisModelSets{
					Deep:  []string{"gpt-5.4", "o4-mini"},
					Quick: []string{"gpt-5.4-mini", "gpt-4.1-nano"},
				},
			},
			{
				ID:                "openai_compatible",
				Name:              "OpenAI 兼容网关",
				CustomModels:      true,
				DefaultBackendURL: "https://ai-gateway.vercel.sh/v1",
				Models: AIAnalysisModelSets{
					Deep:  []string{"anthropic/claude-haiku-4.5", "google/gemini-3.1-flash-lite-preview"},
					Quick: []string{"anthropic/claude-haiku-4.5", "google/gemini-3.1-flash-lite-preview"},
				},
			},
			{
				ID:   "google",
				Name: "Google Gemini",
				Models: AIAnalysisModelSets{
					Deep:  []string{"gemini-3.0-flash-thinking", "gemini-3.0-pro"},
					Quick: []string{"gemini-3.0-flash", "gemini-3.1-flash"},
				},
			},
		},
		Analysts: []AIAnalysisAnalystConfig{
			{ID: "fundamental", Name: "基本面分析师", Description: "评估公司财务和业绩指标"},
			{ID: "sentiment", Name: "情绪分析师", Description: "聚合新闻和社交媒体情绪"},
			{ID: "news", Name: "新闻分析师", Description: "监控全球新闻和宏观指标"},
			{ID: "technical", Name: "技术分析师", Description: "基于技术指标检测交易信号"},
			{
				ID:             "canslim_second_layer",
				Name:           "CANSLIM 二层筛选官",
				Description:    "严格复核假成长、盈利质量、技术形态、估值和风险收益比",
				DefaultEnabled: &defaultEnabledFalse,
			},
		},
		DebateRounds: []int{1, 2, 3},
		RiskRounds:   []int{1, 2, 3},
	}
}

func resolveAIAnalysisURL(upstreamURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(upstreamURL), "/")
	if trimmed == "" {
		trimmed = DefaultAIAnalysisUpstreamURL
	}
	if strings.HasSuffix(trimmed, "/analyze") {
		return trimmed
	}
	return trimmed + "/analyze"
}

func resolveAIAnalysisBaseURL(upstreamURL string) string {
	return strings.TrimSuffix(resolveAIAnalysisURL(upstreamURL), "/analyze")
}
