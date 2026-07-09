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

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type AIAnalysisHandler struct {
	upstream services.AIAnalysisUpstream
	timeout  time.Duration
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
		upstream: upstream,
		timeout:  timeout,
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
