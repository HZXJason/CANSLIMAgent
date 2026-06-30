package handlers

import (
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type ChartsHandler struct {
	analysis *services.AnalysisService
}

func NewChartsHandler(analysis *services.AnalysisService) *ChartsHandler {
	return &ChartsHandler{analysis: analysis}
}

func (h *ChartsHandler) Summary(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	data, err := h.analysis.StockSummary(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusNotFound, "未找到股票基础信息，请先在选股器中刷新市场数据")
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) KLine(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	period := c.DefaultQuery("period", services.PeriodDaily)
	data, err := h.analysis.KLine(c.Request.Context(), symbol, period)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) RS(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	period := c.DefaultQuery("period", services.PeriodDaily)
	data, err := h.analysis.RS(c.Request.Context(), symbol, period)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) EPSQuarterly(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	data, err := h.analysis.EPSQuarterly(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) RevenueAnnual(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	data, err := h.analysis.RevenueAnnual(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func (h *ChartsHandler) GrossMarginQuarterly(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	data, err := h.analysis.GrossMarginQuarterly(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, data)
}

func normalizeSymbolParam(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
