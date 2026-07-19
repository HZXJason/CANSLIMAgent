package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

var safeSymbolPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.^=_-]{0,31}$`)

type ChartsHandler struct {
	analysis *services.AnalysisService
	scanner  *services.ScannerService
}

func NewChartsHandler(analysis *services.AnalysisService, scanner *services.ScannerService) *ChartsHandler {
	return &ChartsHandler{analysis: analysis, scanner: scanner}
}

func (h *ChartsHandler) Summary(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	var sources map[string]services.MetricSourcePayload
	if h.scanner != nil {
		refreshedSources, _, err := h.scanner.HydrateStockMetrics(c.Request.Context(), symbol)
		if err == nil {
			sources = refreshedSources
		}
	}
	data, err := h.analysis.StockSummary(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusNotFound, "未找到股票基础信息，请先在选股器中刷新市场数据")
		return
	}
	data.MetricSources = sources
	OK(c, data)
}

func (h *ChartsHandler) Profile(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	data, err := h.analysis.CompanyProfile(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) KLine(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", services.PeriodDaily)
	data, err := h.analysis.KLine(c.Request.Context(), symbol, period)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) RS(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", services.PeriodDaily)
	data, err := h.analysis.RS(c.Request.Context(), symbol, period)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) EPSQuarterly(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	data, err := h.analysis.EPSQuarterly(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) RevenueAnnual(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	data, err := h.analysis.RevenueAnnual(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) GrossMarginQuarterly(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	data, err := h.analysis.GrossMarginQuarterly(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) SecondLayerData(c *gin.Context) {
	symbol, ok := validatedSymbol(c)
	if !ok {
		return
	}
	data, err := h.analysis.SecondLayerData(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func validatedSymbol(c *gin.Context) (string, bool) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码格式无效")
		return "", false
	}
	return symbol, true
}

func normalizeSymbolParam(value string) string {
	symbol := strings.ToUpper(strings.TrimSpace(value))
	if !safeSymbolPattern.MatchString(symbol) {
		return ""
	}
	if canonical := services.NormalizeAshareSymbol(symbol); canonical != "" {
		return canonical
	}
	// A symbol that explicitly looks like a mainland listing must never fall
	// through to the US provider chain when its code range and exchange disagree.
	if looksLikeAshareInput(symbol) {
		return ""
	}
	return symbol
}

func looksLikeAshareInput(symbol string) bool {
	if len(symbol) == 6 {
		for _, character := range symbol {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	if len(symbol) == 9 && symbol[6] == '.' {
		if suffix := symbol[7:]; suffix == "SH" || suffix == "SZ" || suffix == "BJ" || suffix == "SS" {
			for _, character := range symbol[:6] {
				if character < '0' || character > '9' {
					return false
				}
			}
			return true
		}
	}
	if len(symbol) == 8 && (strings.HasPrefix(symbol, "SH") || strings.HasPrefix(symbol, "SZ") || strings.HasPrefix(symbol, "BJ")) {
		for _, character := range symbol[2:] {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	if len(symbol) == 9 && (strings.HasPrefix(symbol, "SH") || strings.HasPrefix(symbol, "SZ") || strings.HasPrefix(symbol, "BJ")) && (symbol[2] == '.' || symbol[2] == '-') {
		for _, character := range symbol[3:] {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	return false
}
