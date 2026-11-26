package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type EvidenceCardItem struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

type EvidenceCard struct {
	ID    string             `json:"id"`
	Title string             `json:"title"`
	Items []EvidenceCardItem `json:"items"`
}

type EvidenceSource struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	DataAsOf string `json:"data_as_of,omitempty"`
	Status   string `json:"status"`
}

type EvidenceCounterItem struct {
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
	SourceIDs []string `json:"source_ids"`
}

type EvidenceCounterThesis struct {
	BullAnchors    []EvidenceCounterItem `json:"bull_anchors"`
	BearFalsifiers []EvidenceCounterItem `json:"bear_falsifiers"`
	BlindSpots     []EvidenceCounterItem `json:"blind_spots"`
}

type EvidenceCardsPack struct {
	Symbol        string                 `json:"symbol"`
	DataAsOf      string                 `json:"data_as_of,omitempty"`
	LatestClose   *float64               `json:"latest_close,omitempty"`
	ATR14         *float64               `json:"atr_14,omitempty"`
	ATR14Percent  *float64               `json:"atr_14_percent,omitempty"`
	Dist52WHigh   *float64               `json:"distance_to_52w_high_percent,omitempty"`
	Cards         []EvidenceCard         `json:"cards"`
	Sources       []EvidenceSource       `json:"sources"`
	AIStatus      string                 `json:"ai_status"`
	AIError       string                 `json:"ai_error,omitempty"`
	CounterThesis *EvidenceCounterThesis `json:"counter_thesis"`
}

type EvidenceCardsHandler struct {
	analysis    *services.AnalysisService
	credentials *services.LLMCredentialStore
	sidecarURL  string
	secret      string
	client      *http.Client
}

