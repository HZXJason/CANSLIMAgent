package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultSidecarURL = "http://127.0.0.1:8765"
const defaultAIAnalysisTimeout = 20 * time.Minute

type AIAnalysisHandler struct {
	sidecarURL string
	client     *http.Client
	timeout    time.Duration
}

type AIAnalysisRequest struct {
	Ticker          string            `json:"ticker"`
	LLMProvider     string            `json:"llm_provider"`
	DeepThinkLLM    string            `json:"deep_think_llm"`
	QuickThinkLLM   string            `json:"quick_think_llm"`
	BackendURL      string            `json:"backend_url"`
	MaxDebateRounds int               `json:"max_debate_rounds"`
	MaxRiskRounds   int               `json:"max_risk_rounds"`
	OnlineTools     bool              `json:"online_tools"`
	Analysts        []string          `json:"analysts"`
	APIKeys         map[string]string `json:"api_keys,omitempty"`
	AnalysisContext map[string]any    `json:"analysis_context,omitempty"`
}

func NewAIAnalysisHandler() *AIAnalysisHandler {
	return NewAIAnalysisHandlerWithSidecarURL(defaultSidecarURL)
}

func NewAIAnalysisHandlerWithSidecarURL(sidecarURL string) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		sidecarURL: strings.TrimRight(strings.TrimSpace(sidecarURL), "/"),
		client:     &http.Client{},
		timeout:    aiAnalysisTimeoutFromEnv(),
	}
}

func (h *AIAnalysisHandler) Stream(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码不能为空")
		return
	}

	var payload AIAnalysisRequest
	if err := decodeOptionalJSON(c.Request.Body, &payload); err != nil {
		Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	payload.Ticker = symbol

	body, err := json.Marshal(payload)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "AI 分析请求构造失败")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.analyzeURL(), bytes.NewReader(body))
	if err != nil {
		Fail(c, http.StatusInternalServerError, "AI 分析服务地址无效")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := h.client.Do(req)
	if err != nil {
		Fail(c, http.StatusServiceUnavailable, "AI 分析 sidecar 暂不可用，请先启动 sidecar：cd sidecar; .\\start.ps1")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := fmt.Sprintf("AI 分析 sidecar 返回异常：HTTP %d", resp.StatusCode)
		if detail := readErrorDetail(resp.Body); detail != "" {
			message = message + "，" + detail
		}
		Fail(c, http.StatusBadGateway, message)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, writeErr := c.Writer.Write(line); writeErr != nil {
				return
			}
			c.Writer.Flush()
		}
		if errors.Is(readErr, io.EOF) {
			return
		}
		if readErr != nil {
			return
		}
	}
}

func (h *AIAnalysisHandler) Config(c *gin.Context) {
	OK(c, gin.H{
		"providers": []gin.H{
			{
				"id":   "anthropic",
				"name": "Anthropic",
				"models": gin.H{
					"deep":  []string{"claude-opus-4-6", "claude-sonnet-4-6"},
					"quick": []string{"claude-sonnet-4-6", "claude-haiku-4-5-20251001"},
				},
			},
			{
				"id":   "openai",
				"name": "OpenAI",
				"models": gin.H{
					"deep":  []string{"gpt-5.4", "o4-mini"},
					"quick": []string{"gpt-5.4-mini", "gpt-4.1-nano"},
				},
			},
			{
				"id":                  "openai_compatible",
				"name":                "OpenAI 兼容网关",
				"custom_models":       true,
				"default_backend_url": "https://ai-gateway.vercel.sh/v1",
				"models": gin.H{
					"deep":  []string{"anthropic/claude-haiku-4.5", "google/gemini-3.1-flash-lite-preview"},
					"quick": []string{"anthropic/claude-haiku-4.5", "google/gemini-3.1-flash-lite-preview"},
				},
			},
			{
				"id":   "google",
				"name": "Google Gemini",
				"models": gin.H{
					"deep":  []string{"gemini-3.0-flash-thinking", "gemini-3.0-pro"},
					"quick": []string{"gemini-3.0-flash", "gemini-3.1-flash"},
				},
			},
		},
		"analysts": []gin.H{
			{"id": "fundamental", "name": "基本面分析师", "description": "评估公司财务和业绩指标"},
			{"id": "sentiment", "name": "情绪分析师", "description": "聚合新闻和社交媒体情绪"},
			{"id": "news", "name": "新闻分析师", "description": "监控全球新闻和宏观指标"},
			{"id": "technical", "name": "技术分析师", "description": "基于技术指标检测交易信号"},
			{"id": "canslim_second_layer", "name": "CANSLIM 二层筛选官", "description": "严格复核假成长、盈利质量、技术形态、估值和风险收益比", "default_enabled": false},
		},
		"debate_rounds": []int{1, 2, 3},
		"risk_rounds":   []int{1, 2, 3},
	})
}

func (h *AIAnalysisHandler) analyzeURL() string {
	if strings.HasSuffix(h.sidecarURL, "/analyze") {
		return h.sidecarURL
	}
	return h.sidecarURL + "/analyze"
}

func aiAnalysisTimeoutFromEnv() time.Duration {
	value := strings.TrimSpace(os.Getenv("AI_ANALYSIS_TIMEOUT_SECONDS"))
	if value == "" {
		return defaultAIAnalysisTimeout
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return defaultAIAnalysisTimeout
	}
	return time.Duration(seconds) * time.Second
}

func decodeOptionalJSON(body io.Reader, target interface{}) error {
	if body == nil {
		return nil
	}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func readErrorDetail(body io.Reader) string {
	payload, err := io.ReadAll(io.LimitReader(body, 2048))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}
