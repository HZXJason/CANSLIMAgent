package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type AnalysisHandler struct {
	analysis  *services.AnalysisService
	streamURL string
	client    *http.Client
}

func NewAnalysisHandler(analysis *services.AnalysisService, streamURL string, proxyTimeout time.Duration) *AnalysisHandler {
	if proxyTimeout <= 0 {
		proxyTimeout = 30 * time.Second
	}
	return &AnalysisHandler{
		analysis:  analysis,
		streamURL: strings.TrimSpace(streamURL),
		client:    &http.Client{Timeout: proxyTimeout},
	}
}

func (h *AnalysisHandler) Stream(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码格式无效")
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if h.streamURL != "" {
		h.proxyStream(c, symbol)
		return
	}
	h.localStream(c, symbol)
}

func (h *AnalysisHandler) proxyStream(c *gin.Context, symbol string) {
	target := resolveAnalysisStreamURL(h.streamURL, symbol)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "AI 分析服务地址无效"})
		c.Writer.Flush()
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := h.client.Do(req)
	if err != nil {
		c.SSEvent("error", gin.H{"message": "AI 分析服务连接失败"})
		c.Writer.Flush()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.SSEvent("error", gin.H{"message": fmt.Sprintf("AI 分析服务返回异常：HTTP %d", resp.StatusCode)})
		c.Writer.Flush()
		return
	}
	_, _ = io.Copy(c.Writer, resp.Body)
	c.Writer.Flush()
}

func (h *AnalysisHandler) localStream(c *gin.Context, symbol string) {
	events := []struct {
		name string
		data gin.H
	}{
		{name: "thought", data: gin.H{"agent": "基本面分析师", "content": "正在读取季度 EPS、年度营收与毛利率趋势。"}},
		{name: "thought", data: gin.H{"agent": "相对强度分析师", "content": "正在读取 RS 相对强度线。"}},
		{name: "thought", data: gin.H{"agent": "CANSLIM 复核员", "content": "正在汇总 C/A/S/L/N 标签线索。"}},
	}
	for _, event := range events {
		if !writeSSE(c, event.name, event.data, 320*time.Millisecond) {
			return
		}
	}

	report := h.buildLocalReport(c.Request.Context(), symbol)
	for _, chunk := range chunkMarkdown(report) {
		if !writeSSE(c, "report", gin.H{"content": chunk}, 260*time.Millisecond) {
			return
		}
	}
	_ = writeSSE(c, "done", gin.H{"status": "complete"}, 0)
}

func (h *AnalysisHandler) buildLocalReport(ctx context.Context, symbol string) string {
	summary, summaryErr := h.analysis.StockSummary(ctx, symbol)
	rs, _ := h.analysis.RS(ctx, symbol, services.PeriodDaily)
	eps, epsErr := h.analysis.EPSQuarterly(ctx, symbol)
	revenue, _ := h.analysis.RevenueAnnual(ctx, symbol)
	margin, _ := h.analysis.GrossMarginQuarterly(ctx, symbol)

	name := symbol
	market := "未知市场"
	if summaryErr == nil {
		name = summary.Name
		market = summary.Market
	}

	rsText := "RS 强度数据不足"
	if len(rs.RSLine) > 0 {
		last := rs.RSLine[len(rs.RSLine)-1]
		rsText = fmt.Sprintf("最新 RS 强度线 %.1f，截止 %s，来源：%s", last.Value, last.Date, rs.Source)
	}

	epsText := "EPS 历史不足"
	if epsErr != nil {
		epsText = "EPS 数据源失败：" + epsErr.Error()
	} else if len(eps.Quarters) > 0 {
		q := eps.Quarters[0]
		epsText = fmt.Sprintf("%s EPS %.2f，YoY %.2f%%，来源：%s，截止 %s", q.Period, q.EPS, q.EPSYoYGrowth, eps.Source, eps.DataAsOf)
		if !q.DataSufficient {
			epsText += "（同比基准不足，仅供参考）"
		}
	}

	revenueText := "年度营收历史不足"
	if len(revenue.Years) > 0 {
		year := revenue.Years[0]
		revenueText = fmt.Sprintf("%d 年营收 %.2f 亿美元，YoY %.2f%%，来源：%s", year.Year, year.Revenue/100000000, year.RevenueYoYGrowth, revenue.Source)
	}

	marginText := "毛利率历史不足"
	if len(margin.Quarters) > 0 {
		q := margin.Quarters[0]
		marginText = fmt.Sprintf("%s 毛利率 %.2f%%，来源：%s", q.Period, q.GrossMargin, margin.Source)
	}

	nText := "N 标签尚未写回"
	if summaryErr == nil && summary.Metrics.N != nil && *summary.Metrics.N {
		nText = "N 标签已写回选股器"
	}

	return fmt.Sprintf(`# 综合研判：%s（%s）

## 相对强度

- %s。

## 基本面质量

- %s。
- %s。
- %s。

## CANSLIM 复核

- 市场池：%s。
- N 标签状态：%s。

## 操作提示

当前报告由 AlphaLens 本地规则流生成。K 线图已临时关闭，后续应优先确认 EPS、营收、毛利率和 RS 数据源稳定性，再决定是否写回 N 标签。
`, symbol, name, rsText, epsText, revenueText, marginText, market, nText)
}

func writeSSE(c *gin.Context, name string, data gin.H, delay time.Duration) bool {
	select {
	case <-c.Request.Context().Done():
		return false
	default:
	}
	c.SSEvent(name, data)
	c.Writer.Flush()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-c.Request.Context().Done():
			return false
		case <-timer.C:
		}
	}
	return true
}

func chunkMarkdown(value string) []string {
	parts := strings.SplitAfter(value, "\n\n")
	chunks := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			chunks = append(chunks, part)
		}
	}
	return chunks
}

func resolveAnalysisStreamURL(raw string, symbol string) string {
	if strings.Contains(raw, "{symbol}") {
		return strings.ReplaceAll(raw, "{symbol}", url.QueryEscape(symbol))
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	if query.Get("symbol") == "" {
		query.Set("symbol", symbol)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
