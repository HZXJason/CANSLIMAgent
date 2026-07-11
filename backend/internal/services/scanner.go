package services

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MetricPayload struct {
	Value      interface{} `json:"value"`
	Sufficient bool        `json:"sufficient"`
}

type MetricsPayload struct {
	C MetricPayload `json:"c"`
	A MetricPayload `json:"a"`
	S MetricPayload `json:"s"`
	L MetricPayload `json:"l"`
	I MetricPayload `json:"i"`
	N *bool         `json:"n"`
}

type StockResult struct {
	Symbol     string         `json:"symbol"`
	Name       string         `json:"name"`
	Market     string         `json:"market"`
	Metrics    MetricsPayload `json:"metrics"`
	AIAnalyzed bool           `json:"ai_analyzed"`
	DataAsOf   string         `json:"data_as_of"`
}

// MetricSourcePayload identifies the provider that supplied a live per-symbol
// backfill, so clients can distinguish an old cache from unavailable upstream data.
type MetricSourcePayload struct {
	Source string `json:"source"`
	Status string `json:"status"`
}

type ScanEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type scanStatusPayload struct {
	Message string `json:"message"`
	Symbol  string `json:"symbol,omitempty"`
}

type scanProgressPayload struct {
	Total    int `json:"total"`
	Analyzed int `json:"analyzed"`
}