func NewEvidenceCardsHandler(analysis *services.AnalysisService, credentials *services.LLMCredentialStore, sidecarBaseURL, sharedSecret string) *EvidenceCardsHandler {
	base := strings.TrimRight(strings.TrimSpace(sidecarBaseURL), "/")
	endpoint := ""
	if base != "" {
		endpoint = base + "/evidence-cards"
	}
	return &EvidenceCardsHandler{
		analysis:    analysis,
		credentials: credentials,
		sidecarURL:  endpoint,
		secret:      strings.TrimSpace(sharedSecret),
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *EvidenceCardsHandler) Compile(c *gin.Context) {
	symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "symbol is required")
		return
	}

	var req struct {
		IncludeAI   bool   `json:"include_ai"`
		LLMProvider string `json:"llm_provider"`
		Model       string `json:"model"`
		BackendURL  string `json:"backend_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "request body is invalid")
		return
	}

	pack, contextPayload := h.buildDeterministicPack(c.Request.Context(), symbol)
	if !req.IncludeAI {
		OK(c, pack)
		return
	}

	provider := strings.ToLower(strings.TrimSpace(req.LLMProvider))
	model := strings.TrimSpace(req.Model)
	if provider == "" || model == "" {
		pack.AIStatus = "unavailable"
		pack.AIError = "请先在交易系统设置中选择默认 AI 模型后再生成多空反证。"
		OK(c, pack)
		return
	}
	if h.sidecarURL == "" {
		pack.AIStatus = "unavailable"
		pack.AIError = "AI Sidecar 地址未配置。"
		OK(c, pack)
		return
	}

	apiKey := ""
	if h.credentials != nil {
		resolved, err := h.credentials.Resolve(c.Request.Context(), provider, strings.TrimSpace(req.BackendURL))
		if err != nil {
			pack.AIStatus = "unavailable"
			pack.AIError = "无法读取服务端模型凭据。"
			OK(c, pack)
			return
		}
		apiKey = resolved
	}

	wire := map[string]any{
		"ticker":       symbol,
		"include_ai":   true,
		"llm_provider": provider,
		"model":        model,
		"backend_url":  strings.TrimSpace(req.BackendURL),
		"api_key":      apiKey,
		"context":      contextPayload,
	}
	body, _ := json.Marshal(wire)
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.sidecarURL, bytes.NewReader(body))
	if err != nil {
		pack.AIStatus = "unavailable"
		pack.AIError = err.Error()
		OK(c, pack)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.secret != "" {
		httpReq.Header.Set("X-Sidecar-Token", h.secret)
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		pack.AIStatus = "unavailable"
		pack.AIError = err.Error()
		OK(c, pack)
		return
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		pack.AIStatus = "unavailable"
		pack.AIError = fmt.Sprintf("AI 反证提炼暂不可用（HTTP %d）", resp.StatusCode)
		OK(c, pack)
		return
	}
	var aiPack EvidenceCardsPack
	if err := json.Unmarshal(respBytes, &aiPack); err != nil {
		pack.AIStatus = "unavailable"
		pack.AIError = "AI 返回结构解析失败"
		OK(c, pack)
		return
	}
	aiPack.LatestClose = pack.LatestClose
	aiPack.ATR14 = pack.ATR14
	aiPack.ATR14Percent = pack.ATR14Percent
	aiPack.Dist52WHigh = pack.Dist52WHigh
	OK(c, aiPack)
}

func (h *EvidenceCardsHandler) buildDeterministicPack(ctx context.Context, symbol string) (EvidenceCardsPack, map[string]any) {
	pack := EvidenceCardsPack{
		Symbol:   symbol,
		Cards:    []EvidenceCard{},
		Sources:  []EvidenceSource{},
		AIStatus: "not_requested",
	}
	contextPayload := map[string]any{}

	var summary services.StockSummary
	hasSummary := false
	if h.analysis != nil {
		if loaded, err := h.analysis.StockSummary(ctx, symbol); err == nil {
			summary = loaded
			hasSummary = true
			pack.DataAsOf = loaded.DataAsOf
			contextPayload["stock_summary"] = map[string]any{
				"symbol":             loaded.Symbol,
				"name":               loaded.Name,
				"market":             loaded.Market,
				"data_as_of":         loaded.DataAsOf,
				"n_analysis_summary": loaded.NAnalysisSummary,
				"metrics": map[string]any{
					"c_eps_growth_pct":    loaded.Metrics.C.Value,
					"c_data_sufficient":   loaded.Metrics.C.Sufficient,
					"a_annual_eps_growth": loaded.Metrics.A.Value,
					"a_data_sufficient":   loaded.Metrics.A.Sufficient,
					"s_volume_ratio":      loaded.Metrics.S.Value,
					"s_data_sufficient":   loaded.Metrics.S.Sufficient,
					"l_rs_rating":         loaded.Metrics.L.Value,
					"l_data_sufficient":   loaded.Metrics.L.Sufficient,
				},
			}
			if loaded.DataAsOf != "" {
				pack.Sources = append(pack.Sources, EvidenceSource{ID: "summary-metrics", Label: "CANSLIM Screener Metrics", DataAsOf: loaded.DataAsOf, Status: "available"})
			}
		}
	}

	growthItems := []EvidenceCardItem{}
	if hasSummary && summary.Metrics.C.Sufficient {
		if cVal, ok := asFloat64(summary.Metrics.C.Value); ok {
			growthItems = append(growthItems, EvidenceCardItem{Label: "当季 EPS 同比 (C)", Value: fmt.Sprintf("%+.2f%%", cVal), Status: "ok"})
		}
	}
	if len(growthItems) == 0 {
		growthItems = append(growthItems, EvidenceCardItem{Label: "当季 EPS 同比 (C)", Value: "未知（数据不足）", Status: "unknown"})
	}

	addedA := false
	if hasSummary && summary.Metrics.A.Sufficient {
		if list, ok := summary.Metrics.A.Value.([]float64); ok && len(list) > 0 {
			parts := make([]string, 0, 3)
			for i := 0; i < len(list) && i < 3; i++ {
				parts = append(parts, fmt.Sprintf("%+.1f%%", list[i]))
			}
			growthItems = append(growthItems, EvidenceCardItem{Label: "年度 EPS 增速 (A)", Value: strings.Join(parts, " / "), Status: "ok"})
			addedA = true
		}
	}
	if !addedA {
		growthItems = append(growthItems, EvidenceCardItem{Label: "年度 EPS 增速 (A)", Value: "未知（数据不足）", Status: "unknown"})
	}

	if h.analysis != nil {
		if eps, err := h.analysis.EPSQuarterly(ctx, symbol); err == nil {
			contextPayload["eps"] = eps
			if eps.Source != "" {
				pack.Sources = append(pack.Sources, EvidenceSource{ID: "eps-series", Label: "EPS Quarterly (" + eps.Source + ")", DataAsOf: eps.DataAsOf, Status: "available"})
			}
		}
		if rev, err := h.analysis.RevenueAnnual(ctx, symbol); err == nil {
			contextPayload["revenue"] = rev
			if rev.Source != "" {
				pack.Sources = append(pack.Sources, EvidenceSource{ID: "revenue-series", Label: "Revenue Annual (" + rev.Source + ")", DataAsOf: rev.DataAsOf, Status: "available"})
			}
		}
		if gm, err := h.analysis.GrossMarginQuarterly(ctx, symbol); err == nil && len(gm.Quarters) > 0 {
			contextPayload["gross_margin"] = gm
			latestGM := gm.Quarters[0]
			growthItems = append(growthItems, EvidenceCardItem{
				Label: fmt.Sprintf("最新毛利率 (%s)", latestGM.Period), Value: fmt.Sprintf("%.2f%%", latestGM.GrossMargin), Status: "ok",
			})
			pack.Sources = append(pack.Sources, EvidenceSource{ID: "gross-margin", Label: "Gross Margin (" + gm.Source + ")", DataAsOf: gm.DataAsOf, Status: "available"})
		} else {
			growthItems = append(growthItems, EvidenceCardItem{Label: "最新毛利率", Value: "未知", Status: "unknown"})
		}
	}

	priceItems := []EvidenceCardItem{}
	addedL := false
	if hasSummary && summary.Metrics.L.Sufficient {
		if lVal, ok := asFloat64(summary.Metrics.L.Value); ok {
			priceItems = append(priceItems, EvidenceCardItem{Label: "RS 相对强度 (L)", Value: fmt.Sprintf("%.0f", lVal), Status: "ok"})
			addedL = true
		}
	}
	if !addedL {
		priceItems = append(priceItems, EvidenceCardItem{Label: "RS 相对强度 (L)", Value: "未知", Status: "unknown"})
	}
	addedS := false
	if hasSummary && summary.Metrics.S.Sufficient {
		if sVal, ok := asFloat64(summary.Metrics.S.Value); ok {
			priceItems = append(priceItems, EvidenceCardItem{Label: "量比 (S)", Value: fmt.Sprintf("%.2fx", sVal), Status: "ok"})
			addedS = true
		}
	}
	if !addedS {
		priceItems = append(priceItems, EvidenceCardItem{Label: "量比 (S)", Value: "未知", Status: "unknown"})
	}

	if h.analysis != nil {
		if kline, err := h.analysis.KLine(ctx, symbol, services.PeriodDaily); err == nil && len(kline.Candles) > 0 {
			last := kline.Candles[len(kline.Candles)-1]
			latestClose := math.Round(last.Close*100) / 100
			pack.LatestClose = &latestClose
			if pack.DataAsOf == "" {
				pack.DataAsOf = kline.DataAsOf
			}
			priceItems = append(priceItems, EvidenceCardItem{Label: "最新收盘价", Value: fmt.Sprintf("%.2f (%s)", latestClose, last.Date), Status: "ok"})
			klineStats := map[string]any{"latest_close": latestClose, "source": kline.Source, "data_as_of": kline.DataAsOf}
			if atr := services.ComputeATR14(kline.Candles); atr > 0 && last.Close > 0 {
				atrVal := math.Round(atr*100) / 100
				atrPct := math.Round((atr/last.Close*100)*100) / 100
				pack.ATR14 = &atrVal
				pack.ATR14Percent = &atrPct
				klineStats["atr14"] = atrVal
				klineStats["atr14_percent"] = atrPct
				priceItems = append(priceItems, EvidenceCardItem{Label: "ATR(14) 日均波幅", Value: fmt.Sprintf("%.2f (%.2f%%)", atrVal, atrPct), Status: "ok"})
			}
			high52 := 0.0
			startIdx := len(kline.Candles) - 252
			if startIdx < 0 {
				startIdx = 0
			}
			for _, bar := range kline.Candles[startIdx:] {
				if bar.High > high52 {
					high52 = bar.High
				}
			}
			if high52 > 0 {
				dist := math.Round(((last.Close-high52)/high52*100)*100) / 100
				pack.Dist52WHigh = &dist
				klineStats["distance_to_52w_high_percent"] = dist
				priceItems = append(priceItems, EvidenceCardItem{Label: "距 52 周高点", Value: fmt.Sprintf("%+.2f%%", dist), Status: "ok"})
			}
			contextPayload["kline_stats"] = klineStats
			pack.Sources = append(pack.Sources, EvidenceSource{ID: "kline-daily", Label: "Daily Price & ATR (" + kline.Source + ")", DataAsOf: kline.DataAsOf, Status: "available"})
		}
	}

	catalystItems := []EvidenceCardItem{}
	if hasSummary && strings.TrimSpace(summary.NAnalysisSummary) != "" {
		catalystItems = append(catalystItems, EvidenceCardItem{Label: "新催化记录 (N)", Value: summary.NAnalysisSummary, Status: "ok"})
	} else {
		catalystItems = append(catalystItems, EvidenceCardItem{Label: "新催化记录 (N)", Value: "尚未记录人工或 AI 催化摘要", Status: "unknown"})
	}

	auditItems := []EvidenceCardItem{
		{Label: "可用来源数量", Value: fmt.Sprintf("%d 个已核验来源", len(pack.Sources)), Status: "ok"},
	}
	if pack.DataAsOf != "" {
		auditItems = append(auditItems, EvidenceCardItem{Label: "最新数据日期", Value: pack.DataAsOf, Status: "ok"})
	} else {
		auditItems = append(auditItems, EvidenceCardItem{Label: "最新数据日期", Value: "未知（当前无本地行情或财报快照）", Status: "warning"})
	}

	pack.Cards = []EvidenceCard{
		{ID: "growth_inflection", Title: "C / A 增长与毛利拐点", Items: growthItems},
		{ID: "price_volume_rs", Title: "S / L 量价结构与波动率", Items: priceItems},
		{ID: "catalysts_and_events", Title: "N 催化与事件风险窗口", Items: catalystItems},
		{ID: "audit_and_unknowns", Title: "数据审计与未知边界", Items: auditItems},
	}
	return pack, contextPayload
}

func asFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
