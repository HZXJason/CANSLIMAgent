package services

import (
	"context"
	"errors"
	"math"
	"sort"
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
	Symbol   string         `json:"symbol"`
	Name     string         `json:"name"`
	Market   string         `json:"market"`
	Metrics  MetricsPayload `json:"metrics"`
	DataAsOf string         `json:"data_as_of"`
}

type ScanEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
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

type ScannerService struct {
	db           *gorm.DB
	freeUniverse *FreeUniverseClient
	sec          *SECClient
	stooq        *StooqClient
	fmp          *FMPClient
	polygon      *PolygonClient
	yahoo        *YahooClient
	yfinance     *YFinanceClient
	scanDelay    time.Duration
	jobs         sync.Map
}

func NewScannerService(database *gorm.DB, freeUniverse *FreeUniverseClient, sec *SECClient, stooq *StooqClient, fmp *FMPClient, polygon *PolygonClient, yahoo *YahooClient, yfinance *YFinanceClient, scanDelay time.Duration) *ScannerService {
	return &ScannerService{
		db:           database,
		freeUniverse: freeUniverse,
		sec:          sec,
		stooq:        stooq,
		fmp:          fmp,
		polygon:      polygon,
		yahoo:        yahoo,
		yfinance:     yfinance,
		scanDelay:    scanDelay,
	}
}

func (s *ScannerService) Scan(ctx context.Context, market string) (<-chan ScanEvent, error) {
	if !models.ValidMarket(market) {
		return nil, errors.New(models.MarketDisabledReason(market))
	}

	events := make(chan ScanEvent)
	go func() {
		defer close(events)

		if !sendScanEvent(ctx, events, ScanEvent{Type: "scan_status", Data: map[string]interface{}{"message": "正在连接数据源并获取成分股列表"}}) {
			return
		}

		candidates, err := s.fetchUniverse(ctx, market)
		if err != nil {
			sendScanEvent(ctx, events, ScanEvent{Type: "scan_error", Data: map[string]interface{}{"message": err.Error()}})
			return
		}

		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return deleteStocksNotInUniverse(tx, market, candidates)
		}); err != nil {
			sendScanEvent(ctx, events, ScanEvent{Type: "scan_error", Data: map[string]interface{}{"message": err.Error()}})
			return
		}

		total := len(candidates)
		analyzed := 0
		pending := make([]pendingMetric, 0, len(candidates))

		if !sendScanEvent(ctx, events, ScanEvent{Type: "scan_progress", Data: map[string]interface{}{"total": total, "analyzed": analyzed}}) {
			return
		}
		for _, candidate := range candidates {
			select {
			case <-ctx.Done():
				return
			default:
			}

			symbol := strings.ToUpper(strings.TrimSpace(candidate.Symbol))
			if symbol != "" && !sendScanEvent(ctx, events, ScanEvent{Type: "scan_status", Data: map[string]interface{}{"message": "正在获取 " + symbol + " 指标数据", "symbol": symbol}}) {
				return
			}

			item, ok := s.buildPendingMetric(ctx, market, candidate)
			if !ok {
				continue
			}
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return upsertStockWithMetric(tx, item.stock, item.metric)
			}); err != nil {
				sendScanEvent(ctx, events, ScanEvent{Type: "scan_error", Data: map[string]interface{}{"message": err.Error()}})
				return
			}
			pending = append(pending, item)
			if !sendScanEvent(ctx, events, ScanEvent{Type: "stock_result", Data: pendingToStockResult(item)}) {
				return
			}
			analyzed++
			if !sendScanEvent(ctx, events, ScanEvent{Type: "scan_progress", Data: map[string]interface{}{"total": total, "analyzed": analyzed}}) {
				return
			}

			if s.scanDelay > 0 {
				time.Sleep(s.scanDelay)
			}
		}

		if !sendScanEvent(ctx, events, ScanEvent{Type: "scan_status", Data: map[string]interface{}{"message": "正在计算相对强度排名"}}) {
			return
		}
		applyRSRatings(pending)
		for _, item := range pending {
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return upsertStockWithMetric(tx, item.stock, item.metric)
			}); err != nil {
				sendScanEvent(ctx, events, ScanEvent{Type: "scan_error", Data: map[string]interface{}{"message": err.Error()}})
				return
			}
			if !sendScanEvent(ctx, events, ScanEvent{Type: "stock_result", Data: pendingToStockResult(item)}) {
				return
			}
		}
		sendScanEvent(ctx, events, ScanEvent{Type: "scan_complete", Data: map[string]interface{}{"total": total, "analyzed": analyzed}})
	}()

	return events, nil
}

func sendScanEvent(ctx context.Context, events chan<- ScanEvent, event ScanEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
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

func (s *ScannerService) StartRefresh(_ context.Context, market string) (*RefreshJob, error) {
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
		err := s.refreshMarketData(context.Background(), market)
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
			"n_label":            nLabel,
			"n_analysis_at":      now,
			"n_analysis_summary": summary,
		}).Error
	return now, err
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
	if s.stooq != nil && s.stooq.Configured() {
		price, err := s.stooq.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			return price, true
		}
	}
	if s.polygon != nil && s.polygon.Configured() {
		price, err := s.polygon.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			return price, true
		}
	}
	if s.fmp != nil && s.fmp.Configured() {
		price, err := s.fmp.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			return price, true
		}
	}
	if s.yahoo != nil && s.yahoo.Configured() {
		price, err := s.yahoo.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
			return price, true
		}
	}
	if s.yfinance != nil && s.yfinance.Configured() {
		price, err := s.yfinance.FetchPriceSignal(ctx, symbol, asOf)
		if err == nil && price.Sufficient {
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
			C: MetricPayload{Value: floatValue(metric.CEPSGrowthPct), Sufficient: metric.CDataSufficient},
			A: MetricPayload{Value: metric.AAnnualEPSGrowth, Sufficient: metric.ADataSufficient},
			S: MetricPayload{Value: floatValue(metric.SVolumeRatio), Sufficient: metric.SDataSufficient},
			L: MetricPayload{Value: intValue(metric.LRSRating), Sufficient: metric.LDataSufficient},
			I: MetricPayload{Value: intValue(metric.IInstitutionDelta), Sufficient: metric.IDataSufficient},
			N: metric.NLabel,
		},
		DataAsOf: metric.DataAsOf.Format("2006-01-02"),
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