type RefreshJob struct {
	ID         string     `json:"id"`
	Market     string     `json:"market"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type pendingMetric struct {
	stock        models.Stock
	metric       models.StockMetric
	rsBase       *float64
	rsSufficient bool
}

type scanRun struct {
	id          string
	market      string
	createdAt   time.Time
	finishedAt  *time.Time
	status      string
	errorText   string
	events      []ScanEvent
	subscribers map[chan ScanEvent]struct{}
	mu          sync.Mutex
}

type ScannerService struct {
	db            *gorm.DB
	freeUniverse  *FreeUniverseClient
	sec           *SECClient
	institutional *InstitutionalClient
	stooq         *StooqClient
	fmp           *FMPClient
	polygon       *PolygonClient
	yahoo         *YahooClient
	yfinance      *YFinanceClient
	scanDelay     time.Duration
	jobs          sync.Map
	scanMu        sync.Mutex
	scanRuns      map[string]*scanRun
}

func NewScannerService(database *gorm.DB, freeUniverse *FreeUniverseClient, sec *SECClient, institutional *InstitutionalClient, stooq *StooqClient, fmp *FMPClient, polygon *PolygonClient, yahoo *YahooClient, yfinance *YFinanceClient, scanDelay time.Duration) *ScannerService {
	return &ScannerService{
		db:            database,
		freeUniverse:  freeUniverse,
		sec:           sec,
		institutional: institutional,
		stooq:         stooq,
		fmp:           fmp,
		polygon:       polygon,
		yahoo:         yahoo,
		yfinance:      yfinance,
		scanDelay:     scanDelay,
		scanRuns:      make(map[string]*scanRun),
	}
}

func (s *ScannerService) Scan(ctx context.Context, market string) (<-chan ScanEvent, error) {
	if !models.ValidMarket(market) {
		return nil, errors.New(models.MarketDisabledReason(market))
	}

	run := s.scanRunForMarket(market)
	return run.subscribe(ctx), nil
}

func (s *ScannerService) scanRunForMarket(market string) *scanRun {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	for key, run := range s.scanRuns {
		if !run.isRunning() && run.finishedAt != nil && time.Since(*run.finishedAt) > 10*time.Minute {
			delete(s.scanRuns, key)
		}
	}

	if run, ok := s.scanRuns[market]; ok && run.isRunning() {
		return run
	}

	run := newScanRun(market)
	s.scanRuns[market] = run
	go s.runResumableScan(run)
	return run
}

func newScanRun(market string) *scanRun {
	return &scanRun{
		id:          uuid.NewString(),
		market:      market,
		createdAt:   time.Now().UTC(),
		status:      "running",
		events:      make([]ScanEvent, 0, 256),
		subscribers: make(map[chan ScanEvent]struct{}),
	}
}

func (r *scanRun) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status == "running"
}

func (r *scanRun) subscribe(ctx context.Context) <-chan ScanEvent {
	r.mu.Lock()
	bufferSize := len(r.events) + 256
	if bufferSize < 512 {
		bufferSize = 512
	}
	events := make(chan ScanEvent, bufferSize)
	for _, event := range r.events {
		events <- event
	}
	if r.status != "running" {
		r.mu.Unlock()
		close(events)
		return events
	}
	r.subscribers[events] = struct{}{}
	r.mu.Unlock()

	go func() {
		<-ctx.Done()
		r.unsubscribe(events)
	}()
	return events
}

func (r *scanRun) unsubscribe(events chan ScanEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subscribers[events]; !ok {
		return
	}
	delete(r.subscribers, events)
	close(events)
}

func (r *scanRun) emit(event ScanEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	for events := range r.subscribers {
		select {
		case events <- event:
		default:
		}
	}
}

func (r *scanRun) finish(status string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
	if err != nil {
		r.errorText = err.Error()
	}
	finishedAt := time.Now().UTC()
	r.finishedAt = &finishedAt
	for events := range r.subscribers {
		close(events)
		delete(r.subscribers, events)
	}
	r.events = nil
}

func (s *ScannerService) runResumableScan(run *scanRun) {
	ctx := context.Background()
	fail := func(err error) {
		run.emit(ScanEvent{Type: "scan_error", Data: scanStatusPayload{Message: err.Error()}})
		run.finish("failed", err)
	}

	run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "正在连接数据源并获取成分股列表"}})
	candidates, err := s.fetchUniverse(ctx, run.market)
	if err != nil {
		fail(err)
		return
	}

	run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "正在读取本地缓存，准备断点接续"}})
	cached, cachedSymbols, err := s.cachedResultsForUniverse(ctx, run.market, candidates)
	if err != nil {
		fail(err)
		return
	}

	total := len(candidates)
	analyzed := len(cachedSymbols)
	for _, result := range cached {
		run.emit(ScanEvent{Type: "stock_result", Data: result})
	}
	run.emit(ScanEvent{Type: "scan_progress", Data: scanProgressPayload{Total: total, Analyzed: analyzed}})
	if analyzed > 0 && analyzed < total {
		run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "已保留 " + intText(analyzed) + " 支缓存结果，继续补齐剩余股票"}})
	}

	pending := make([]pendingMetric, 0, total-analyzed)
	for _, candidate := range candidates {
		symbol := strings.ToUpper(strings.TrimSpace(candidate.Symbol))
		if symbol == "" {
			continue
		}
		if _, ok := cachedSymbols[symbol]; ok {
			continue
		}

		run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "正在获取 " + symbol + " 指标数据", Symbol: symbol}})
		item, ok := s.buildPendingMetric(ctx, run.market, candidate)
		if !ok {
			continue
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return upsertStockWithMetric(tx, item.stock, item.metric)
		}); err != nil {
			fail(err)
			return
		}

		pending = append(pending, item)
		run.emit(ScanEvent{Type: "stock_result", Data: pendingToStockResult(item)})
		analyzed++
		run.emit(ScanEvent{Type: "scan_progress", Data: scanProgressPayload{Total: total, Analyzed: analyzed}})

		if s.scanDelay > 0 {
			timer := time.NewTimer(s.scanDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				run.finish("cancelled", ctx.Err())
				return
			case <-timer.C:
			}
		}
	}

	if len(pending) > 0 {
		run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "正在计算新增股票的相对强度排名"}})
		applyRSRatings(pending)
		for _, item := range pending {
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return upsertStockWithMetric(tx, item.stock, item.metric)
			}); err != nil {
				fail(err)
				return
			}
			run.emit(ScanEvent{Type: "stock_result", Data: pendingToStockResult(item)})
		}
	} else {
		run.emit(ScanEvent{Type: "scan_status", Data: scanStatusPayload{Message: "本地缓存已覆盖该市场；如需覆盖重算，请使用刷新数据"}})
	}

	run.emit(ScanEvent{Type: "scan_complete", Data: scanProgressPayload{Total: total, Analyzed: analyzed}})
	run.finish("completed", nil)
}

func (s *ScannerService) cachedResultsForUniverse(ctx context.Context, market string, candidates []StockCandidate) ([]StockResult, map[string]struct{}, error) {
	candidateSymbols := make([]string, 0, len(candidates))
	seenCandidates := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		symbol := strings.ToUpper(strings.TrimSpace(candidate.Symbol))
		if symbol == "" {
			continue
		}
		if _, ok := seenCandidates[symbol]; ok {
			continue
		}
		seenCandidates[symbol] = struct{}{}
		candidateSymbols = append(candidateSymbols, symbol)
	}

	cachedSymbols := make(map[string]struct{}, len(candidateSymbols))
	if len(candidateSymbols) == 0 {
		return nil, cachedSymbols, nil
	}

	var stocks []models.Stock
	if err := s.db.WithContext(ctx).
		Preload("Metric").
		Where("market = ? AND symbol IN ?", market, candidateSymbols).
		Find(&stocks).Error; err != nil {
		return nil, nil, err
	}

	resultsBySymbol := make(map[string]StockResult, len(stocks))
	for _, stock := range stocks {
		if !isCompleteCachedMetric(stock.Metric, s.institutional != nil && s.institutional.Configured()) {
			continue
		}
		result := toStockResult(stock)
		resultsBySymbol[result.Symbol] = result
		cachedSymbols[result.Symbol] = struct{}{}
	}

	results := make([]StockResult, 0, len(resultsBySymbol))
	for _, symbol := range candidateSymbols {
		if result, ok := resultsBySymbol[symbol]; ok {
			results = append(results, result)
		}
	}
	return results, cachedSymbols, nil
}

func intText(value int) string {
	return strconv.Itoa(value)
}

func isCompleteCachedMetric(metric models.StockMetric, institutionalRequired bool) bool {
	if metric.ID == 0 {
		return false
	}
	if metric.SDataSufficient && metric.SVolumeRatio == nil {
		return false
	}
	if metric.LDataSufficient && metric.LRSRating == nil {
		return false
	}
	if institutionalRequired && metric.IInstitutionDelta == nil {
		return false
	}
	if metric.IDataSufficient && metric.IInstitutionDelta == nil {
		return false
	}
	return true
}

func (s *ScannerService) GetCachedResults(ctx context.Context, market string) ([]StockResult, error) {
	if !models.ValidMarket(market) {
		return nil, errors.New(models.MarketDisabledReason(market))
	}

	var stocks []models.Stock
	if err := s.db.WithContext(ctx).
		Preload("Metric").
		Where("market = ?", market).
		Order("symbol asc").
		Find(&stocks).Error; err != nil {
		return nil, err
	}

	results := make([]StockResult, 0, len(stocks))
	for _, stock := range stocks {
		if stock.Metric.ID == 0 {
			continue
		}
		results = append(results, toStockResult(stock))
	}
	return results, nil
}

func (s *ScannerService) StartRefresh(ctx context.Context, market string) (*RefreshJob, error) {
	if !models.ValidMarket(market) {
		return nil, errors.New(models.MarketDisabledReason(market))
	}

	job := &RefreshJob{
		ID:        uuid.NewString(),
		Market:    market,
		Status:    "running",
		CreatedAt: time.Now().UTC(),
	}
	s.jobs.Store(job.ID, job)

	go func() {
		// A refresh outlives the short POST request that created it. Passing the
		// request context here cancelled the job as soon as the handler replied,
		// which the UI misleadingly presented as a network failure.
		err := s.refreshMarketData(refreshJobContext(ctx), market)
		finishedAt := time.Now().UTC()
		job.FinishedAt = &finishedAt
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "completed"
		}
		s.jobs.Store(job.ID, job)
	}()

	return job, nil
}

func refreshJobContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

func (s *ScannerService) GetRefreshJob(jobID string) (*RefreshJob, bool) {
	value, ok := s.jobs.Load(jobID)
	if !ok {
		return nil, false
	}
	job, ok := value.(*RefreshJob)
	return job, ok
}

func (s *ScannerService) UpdateNLabel(ctx context.Context, symbol string, nLabel bool, summary string) (time.Time, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return time.Time{}, errors.New("股票代码不能为空")
	}

	var stock models.Stock
	if err := s.db.WithContext(ctx).Where("symbol = ?", symbol).First(&stock).Error; err != nil {
		return time.Time{}, err
	}

	now := time.Now().UTC()
	err := s.db.WithContext(ctx).
		Model(&models.StockMetric{}).
		Where("stock_id = ?", stock.ID).
		Updates(map[string]interface{}{
			"ai_analysis_at":     now,
			"n_label":            nLabel,
			"n_analysis_at":      now,
			"n_analysis_summary": summary,
		}).Error
	return now, err
}

func (s *ScannerService) MarkAIAnalysisCompleted(ctx context.Context, symbol string) (time.Time, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return time.Time{}, errors.New("股票代码不能为空")
	}

	var stock models.Stock
	if err := s.db.WithContext(ctx).Where("symbol = ?", symbol).Order("market asc").First(&stock).Error; err != nil {
		return time.Time{}, err
	}
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Model(&models.StockMetric{}).Where("stock_id = ?", stock.ID).Update("ai_analysis_at", now).Error
	return now, err
}

// HydrateStockMetrics refreshes the S/L/I metrics needed by an individual
// analysis tab without a full market scan. The screener still calculates L as
// a cross-sectional rank; a single-stock request uses the same bounded score
// against SPY so it never fabricates a market percentile from one observation.
func (s *ScannerService) HydrateStockMetrics(ctx context.Context, symbol string) (map[string]MetricSourcePayload, bool, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, false, errors.New("股票代码不能为空")
	}

	var stock models.Stock
	if err := s.db.WithContext(ctx).Preload("Metric").Where("symbol = ?", symbol).Order("market asc").First(&stock).Error; err != nil {
		return nil, false, err
	}
	if !needsSingleStockHydration(stock.Metric) {
		return map[string]MetricSourcePayload{
			"s": {Source: "本地缓存", Status: "cached"},
			"l": {Source: "本地缓存", Status: "cached"},
			"i": {Source: "本地缓存", Status: "cached"},
		}, false, nil
	}

	sources := map[string]MetricSourcePayload{}
	metric := stock.Metric
	now := time.Now().UTC()
	price, priceOK := s.fetchPriceSignal(ctx, stock.Symbol, now)
	if priceOK && price.VolumeRatio != nil {
		metric.SVolumeRatio = price.VolumeRatio
		metric.SDataSufficient = true
		if price.DataAsOf != nil && !price.DataAsOf.IsZero() {
			metric.DataAsOf = *price.DataAsOf
		}
		sources["s"] = MetricSourcePayload{Source: price.Source, Status: "live"}
	} else {
		metric.SVolumeRatio = nil
		metric.SDataSufficient = false
		sources["s"] = MetricSourcePayload{Source: "价格路由", Status: "unavailable"}
	}

	if priceOK && price.TwelveMonthReturn != nil {
		benchmark, benchmarkOK := s.fetchPriceSignal(ctx, "SPY", now)
		if benchmarkOK && benchmark.TwelveMonthReturn != nil {
			rating := int(math.Round(rsScore(*price.TwelveMonthReturn, *benchmark.TwelveMonthReturn)))
			metric.LRSRating = &rating
			metric.LDataSufficient = true
			sources["l"] = MetricSourcePayload{Source: price.Source + " / " + benchmark.Source + "（相对 SPY）", Status: "live"}
		} else {
			metric.LRSRating = nil
			metric.LDataSufficient = false
			sources["l"] = MetricSourcePayload{Source: "价格路由 / SPY", Status: "unavailable"}
		}
	} else {
		metric.LRSRating = nil
		metric.LDataSufficient = false
		sources["l"] = MetricSourcePayload{Source: "价格路由", Status: "unavailable"}
	}

	if s.institutional != nil && s.institutional.Configured() {
		signal, err := s.institutional.FetchInstitutionalSignal(ctx, stock.Symbol)
		if err == nil && signal.Sufficient && signal.Delta != nil {
			metric.IInstitutionDelta = signal.Delta
			metric.IDataSufficient = true
			sources["i"] = MetricSourcePayload{Source: "Nasdaq 机构持仓", Status: "live"}
		} else {
			metric.IInstitutionDelta = nil
			metric.IDataSufficient = false
			sources["i"] = MetricSourcePayload{Source: "Nasdaq 机构持仓", Status: "unavailable"}
		}
	} else {
		sources["i"] = MetricSourcePayload{Source: "机构持仓数据源未启用", Status: "disabled"}
	}

	metric.FetchedAt = now
	if err := s.db.WithContext(ctx).Model(&models.StockMetric{}).Where("stock_id = ?", stock.ID).Updates(map[string]interface{}{
		"s_volume_ratio":      metric.SVolumeRatio,
		"s_data_sufficient":   metric.SDataSufficient,
		"l_rs_rating":         metric.LRSRating,
		"l_data_sufficient":   metric.LDataSufficient,
		"i_institution_delta": metric.IInstitutionDelta,
		"i_data_sufficient":   metric.IDataSufficient,
		"data_as_of":          metric.DataAsOf,
		"fetched_at":          metric.FetchedAt,
	}).Error; err != nil {
		return sources, false, err
	}
	return sources, true, nil
}

func needsSingleStockHydration(metric models.StockMetric) bool {
	if !metric.SDataSufficient || metric.SVolumeRatio == nil || !metric.LDataSufficient || metric.LRSRating == nil || !metric.IDataSufficient || metric.IInstitutionDelta == nil {
		return true
	}
	return metric.DataAsOf.IsZero() || metric.DataAsOf.Before(time.Now().UTC().AddDate(0, 0, -3))
}

func (s *ScannerService) ensureMarketData(ctx context.Context, market string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.StockMetric{}).
		Joins("JOIN stocks ON stocks.id = stock_metrics.stock_id").
		Where("stocks.market = ?", market).
		Count(&count).Error; err != nil {
		return err
	}
	var latest time.Time
	if count > 0 {
		_ = s.db.WithContext(ctx).Model(&models.StockMetric{}).
			Joins("JOIN stocks ON stocks.id = stock_metrics.stock_id").
			Where("stocks.market = ?", market).
			Select("MAX(stock_metrics.data_as_of)").
			Scan(&latest).Error
	}
	var legacySyntheticCount int64
	if count > 0 {
		_ = s.db.WithContext(ctx).Model(&models.StockMetric{}).
			Joins("JOIN stocks ON stocks.id = stock_metrics.stock_id").
			Where("stocks.market = ? AND stock_metrics.i_data_sufficient = ?", market, true).
			Count(&legacySyntheticCount).Error
	}
	if count > 0 && legacySyntheticCount == 0 && !latest.IsZero() && latest.After(time.Now().UTC().AddDate(0, 0, -5)) {
		return nil
	}
	return s.refreshMarketData(ctx, market)
}

func (s *ScannerService) refreshMarketData(ctx context.Context, market string) error {
	candidates, err := s.fetchUniverse(ctx, market)
	if err != nil {
		return err
	}

	pending := make([]pendingMetric, 0, len(candidates))
	for _, candidate := range candidates {
		item, ok := s.buildPendingMetric(ctx, market, candidate)
		if !ok {
			continue
		}
		pending = append(pending, item)
	}

	applyRSRatings(pending)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleteStocksNotInUniverse(tx, market, candidates); err != nil {
			return err
		}
		for _, item := range pending {
			if err := upsertStockWithMetric(tx, item.stock, item.metric); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *ScannerService) buildPendingMetric(ctx context.Context, market string, candidate StockCandidate) (pendingMetric, bool) {
	stock := models.Stock{
		Symbol: strings.ToUpper(strings.TrimSpace(candidate.Symbol)),
		Name:   strings.TrimSpace(candidate.Name),
		Market: market,
	}
	if stock.Symbol == "" || stock.Name == "" {
		return pendingMetric{}, false
	}

	metric := emptyMetric(time.Now().UTC())
	var rsBase *float64
	rsSufficient := false

	s.applyEPSFallback(ctx, stock.Symbol, &metric)
	s.applyInstitutionalSignal(ctx, stock.Symbol, &metric)

	price, ok := s.fetchPriceSignal(ctx, stock.Symbol, metric.DataAsOf)
	if ok {
		metric.SVolumeRatio = price.VolumeRatio
		metric.SDataSufficient = price.Sufficient && price.VolumeRatio != nil
		rsBase = price.TwelveMonthReturn
		rsSufficient = price.Sufficient && price.TwelveMonthReturn != nil
		if price.DataAsOf != nil && !price.DataAsOf.IsZero() {
			metric.DataAsOf = *price.DataAsOf
		}
	} else {
		metric.SDataSufficient = false
		metric.LRSRating = nil
		metric.LDataSufficient = false
	}

	return pendingMetric{stock: stock, metric: metric, rsBase: rsBase, rsSufficient: rsSufficient}, true
}

func (s *ScannerService) applyInstitutionalSignal(ctx context.Context, symbol string, metric *models.StockMetric) {
	if s.institutional == nil || !s.institutional.Configured() {
		return
	}
	signal, err := s.institutional.FetchInstitutionalSignal(ctx, symbol)
	if err != nil || !signal.Sufficient || signal.Delta == nil {
		return
	}
	metric.IInstitutionDelta = signal.Delta
	metric.IDataSufficient = true
}

func (s *ScannerService) fetchUniverse(ctx context.Context, market string) ([]StockCandidate, error) {
	if s.freeUniverse != nil && s.freeUniverse.Configured() {
		candidates, err := s.freeUniverse.FetchUniverse(ctx, market)
		if err == nil && len(candidates) > 0 {
			return candidates, nil
		}
	}
	if s.fmp != nil && s.fmp.Configured() {
		candidates, err := s.fmp.FetchUniverse(ctx, market)
		if err == nil && len(candidates) > 0 {
			return candidates, nil
		}
	}
	if s.yfinance != nil && s.yfinance.Configured() {
		candidates, err := s.yfinance.FetchUniverse(ctx, market)
		if err == nil && len(candidates) > 0 {
			return candidates, nil
		}
	}
	return nil, errors.New("无法连接真实数据源获取成分股列表，请检查网络连接、代理或 API Key 配置")
}

func (s *ScannerService) applyEPSFallback(ctx context.Context, symbol string, metric *models.StockMetric) {
	if s.sec != nil && s.sec.Configured() {
		quarter, quarterSufficient, annual, years, annualSufficient, asOf, err := s.sec.FetchEPSGrowth(ctx, symbol)
		if err == nil {
			metric.CEPSGrowthPct = quarter
			metric.CDataSufficient = quarter != nil && quarterSufficient
			metric.AAnnualEPSGrowth = annual
			metric.AYearsAvailable = years
			metric.ADataSufficient = len(annual) >= 3 && annualSufficient
			if !asOf.IsZero() {
				metric.DataAsOf = asOf
			}
			return
		}
	}

	if s.fmp != nil && s.fmp.Configured() {
		quarter, quarterSufficient, annual, years, annualSufficient, asOf, err := s.fmp.FetchEPSGrowth(ctx, symbol)
		if err == nil {
			metric.CEPSGrowthPct = quarter
			metric.CDataSufficient = quarter != nil && quarterSufficient
			metric.AAnnualEPSGrowth = annual
			metric.AYearsAvailable = years
			metric.ADataSufficient = len(annual) >= 3 && annualSufficient
			if !asOf.IsZero() {
				metric.DataAsOf = asOf
			}
			return
		}
	}

	if s.yfinance != nil && s.yfinance.Configured() {
		quarter, quarterSufficient, annual, years, annualSufficient, asOf, err := s.yfinance.FetchEPSGrowth(ctx, symbol)
		if err == nil {
			metric.CEPSGrowthPct = quarter
			metric.CDataSufficient = quarter != nil && quarterSufficient
			metric.AAnnualEPSGrowth = annual
			metric.AYearsAvailable = years
			metric.ADataSufficient = len(annual) >= 3 && annualSufficient
			if !asOf.IsZero() {
				metric.DataAsOf = asOf
			}
			return
		}
	}
}

func (s *ScannerService) fetchPriceSignal(ctx context.Context, symbol string, asOf time.Time) (priceSignal, bool) {
	// Vibe-Trading's US-equity router leads with Yahoo because its public chart
	// endpoint is generally more throttle-tolerant than Stooq's browser-guarded
	// download endpoint. Keep every existing provider as a fallback.
	if s.yahoo != nil && s.yahoo.Configured() {
		price, err := s.yahoo.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			price.Source = "Yahoo Finance"
			return price, true
		}
	}
	if s.stooq != nil && s.stooq.Configured() {
		price, err := s.stooq.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			price.Source = "Stooq"
			return price, true
		}
	}
	if s.polygon != nil && s.polygon.Configured() {
		price, err := s.polygon.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			price.Source = "Polygon"
			return price, true
		}
	}
	if s.fmp != nil && s.fmp.Configured() {
		price, err := s.fmp.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			price.Source = "FMP"
			return price, true
		}
	}
	if s.yfinance != nil && s.yfinance.Configured() {
		price, err := s.yfinance.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			price.Source = "yfinance"
			return price, true
		}
	}
	return priceSignal{}, false
}

func emptyMetric(dataAsOf time.Time) models.StockMetric {
	return models.StockMetric{
		CDataSufficient:  false,
		AAnnualEPSGrowth: []float64{},
		ADataSufficient:  false,
		SDataSufficient:  false,
		LDataSufficient:  false,
		IDataSufficient:  false,
		DataAsOf:         dataAsOf,
	}
}

func deleteStocksNotInUniverse(tx *gorm.DB, market string, candidates []StockCandidate) error {
	symbols := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		symbol := strings.ToUpper(strings.TrimSpace(candidate.Symbol))
		if symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	if len(symbols) == 0 {
		return nil
	}
	staleStocks := tx.Model(&models.Stock{}).
		Select("id").
		Where("market = ? AND symbol NOT IN ?", market, symbols)
	if err := tx.Where("stock_id IN (?)", staleStocks).Delete(&models.StockMetric{}).Error; err != nil {
		return err
	}
	return tx.Where("market = ? AND symbol NOT IN ?", market, symbols).Delete(&models.Stock{}).Error
}

func upsertStockWithMetric(tx *gorm.DB, stock models.Stock, metric models.StockMetric) error {
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "symbol"}, {Name: "market"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "updated_at"}),
	}).Create(&stock).Error; err != nil {
		return err
	}

	if err := tx.Where("symbol = ? AND market = ?", stock.Symbol, stock.Market).First(&stock).Error; err != nil {
		return err
	}

	metric.StockID = stock.ID
	metric.FetchedAt = time.Now().UTC()
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stock_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"c_eps_growth_pct", "c_data_sufficient",
			"a_annual_eps_growth", "a_years_available", "a_data_sufficient",
			"s_volume_ratio", "s_data_sufficient",
			"l_rs_rating", "l_data_sufficient",
			"i_institution_delta", "i_data_sufficient",
			"data_as_of", "fetched_at",
		}),
	}).Create(&metric).Error
}

func toStockResult(stock models.Stock) StockResult {
	metric := stock.Metric
	return StockResult{
		Symbol: stock.Symbol,
		Name:   stock.Name,
		Market: stock.Market,
		Metrics: MetricsPayload{
			C: MetricPayload{Value: floatValue(metric.CEPSGrowthPct), Sufficient: metric.CDataSufficient && metric.CEPSGrowthPct != nil},
			A: MetricPayload{Value: metric.AAnnualEPSGrowth, Sufficient: metric.ADataSufficient && len(metric.AAnnualEPSGrowth) > 0},
			S: MetricPayload{Value: floatValue(metric.SVolumeRatio), Sufficient: metric.SDataSufficient && metric.SVolumeRatio != nil},
			L: MetricPayload{Value: intValue(metric.LRSRating), Sufficient: metric.LDataSufficient && metric.LRSRating != nil},
			I: MetricPayload{Value: intValue(metric.IInstitutionDelta), Sufficient: metric.IDataSufficient && metric.IInstitutionDelta != nil},
			N: metric.NLabel,
		},
		AIAnalyzed: metric.AIAnalysisAt != nil,
		DataAsOf:   metric.DataAsOf.Format("2006-01-02"),
	}
}

func pendingToStockResult(item pendingMetric) StockResult {
	stock := item.stock
	stock.Metric = item.metric
	return toStockResult(stock)
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func applyRSRatings(pending []pendingMetric) {
	type pair struct {
		index int
		value float64
	}

	pairs := make([]pair, 0, len(pending))
	for index, item := range pending {
		if item.rsSufficient && item.rsBase != nil {
			pairs = append(pairs, pair{index: index, value: *item.rsBase})
		}
	}
	if len(pairs) == 0 {
		return
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].value < pairs[j].value
	})
	for rank, item := range pairs {
		rating := int(math.Round(float64(rank+1) / float64(len(pairs)) * 99))
		if rating < 1 {
			rating = 1
		}
		if rating > 99 {
			rating = 99
		}
		pending[item.index].metric.LRSRating = &rating
		pending[item.index].metric.LDataSufficient = true
	}
	for index := range pending {
		if !pending[index].rsSufficient && pending[index].rsBase == nil && pending[index].metric.LRSRating == nil {
			pending[index].metric.LDataSufficient = false
		}
	}
}
