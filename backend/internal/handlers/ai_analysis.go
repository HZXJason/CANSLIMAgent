package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AIAnalysisHandler struct {
	upstream         services.AIAnalysisUpstream
	timeout          time.Duration
	loadTradeJournal func(context.Context, string) ([]pythonTradeRecord, error)
	modelDiscovery   services.AIModelDiscoverer
}

func NewAIAnalysisHandlerWithUpstreamAndJournal(upstream services.AIAnalysisUpstream, timeout time.Duration, database *gorm.DB) *AIAnalysisHandler {
	handler := NewAIAnalysisHandlerWithUpstream(upstream, timeout)
	if database != nil {
		handler.loadTradeJournal = func(ctx context.Context, symbol string) ([]pythonTradeRecord, error) {
			var entries []models.TradeJournalEntry
			err := database.WithContext(ctx).
				Where("symbol = ? AND quantity > 0 AND price > 0", symbol).
				Order("executed_at DESC").
				Limit(50).
				Find(&entries).Error
			if err != nil {
				return nil, err
			}
			records := make([]pythonTradeRecord, 0, len(entries))
			for index := len(entries) - 1; index >= 0; index-- {
				records = append(records, newPythonTradeRecord(entries[index]))
			}
			return records, nil
		}
	}
	return handler
}

func NewAIAnalysisHandler() *AIAnalysisHandler {
	return NewAIAnalysisHandlerWithUpstream(
		services.NewHTTPAIAnalysisUpstream(services.DefaultAIAnalysisUpstreamURL, services.DefaultAIAnalysisTimeout),
		services.DefaultAIAnalysisTimeout,
	)
}

func NewAIAnalysisHandlerWithSidecarURL(sidecarURL string) *AIAnalysisHandler {
	timeout := services.DefaultAIAnalysisTimeout
	return NewAIAnalysisHandlerWithUpstream(services.NewHTTPAIAnalysisUpstream(sidecarURL, timeout), timeout)
}

func NewAIAnalysisHandlerWithUpstream(upstream services.AIAnalysisUpstream, timeout time.Duration) *AIAnalysisHandler {
	if timeout <= 0 {
		timeout = services.DefaultAIAnalysisTimeout
	}
	if upstream == nil {
		upstream = services.NewHTTPAIAnalysisUpstream(services.DefaultAIAnalysisUpstreamURL, timeout)
	}
	return &AIAnalysisHandler{
		upstream:       upstream,
		timeout:        timeout,
		modelDiscovery: services.NewAIModelDiscoveryService(15 * time.Second),
	}
}

func (h *AIAnalysisHandler) Stream(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码不能为空")
		return
	}

	var payload services.AIAnalysisRequest
	if err := decodeOptionalJSON(c.Request.Body, &payload); err != nil {
		Fail(c, http.StatusBadRequest, "请求参数格式错误")
		return
	}
	payload.Ticker = symbol
	h.enrichTradeJournal(c.Request.Context(), &payload, symbol)

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	stream, err := h.upstream.Start(ctx, payload)
	if err != nil {
		Fail(c, http.StatusServiceUnavailable, "AI 分析上游服务暂不可用，请确认 AI_ANALYSIS_UPSTREAM_URL 或本地 sidecar 已启动")
		return
	}
	defer stream.Body.Close()

	if stream.StatusCode < http.StatusOK || stream.StatusCode >= http.StatusMultipleChoices {
		message := fmt.Sprintf("AI 分析上游服务返回异常：HTTP %d", stream.StatusCode)
		if detail := readErrorDetail(stream.Body); detail != "" {
			message = message + "，" + detail
		}
		Fail(c, http.StatusBadGateway, message)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if runID := strings.TrimSpace(stream.Header.Get("X-Analysis-Run-ID")); runID != "" {
		c.Header("X-Analysis-Run-ID", runID)
	}
	c.Status(http.StatusOK)

	reader := bufio.NewReader(stream.Body)
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

func (h *AIAnalysisHandler) enrichTradeJournal(ctx context.Context, payload *services.AIAnalysisRequest, symbol string) {
	if h.loadTradeJournal == nil || payload == nil {
		return
	}
	records, err := h.loadTradeJournal(ctx, symbol)
	if err != nil {
		return
	}
	if payload.AnalysisContext == nil {
		payload.AnalysisContext = make(map[string]any)
	}
	payload.AnalysisContext["trade_journal"] = map[string]any{
		"schema_version": "vibe.trade_record.v1",
		"records":        records,
	}
}

func (h *AIAnalysisHandler) Config(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	config, err := h.upstream.Config(ctx)
	if err != nil {
		Fail(c, http.StatusBadGateway, "AI 分析配置接口暂不可用")
		return
	}
	OK(c, config)
}

func (h *AIAnalysisHandler) DiscoverModels(c *gin.Context) {
	var payload services.AIModelDiscoveryRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		Fail(c, http.StatusBadRequest, "模型发现请求格式错误")
		return
	}
	payload.Provider = strings.ToLower(strings.TrimSpace(payload.Provider))
	if payload.Provider == "" {
		Fail(c, http.StatusBadRequest, "缺少 LLM 提供商")
		return
	}
	if payload.Provider == "openai_compatible" && strings.TrimSpace(payload.BackendURL) == "" {
		Fail(c, http.StatusBadRequest, "OpenAI 兼容网关需要填写 API 地址")
		return
	}
	if h.modelDiscovery == nil {
		Fail(c, http.StatusServiceUnavailable, "模型发现服务未配置")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result, err := h.modelDiscovery.Discover(ctx, payload)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "backend_url") {
			status = http.StatusBadRequest
		}
		Fail(c, status, "获取模型列表失败："+err.Error())
		return
	}
	OK(c, result)
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
