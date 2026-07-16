package handlers

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
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
	credentials      *services.LLMCredentialStore
	database         *gorm.DB
	calibrationMu    sync.Mutex
	calibrationStops map[string]context.CancelFunc
}

func (h *AIAnalysisHandler) WithCredentialStore(store *services.LLMCredentialStore) *AIAnalysisHandler {
	h.credentials = store
	return h
}

func NewAIAnalysisHandlerWithUpstreamAndJournal(upstream services.AIAnalysisUpstream, timeout time.Duration, database *gorm.DB) *AIAnalysisHandler {
	handler := NewAIAnalysisHandlerWithUpstream(upstream, timeout)
	handler.database = database
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

const aiCalibrationStrategyVersion = "vibe-decision-calibration.v5"
const aiAblationStrategyVersion = "tradingagents-architecture-ablation.v2"

type aiCalibrationInput struct {
	services.AIAnalysisRequest
	Refresh     bool `json:"refresh,omitempty"`
	HorizonDays int  `json:"horizon_days,omitempty"`
}

type aiAblationInput struct {
	services.AIAnalysisRequest
	Refresh              bool    `json:"refresh,omitempty"`
	StartDate            string  `json:"start_date,omitempty"`
	EndDate              string  `json:"end_date,omitempty"`
	HorizonDays          int     `json:"horizon_days,omitempty"`
	MaximumNodes         int     `json:"maximum_nodes,omitempty"`
	Repeats              int     `json:"repeats,omitempty"`
	Concurrency          int     `json:"concurrency,omitempty"`
	TransactionCostBP    float64 `json:"transaction_cost_bps,omitempty"`
	MaxAttempts          int     `json:"max_attempts,omitempty"`
	AdaptiveStages       *bool   `json:"adaptive_stages,omitempty"`
	StageOneNodes        int     `json:"stage_one_nodes,omitempty"`
	ReproducibilityNodes int     `json:"reproducibility_nodes,omitempty"`
}

type aiCalibrationResponse struct {
	ID              string         `json:"id"`
	Symbol          string         `json:"symbol"`
	ConfigHash      string         `json:"config_hash"`
	Provider        string         `json:"provider"`
	DeepThinkLLM    string         `json:"deep_think_llm"`
	QuickThinkLLM   string         `json:"quick_think_llm"`
	StrategyVersion string         `json:"strategy_version"`
	Status          string         `json:"status"`
	TotalNodes      int            `json:"total_nodes"`
	CompletedNodes  int            `json:"completed_nodes"`
	CachedNodes     int            `json:"cached_nodes"`
	FailedNodes     int            `json:"failed_nodes"`
	SampleCount     int            `json:"sample_count"`
	CorrectCount    int            `json:"correct_count"`
	Accuracy        float64        `json:"accuracy"`
	Confidence      string         `json:"confidence"`
	HorizonDays     int            `json:"horizon_days"`
	StartedAt       time.Time      `json:"started_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Error           *string        `json:"error,omitempty"`
	Payload         map[string]any `json:"payload,omitempty"`
}

func calibrationResponse(row models.AICalibration) aiCalibrationResponse {
	return aiCalibrationResponse{
		ID: row.ID.String(), Symbol: row.Symbol, ConfigHash: row.ConfigHash,
		Provider: row.Provider, DeepThinkLLM: row.DeepThinkLLM, QuickThinkLLM: row.QuickThinkLLM,
		StrategyVersion: row.StrategyVersion, Status: row.Status, SampleCount: row.SampleCount,
		TotalNodes: row.TotalNodes, CompletedNodes: row.CompletedNodes,
		CachedNodes: row.CachedNodes, FailedNodes: row.FailedNodes,
		CorrectCount: row.CorrectCount, Accuracy: row.Accuracy, Confidence: row.Confidence,
		HorizonDays: row.HorizonDays, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt,
		Error: row.Error, Payload: row.Payload,
	}
}

func normalizeCalibrationHorizon(value int) (int, bool) {
	if value == 0 {
		return 20, true
	}
	if value == 20 || value == 40 {
		return value, true
	}
	return 0, false
}

func calibrationHash(request services.AIAnalysisRequest, horizonDays int) string {
	analysts := append([]string(nil), request.Analysts...)
	sort.Strings(analysts)
	fingerprintHorizon := horizonDays
	if horizonDays == 20 {
		// Preserve all existing v5 20-day cache keys created before dual-horizon support.
		fingerprintHorizon = 0
	}
	fingerprint := struct {
		Version         string   `json:"version"`
		Provider        string   `json:"provider"`
		DeepThinkLLM    string   `json:"deep_think_llm"`
		QuickThinkLLM   string   `json:"quick_think_llm"`
		BackendURL      string   `json:"backend_url"`
		MaxDebateRounds int      `json:"max_debate_rounds"`
		MaxRiskRounds   int      `json:"max_risk_rounds"`
		OnlineTools     bool     `json:"online_tools"`
		Analysts        []string `json:"analysts"`
		HorizonDays     int      `json:"horizon_days,omitempty"`
	}{aiCalibrationStrategyVersion, request.LLMProvider, request.DeepThinkLLM, request.QuickThinkLLM,
		strings.TrimRight(request.BackendURL, "/"), request.MaxDebateRounds, request.MaxRiskRounds,
		request.OnlineTools, analysts, fingerprintHorizon}
	encoded, _ := json.Marshal(fingerprint)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum[:16])
}

func normalizeAblationInput(input *aiAblationInput) error {
	if input.StartDate == "" {
		input.StartDate = "2022-01-03"
	}
	if input.EndDate == "" {
		input.EndDate = "2025-06-30"
	}
	start, startErr := time.Parse("2006-01-02", input.StartDate)
	end, endErr := time.Parse("2006-01-02", input.EndDate)
	if startErr != nil || endErr != nil || end.Before(start) {
		return errors.New("消融实验日期范围无效")
	}
	if input.HorizonDays == 0 {
		input.HorizonDays = 20
	}
	if input.MaximumNodes == 0 {
		input.MaximumNodes = 45
	}
	if input.Repeats == 0 {
		input.Repeats = 1
	}
	if input.Concurrency == 0 {
		input.Concurrency = 7
	}
	if input.TransactionCostBP == 0 {
		input.TransactionCostBP = 10
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 3
	}
	if input.AdaptiveStages == nil {
		enabled := true
		input.AdaptiveStages = &enabled
	}
	if input.StageOneNodes == 0 {
		input.StageOneNodes = 30
	}
	if input.ReproducibilityNodes == 0 {
		input.ReproducibilityNodes = 5
	}
	if input.HorizonDays < 5 || input.HorizonDays > 60 || input.MaximumNodes < 5 || input.MaximumNodes > 60 || input.Repeats < 1 || input.Repeats > 5 || input.Concurrency < 1 || input.Concurrency > 10 || input.TransactionCostBP < 0 || input.TransactionCostBP > 500 || input.MaxAttempts < 1 || input.MaxAttempts > 5 {
		return errors.New("消融实验参数超出允许范围")
	}
	if *input.AdaptiveStages && (input.Repeats != 1 || input.StageOneNodes < 5 || input.StageOneNodes >= input.MaximumNodes || input.ReproducibilityNodes < 1 || input.ReproducibilityNodes > input.StageOneNodes) {
		return errors.New("分阶段消融实验参数无效")
	}
	if len(input.Analysts) == 0 {
		input.Analysts = []string{"technical"}
	}
	if len(input.Analysts) != 1 || (strings.ToLower(strings.TrimSpace(input.Analysts[0])) != "technical" && strings.ToLower(strings.TrimSpace(input.Analysts[0])) != "market") {
		return errors.New("确认性消融实验只允许可按历史时点重建的技术分析师")
	}
	input.Analysts = []string{"technical"}
	if input.MaxDebateRounds == 0 {
		input.MaxDebateRounds = 1
	}
	if input.MaxRiskRounds == 0 {
		input.MaxRiskRounds = 1
	}
	return nil
}

func ablationHash(input aiAblationInput) string {
	fingerprint := struct {
		Version              string  `json:"version"`
		Provider             string  `json:"provider"`
		DeepThinkLLM         string  `json:"deep_think_llm"`
		QuickThinkLLM        string  `json:"quick_think_llm"`
		BackendURL           string  `json:"backend_url"`
		MaxDebateRounds      int     `json:"max_debate_rounds"`
		MaxRiskRounds        int     `json:"max_risk_rounds"`
		OnlineTools          bool    `json:"online_tools"`
		StartDate            string  `json:"start_date"`
		EndDate              string  `json:"end_date"`
		HorizonDays          int     `json:"horizon_days"`
		MaximumNodes         int     `json:"maximum_nodes"`
		Repeats              int     `json:"repeats"`
		Concurrency          int     `json:"concurrency"`
		TransactionCostBP    float64 `json:"transaction_cost_bps"`
		MaxAttempts          int     `json:"max_attempts"`
		AdaptiveStages       bool    `json:"adaptive_stages"`
		StageOneNodes        int     `json:"stage_one_nodes"`
		ReproducibilityNodes int     `json:"reproducibility_nodes"`
	}{
		aiAblationStrategyVersion, input.LLMProvider, input.DeepThinkLLM, input.QuickThinkLLM,
		strings.TrimRight(input.BackendURL, "/"), input.MaxDebateRounds, input.MaxRiskRounds,
		input.OnlineTools, input.StartDate, input.EndDate, input.HorizonDays, input.MaximumNodes,
		input.Repeats, input.Concurrency, input.TransactionCostBP, input.MaxAttempts,
		*input.AdaptiveStages, input.StageOneNodes, input.ReproducibilityNodes,
	}
	encoded, _ := json.Marshal(fingerprint)
	sum := sha256.Sum256(encoded)
	return "ablation-" + fmt.Sprintf("%x", sum[:16])
}

func (h *AIAnalysisHandler) CalibrationStatus(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码不能为空")
		return
	}
	var input aiCalibrationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "历史校准配置格式错误")
		return
	}
	horizonDays, valid := normalizeCalibrationHorizon(input.HorizonDays)
	if !valid {
		Fail(c, http.StatusBadRequest, "历史校准周期仅支持 20 或 40 个交易日")
		return
	}
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, calibrationHash(input.AIAnalysisRequest, horizonDays)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "读取历史校准结果失败")
		return
	}
	if row.Status == models.AICalibrationStatusRunning && time.Since(row.StartedAt) > 4*time.Hour {
		message := "历史校准任务已超过四小时，服务可能曾重启；请重新运行"
		now := time.Now().UTC()
		_ = h.database.Model(&row).Updates(map[string]any{
			"status": models.AICalibrationStatusFailed, "error": message, "completed_at": &now,
		}).Error
		row.Status, row.Error, row.CompletedAt = models.AICalibrationStatusFailed, &message, &now
	}
	OK(c, calibrationResponse(row))
}

func (h *AIAnalysisHandler) StartCalibration(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	if symbol == "" {
		Fail(c, http.StatusBadRequest, "股票代码不能为空")
		return
	}
	var input aiCalibrationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "历史校准配置格式错误")
		return
	}
	input.Ticker = symbol
	horizonDays, valid := normalizeCalibrationHorizon(input.HorizonDays)
	if !valid {
		Fail(c, http.StatusBadRequest, "历史校准周期仅支持 20 或 40 个交易日")
		return
	}
	if err := h.attachStoredAPIKey(c.Request.Context(), input.LLMProvider, input.BackendURL, &input.APIKey); err != nil {
		Fail(c, http.StatusInternalServerError, "后端保存的 LLM 密钥无法读取")
		return
	}
	hash := calibrationHash(input.AIAnalysisRequest, horizonDays)
	now := time.Now().UTC()

	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, hash).First(&row).Error
	if err == nil && row.Status == models.AICalibrationStatusRunning && time.Since(row.StartedAt) <= 4*time.Hour {
		c.JSON(http.StatusAccepted, APIResponse{Success: true, Data: calibrationResponse(row), Error: nil})
		return
	}
	if err == nil && row.Status == models.AICalibrationStatusComplete && !input.Refresh {
		OK(c, calibrationResponse(row))
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusInternalServerError, "创建历史校准任务失败")
		return
	}
	forceRefresh := err == nil && row.Status == models.AICalibrationStatusComplete && input.Refresh
	row.Symbol = symbol
	row.ConfigHash = hash
	row.Provider = input.LLMProvider
	row.DeepThinkLLM = input.DeepThinkLLM
	row.QuickThinkLLM = input.QuickThinkLLM
	row.StrategyVersion = aiCalibrationStrategyVersion
	row.Status = models.AICalibrationStatusRunning
	row.TotalNodes = 10
	row.CompletedNodes = 0
	row.CachedNodes = 0
	row.FailedNodes = 0
	row.SampleCount = 0
	row.CorrectCount = 0
	row.Accuracy = 0
	row.Confidence = "INSUFFICIENT"
	row.HorizonDays = horizonDays
	row.StartedAt = now
	row.CompletedAt = nil
	row.Error = nil
	row.Payload = map[string]any{
		"schema_version":     aiCalibrationStrategyVersion,
		"requested_analysts": append([]string(nil), input.Analysts...),
	}
	if err := h.database.Save(&row).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "保存历史校准任务失败")
		return
	}

	request := services.AICalibrationRequest{
		AIAnalysisRequest: input.AIAnalysisRequest, JobID: row.ID.String(), SampleCount: 10, HorizonDays: horizonDays, ForceRefresh: forceRefresh,
	}
	request.Ticker = symbol
	request.Progress = func(progress services.AICalibrationProgress) {
		_ = h.database.Model(&models.AICalibration{}).Where("id = ? AND status = ?", row.ID, models.AICalibrationStatusRunning).Updates(map[string]any{
			"total_nodes": progress.TotalNodes, "completed_nodes": progress.CompletedNodes,
			"cached_nodes": progress.CachedNodes, "failed_nodes": progress.FailedNodes,
		}).Error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	h.calibrationStops[row.ID.String()] = cancel
	go h.runCalibration(ctx, row.ID.String(), request)
	c.JSON(http.StatusAccepted, APIResponse{Success: true, Data: calibrationResponse(row), Error: nil})
}

func (h *AIAnalysisHandler) StopCalibration(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	var input aiCalibrationInput
	if symbol == "" || c.ShouldBindJSON(&input) != nil {
		Fail(c, http.StatusBadRequest, "历史校准配置格式错误")
		return
	}
	horizonDays, valid := normalizeCalibrationHorizon(input.HorizonDays)
	if !valid {
		Fail(c, http.StatusBadRequest, "历史校准周期仅支持 20 或 40 个交易日")
		return
	}

	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, calibrationHash(input.AIAnalysisRequest, horizonDays)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "读取历史校准任务失败")
		return
	}
	if row.Status != models.AICalibrationStatusRunning {
		OK(c, calibrationResponse(row))
		return
	}
	if canceler, ok := h.upstream.(services.AICalibrationCanceler); ok {
		stopCtx, stopCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		err = canceler.CancelCalibration(stopCtx, row.ID.String())
		stopCancel()
		if err != nil {
			Fail(c, http.StatusBadGateway, "停止 sidecar 历史校准失败："+err.Error())
			return
		}
	}
	if cancel := h.calibrationStops[row.ID.String()]; cancel != nil {
		cancel()
	}
	now := time.Now().UTC()
	message := "用户已停止任务；正在执行的模型请求可能需要短暂时间结束"
	if err := h.database.Model(&row).Updates(map[string]any{
		"status": models.AICalibrationStatusFailed, "error": message, "completed_at": &now,
	}).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "保存停止状态失败")
		return
	}
	row.Status, row.Error, row.CompletedAt = models.AICalibrationStatusFailed, &message, &now
	OK(c, calibrationResponse(row))
}

func (h *AIAnalysisHandler) runCalibration(ctx context.Context, id string, request services.AICalibrationRequest) {
	defer func() {
		h.calibrationMu.Lock()
		if cancel := h.calibrationStops[id]; cancel != nil {
			cancel()
			delete(h.calibrationStops, id)
		}
		h.calibrationMu.Unlock()
	}()
	result, err := h.upstream.Calibrate(ctx, request)
	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	now := time.Now().UTC()
	var row models.AICalibration
	if loadErr := h.database.First(&row, "id = ?", id).Error; loadErr != nil {
		fmt.Printf("AI calibration load failed for %s: %v\n", request.Ticker, loadErr)
		return
	}
	if row.Status != models.AICalibrationStatusRunning {
		return
	}
	row.CompletedAt = &now
	if err != nil {
		message := strings.TrimSpace(err.Error())
		if len(message) > 2000 {
			message = message[:2000]
		}
		row.Status = models.AICalibrationStatusFailed
		row.Error = &message
	} else {
		requestedAnalysts := row.Payload["requested_analysts"]
		encoded, _ := json.Marshal(result)
		payload := map[string]any{}
		_ = json.Unmarshal(encoded, &payload)
		if requestedAnalysts != nil {
			payload["requested_analysts"] = requestedAnalysts
		}
		row.Status = models.AICalibrationStatusComplete
		row.TotalNodes = result.TotalNodeCount
		if row.TotalNodes == 0 {
			row.TotalNodes = result.SampleCount
		}
		row.CompletedNodes = row.TotalNodes
		row.SampleCount = result.SampleCount
		row.CorrectCount = result.CorrectCount
		row.Accuracy = result.Accuracy
		row.Confidence = result.Confidence
		row.HorizonDays = result.HorizonDays
		row.Payload = payload
		row.Error = nil
	}
	if updateErr := h.database.Save(&row).Error; updateErr != nil {
		fmt.Printf("AI calibration persistence failed for %s: %v\n", request.Ticker, updateErr)
	}
}

func (h *AIAnalysisHandler) AblationStatus(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	var input aiAblationInput
	if symbol == "" || c.ShouldBindJSON(&input) != nil || normalizeAblationInput(&input) != nil {
		Fail(c, http.StatusBadRequest, "消融实验配置格式错误")
		return
	}
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, ablationHash(input)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "读取消融实验失败")
		return
	}
	if row.Status == models.AICalibrationStatusRunning && time.Since(row.StartedAt) > 12*time.Hour {
		message := "消融实验已超过十二小时，服务可能曾重启；可重新发起并复用已保存节点"
		now := time.Now().UTC()
		_ = h.database.Model(&row).Updates(map[string]any{
			"status": models.AICalibrationStatusFailed, "error": message, "completed_at": &now,
		}).Error
		row.Status, row.Error, row.CompletedAt = models.AICalibrationStatusFailed, &message, &now
	}
	OK(c, calibrationResponse(row))
}

func (h *AIAnalysisHandler) StartAblation(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	upstream, ok := h.upstream.(services.AIAbalationUpstream)
	if !ok {
		Fail(c, http.StatusNotImplemented, "当前 AI 上游不支持消融实验")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	var input aiAblationInput
	if symbol == "" || c.ShouldBindJSON(&input) != nil {
		Fail(c, http.StatusBadRequest, "消融实验配置格式错误")
		return
	}
	input.Ticker = symbol
	if err := normalizeAblationInput(&input); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.attachStoredAPIKey(c.Request.Context(), input.LLMProvider, input.BackendURL, &input.APIKey); err != nil {
		Fail(c, http.StatusInternalServerError, "后端保存的 LLM 密钥无法读取")
		return
	}
	hash := ablationHash(input)
	now := time.Now().UTC()

	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, hash).First(&row).Error
	if err == nil && row.Status == models.AICalibrationStatusRunning && time.Since(row.StartedAt) <= 12*time.Hour {
		c.JSON(http.StatusAccepted, APIResponse{Success: true, Data: calibrationResponse(row), Error: nil})
		return
	}
	if err == nil && row.Status == models.AICalibrationStatusComplete && !input.Refresh {
		OK(c, calibrationResponse(row))
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		Fail(c, http.StatusInternalServerError, "创建消融实验失败")
		return
	}
	totalRuns := input.MaximumNodes * input.Repeats * 2
	if *input.AdaptiveStages {
		totalRuns = input.MaximumNodes*2 + input.ReproducibilityNodes*2
	}
	row.Symbol = symbol
	row.ConfigHash = hash
	row.Provider = input.LLMProvider
	row.DeepThinkLLM = input.DeepThinkLLM
	row.QuickThinkLLM = input.QuickThinkLLM
	row.StrategyVersion = aiAblationStrategyVersion
	row.Status = models.AICalibrationStatusRunning
	row.TotalNodes = totalRuns
	row.CompletedNodes = 0
	row.CachedNodes = 0
	row.FailedNodes = 0
	row.SampleCount = 0
	row.CorrectCount = 0
	row.Accuracy = 0
	row.Confidence = "PENDING"
	row.HorizonDays = input.HorizonDays
	row.StartedAt = now
	row.CompletedAt = nil
	row.Error = nil
	row.Payload = map[string]any{
		"schema_version":        aiAblationStrategyVersion,
		"kind":                  "tradingagents_architecture_ablation",
		"start_date":            input.StartDate,
		"end_date":              input.EndDate,
		"maximum_nodes":         input.MaximumNodes,
		"repeats":               input.Repeats,
		"concurrency":           input.Concurrency,
		"transaction_cost_bps":  input.TransactionCostBP,
		"adaptive_stages":       *input.AdaptiveStages,
		"stage_one_nodes":       input.StageOneNodes,
		"reproducibility_nodes": input.ReproducibilityNodes,
	}
	if err := h.database.Save(&row).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "保存消融实验失败")
		return
	}
	request := services.AIAbalationRequest{
		AIAnalysisRequest:    input.AIAnalysisRequest,
		JobID:                row.ID.String(),
		StartDate:            input.StartDate,
		EndDate:              input.EndDate,
		HorizonDays:          input.HorizonDays,
		MaximumNodes:         input.MaximumNodes,
		Repeats:              input.Repeats,
		Concurrency:          input.Concurrency,
		TransactionCostBP:    input.TransactionCostBP,
		MaxAttempts:          input.MaxAttempts,
		AdaptiveStages:       *input.AdaptiveStages,
		StageOneNodes:        input.StageOneNodes,
		ReproducibilityNodes: input.ReproducibilityNodes,
	}
	request.Ticker = symbol
	request.Progress = func(progress services.AIAbalationProgress) {
		updates := map[string]any{
			"completed_nodes": progress.CompletedRuns,
			"total_nodes":     progress.TotalRuns,
			"cached_nodes":    progress.CachedRuns,
			"failed_nodes":    progress.FailedRuns,
		}
		_ = h.database.Model(&models.AICalibration{}).Where("id = ? AND status = ?", row.ID, models.AICalibrationStatusRunning).Updates(updates).Error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	h.calibrationStops[row.ID.String()] = cancel
	go h.runAblation(ctx, row.ID.String(), request, upstream)
	c.JSON(http.StatusAccepted, APIResponse{Success: true, Data: calibrationResponse(row), Error: nil})
}

func (h *AIAnalysisHandler) StopAblation(c *gin.Context) {
	if h.database == nil {
		Fail(c, http.StatusServiceUnavailable, "数据库不可用")
		return
	}
	symbol := normalizeSymbolParam(c.Param("symbol"))
	var input aiAblationInput
	if symbol == "" || c.ShouldBindJSON(&input) != nil || normalizeAblationInput(&input) != nil {
		Fail(c, http.StatusBadRequest, "消融实验配置格式错误")
		return
	}
	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	var row models.AICalibration
	err := h.database.Where("symbol = ? AND config_hash = ?", symbol, ablationHash(input)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "读取消融实验失败")
		return
	}
	if row.Status != models.AICalibrationStatusRunning {
		OK(c, calibrationResponse(row))
		return
	}
	if upstream, ok := h.upstream.(services.AIAbalationUpstream); ok {
		stopCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		err = upstream.CancelAblation(stopCtx, row.ID.String())
		cancel()
		if err != nil {
			Fail(c, http.StatusBadGateway, "停止 sidecar 消融实验失败："+err.Error())
			return
		}
	}
	if cancel := h.calibrationStops[row.ID.String()]; cancel != nil {
		cancel()
	}
	now := time.Now().UTC()
	message := "用户已停止消融实验；已完成的证据将在下次发起时复用"
	if err := h.database.Model(&row).Updates(map[string]any{
		"status": models.AICalibrationStatusFailed, "error": message, "completed_at": &now,
	}).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "保存停止状态失败")
		return
	}
	row.Status, row.Error, row.CompletedAt = models.AICalibrationStatusFailed, &message, &now
	OK(c, calibrationResponse(row))
}

func (h *AIAnalysisHandler) runAblation(ctx context.Context, id string, request services.AIAbalationRequest, upstream services.AIAbalationUpstream) {
	defer func() {
		h.calibrationMu.Lock()
		if cancel := h.calibrationStops[id]; cancel != nil {
			cancel()
			delete(h.calibrationStops, id)
		}
		h.calibrationMu.Unlock()
	}()
	result, err := upstream.Ablate(ctx, request)
	h.calibrationMu.Lock()
	defer h.calibrationMu.Unlock()
	now := time.Now().UTC()
	var row models.AICalibration
	if loadErr := h.database.First(&row, "id = ?", id).Error; loadErr != nil {
		fmt.Printf("AI ablation load failed for %s: %v\n", request.Ticker, loadErr)
		return
	}
	if row.Status != models.AICalibrationStatusRunning {
		return
	}
	row.CompletedAt = &now
	if err != nil {
		message := strings.TrimSpace(err.Error())
		if len(message) > 2000 {
			message = message[:2000]
		}
		row.Status = models.AICalibrationStatusFailed
		row.Error = &message
	} else {
		row.Payload = map[string]any(result)
		row.CompletedNodes = row.TotalNodes
		if value, ok := result["independent_nodes"].(float64); ok {
			row.SampleCount = int(value)
		}
		classification, _ := result["architecture_classification"].(string)
		row.Confidence = strings.ToUpper(classification)
		if status, _ := result["status"].(string); status == "complete" {
			row.Status = models.AICalibrationStatusComplete
			row.Error = nil
		} else {
			message := "消融实验存在失败运行；可重新发起并复用已完成证据"
			row.Status = models.AICalibrationStatusFailed
			row.Error = &message
		}
	}
	if updateErr := h.database.Save(&row).Error; updateErr != nil {
		fmt.Printf("AI ablation persistence failed for %s: %v\n", request.Ticker, updateErr)
	}
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
		upstream:         upstream,
		timeout:          timeout,
		calibrationStops: make(map[string]context.CancelFunc),
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
	h.enrichCalibration(c.Request.Context(), &payload, symbol)
	if err := h.attachStoredAPIKey(c.Request.Context(), payload.LLMProvider, payload.BackendURL, &payload.APIKey); err != nil {
		Fail(c, http.StatusInternalServerError, "后端保存的 LLM 密钥无法读取")
		return
	}

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

func (h *AIAnalysisHandler) enrichCalibration(ctx context.Context, payload *services.AIAnalysisRequest, symbol string) {
	if h.database == nil || payload == nil {
		return
	}
	var row models.AICalibration
	err := h.database.WithContext(ctx).
		Where("symbol = ? AND config_hash = ? AND status = ?", symbol, calibrationHash(*payload, 20), models.AICalibrationStatusComplete).
		First(&row).Error
	if err != nil {
		return
	}
	if payload.AnalysisContext == nil {
		payload.AnalysisContext = make(map[string]any)
	}
	payload.AnalysisContext["model_calibration"] = map[string]any{
		"schema_version": row.StrategyVersion,
		"sample_count":   row.SampleCount,
		"correct_count":  row.CorrectCount,
		"accuracy":       row.Accuracy,
		"confidence":     row.Confidence,
		"horizon_days":   row.HorizonDays,
		"completed_at":   row.CompletedAt,
		"rule":           "Historical calibration may reduce confidence; ten samples must not be treated as a precise probability.",
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
	payload.BackendURL = strings.TrimSpace(payload.BackendURL)
	if payload.Provider == "" {
		Fail(c, http.StatusBadRequest, "缺少 LLM 提供商")
		return
	}
	if payload.Provider == "openai_compatible" && payload.BackendURL == "" {
		Fail(c, http.StatusBadRequest, "OpenAI 兼容网关需要填写 API 地址")
		return
	}
	if err := h.attachStoredAPIKey(c.Request.Context(), payload.Provider, payload.BackendURL, &payload.APIKey); err != nil {
		Fail(c, http.StatusInternalServerError, "后端保存的 LLM 密钥无法读取")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result, err := h.upstream.Models(ctx, payload)
	if err != nil {
		Fail(c, http.StatusBadGateway, "获取模型列表失败："+err.Error())
		return
	}
	OK(c, result)
}

func (h *AIAnalysisHandler) attachStoredAPIKey(ctx context.Context, provider, backendURL string, target *string) error {
	if h.credentials == nil || strings.TrimSpace(provider) == "" || target == nil {
		return nil
	}
	apiKey, err := h.credentials.Resolve(ctx, provider, backendURL)
	if err != nil {
		return err
	}
	*target = apiKey
	return nil
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
