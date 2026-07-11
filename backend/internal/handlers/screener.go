package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
)

type ScreenerHandler struct {
	scanner *services.ScannerService
}

func NewScreenerHandler(scanner *services.ScannerService) *ScreenerHandler {
	return &ScreenerHandler{scanner: scanner}
}

func (h *ScreenerHandler) Scan(c *gin.Context) {
	market := strings.ToUpper(c.DefaultQuery("market", models.MarketSP500))
	events, err := h.scanner.Scan(c.Request.Context(), market)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	c.Stream(func(w io.Writer) bool {
		event, ok := <-events
		if !ok {
			return false
		}
		c.SSEvent("message", event)
		return true
	})
}

func (h *ScreenerHandler) Results(c *gin.Context) {
	market := strings.ToUpper(c.DefaultQuery("market", models.MarketSP500))
	results, err := h.scanner.GetCachedResults(c.Request.Context(), market)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, results)
}

func (h *ScreenerHandler) Refresh(c *gin.Context) {
	var req struct {
		Market string `json:"market"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}
	req.Market = strings.ToUpper(strings.TrimSpace(req.Market))
	if req.Market == "" {
		req.Market = models.MarketSP500
	}

	job, err := h.scanner.StartRefresh(c.Request.Context(), req.Market)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"job_id": job.ID, "status": job.Status})
}

func (h *ScreenerHandler) RefreshJob(c *gin.Context) {
	jobID := c.Param("jobID")
	job, ok := h.scanner.GetRefreshJob(jobID)
	if !ok {
		Fail(c, http.StatusNotFound, "刷新任务不存在")
		return
	}
	OK(c, job)
}

func (h *ScreenerHandler) UpdateNLabel(c *gin.Context) {
	symbol := strings.ToUpper(strings.TrimSpace(c.Param("symbol")))
	var req struct {
		NLabel          bool   `json:"n_label"`
		AnalysisSummary string `json:"analysis_summary"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "请求体格式不正确")
		return
	}

	analysisAt, err := h.scanner.UpdateNLabel(c.Request.Context(), symbol, req.NLabel, req.AnalysisSummary)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	OK(c, gin.H{"symbol": symbol, "n_label": req.NLabel, "n_analysis_at": analysisAt})
}

func (h *ScreenerHandler) MarkAIAnalysisCompleted(c *gin.Context) {
	symbol := normalizeSymbolParam(c.Param("symbol"))
	analysisAt, err := h.scanner.MarkAIAnalysisCompleted(c.Request.Context(), symbol)
	if err != nil {
		Fail(c, http.StatusBadRequest, "AI 分析标记写回失败："+err.Error())
		return
	}
	OK(c, gin.H{"symbol": symbol, "ai_analyzed": true, "ai_analysis_at": analysisAt})
}
