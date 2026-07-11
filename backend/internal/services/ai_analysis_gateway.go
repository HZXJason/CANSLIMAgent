package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultAIAnalysisUpstreamURL = "http://127.0.0.1:8765"
	DefaultAIAnalysisTimeout     = 20 * time.Minute

	AIAnalysisContractVersion = "ai-analysis.v1"
)

type AIAnalysisRequest struct {
	Ticker          string            `json:"ticker"`
	LLMProvider     string            `json:"llm_provider"`
	DeepThinkLLM    string            `json:"deep_think_llm"`
	QuickThinkLLM   string            `json:"quick_think_llm"`
	BackendURL      string            `json:"backend_url"`
	MaxDebateRounds int               `json:"max_debate_rounds,omitempty"`
	MaxRiskRounds   int               `json:"max_risk_rounds,omitempty"`
	OnlineTools     bool              `json:"online_tools"`
	Analysts        []string          `json:"analysts"`
	APIKeys         map[string]string `json:"api_keys,omitempty"`
	AnalysisContext map[string]any    `json:"analysis_context,omitempty"`
	RunID           string            `json:"run_id,omitempty"`
	Resume          bool              `json:"resume,omitempty"`
	LastEventID     int               `json:"last_event_id,omitempty"`
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
	Start(ctx context.Context, request AIAnalysisRequest) (*AIAnalysisStream, error)
}

type HTTPAIAnalysisUpstream struct {
	analyzeURL string
	client     *http.Client
}

func NewHTTPAIAnalysisUpstream(upstreamURL string, timeout time.Duration) *HTTPAIAnalysisUpstream {
	if timeout <= 0 {
		timeout = DefaultAIAnalysisTimeout
	}
	return &HTTPAIAnalysisUpstream{
		analyzeURL: resolveAIAnalysisURL(upstreamURL),
		client:     &http.Client{Timeout: timeout + 30*time.Second},
	}
}

func (u *HTTPAIAnalysisUpstream) Config(_ context.Context) (AIAnalysisConfigResponse, error) {
	return DefaultAIAnalysisConfig(), nil
}

func (u *HTTPAIAnalysisUpstream) Start(ctx context.Context, payload AIAnalysisRequest) (*AIAnalysisStream, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.analyzeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

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
