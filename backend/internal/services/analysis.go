package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"gorm.io/gorm"
)

const (
	PeriodDaily   = "daily"
	PeriodWeekly  = "weekly"
	PeriodMonthly = "monthly"
)

type AnalysisService struct {
	db           *gorm.DB
	alphaVantage *AlphaVantageClient
	translator   *TranslationClient
	ashare       *AshareClient
	sec          *SECClient
	stooq        *StooqClient
	fmp          *FMPClient
	polygon      *PolygonClient
	yahoo        *YahooClient
	yfinance     *YFinanceClient
}

type StockSummary struct {
	Symbol           string                         `json:"symbol"`
	Name             string                         `json:"name"`
	Market           string                         `json:"market"`
	Metrics          MetricsPayload                 `json:"metrics"`
	AIAnalyzed       bool                           `json:"ai_analyzed"`
	DataAsOf         string                         `json:"data_as_of"`
	NAnalysisAt      *time.Time                     `json:"n_analysis_at"`
	NAnalysisSummary string                         `json:"n_analysis_summary"`
	MetricSources    map[string]MetricSourcePayload `json:"metric_sources,omitempty"`
}

type CompanyProfile struct {
	Symbol      string `json:"symbol"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Sector      string `json:"sector"`
	Industry    string `json:"industry"`
	Website     string `json:"website"`
	Source      string `json:"source"`
	UpdatedAt   string `json:"updated_at"`
}

type Candle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

type KLineData struct {
	Symbol   string   `json:"symbol"`
	Period   string   `json:"period"`
	Source   string   `json:"source"`
	DataAsOf string   `json:"data_as_of"`
	Candles  []Candle `json:"candles"`
}

type RSPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type RSData struct {
	Symbol   string    `json:"symbol"`
	Period   string    `json:"period"`
	Source   string    `json:"source"`
	DataAsOf string    `json:"data_as_of"`
	RSLine   []RSPoint `json:"rs_line"`
}

type EPSQuarter struct {
	Period         string  `json:"period"`
	EPS            float64 `json:"eps"`
	EPSYoYGrowth   float64 `json:"eps_yoy_growth"`
	DataSufficient bool    `json:"data_sufficient"`
	ReportDate     string  `json:"report_date"`
}

type EPSData struct {
	Symbol   string       `json:"symbol"`
	Source   string       `json:"source"`
	DataAsOf string       `json:"data_as_of"`
	Quarters []EPSQuarter `json:"quarters"`
}

type RevenueYear struct {
	Year             int     `json:"year"`
	Revenue          float64 `json:"revenue"`
	RevenueYoYGrowth float64 `json:"revenue_yoy_growth"`
	DataSufficient   bool    `json:"data_sufficient"`
}

type RevenueData struct {
	Symbol   string        `json:"symbol"`
	Source   string        `json:"source"`
	DataAsOf string        `json:"data_as_of"`
	Years    []RevenueYear `json:"years"`
}

type GrossMarginQuarter struct {
	Period      string  `json:"period"`
	GrossMargin float64 `json:"gross_margin"`
	ReportDate  string  `json:"report_date"`
}

type GrossMarginData struct {
	Symbol   string               `json:"symbol"`
	Source   string               `json:"source"`
	DataAsOf string               `json:"data_as_of"`
	Quarters []GrossMarginQuarter `json:"quarters"`
}

type SecondLayerData map[string]interface{}

type candleSourceResult struct {
	source  string
	candles []Candle
}

func NewAnalysisService(database *gorm.DB, alphaVantage *AlphaVantageClient, translator *TranslationClient, ashare *AshareClient, sec *SECClient, stooq *StooqClient, fmp *FMPClient, polygon *PolygonClient, yahoo *YahooClient, yfinance *YFinanceClient) *AnalysisService {
	return &AnalysisService{
		db:           database,
		alphaVantage: alphaVantage,
		translator:   translator,
		ashare:       ashare,
		sec:          sec,
		stooq:        stooq,
		fmp:          fmp,
		polygon:      polygon,
		yahoo:        yahoo,
		yfinance:     yfinance,
	}
}

func (s *AnalysisService) StockSummary(ctx context.Context, symbol string) (StockSummary, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return StockSummary{}, errors.New("股票代码不能为空")
	}

	var stock models.Stock
	if err := s.db.WithContext(ctx).
		Preload("Metric").
		Where("symbol = ?", symbol).
		Order("market asc").
		First(&stock).Error; err != nil {
		return StockSummary{}, err
	}

	result := toStockResult(stock)
	return StockSummary{
		Symbol:           result.Symbol,
		Name:             result.Name,
		Market:           result.Market,
		Metrics:          result.Metrics,
		AIAnalyzed:       result.AIAnalyzed,
		DataAsOf:         result.DataAsOf,
		NAnalysisAt:      stock.Metric.NAnalysisAt,
		NAnalysisSummary: stock.Metric.NAnalysisSummary,
	}, nil
}

func (s *AnalysisService) CompanyProfile(ctx context.Context, symbol string) (CompanyProfile, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return CompanyProfile{}, errors.New("股票代码不能为空")
	}
	if IsAshareSymbol(symbol) {
		if s.ashare == nil || !s.ashare.Configured() {
			return CompanyProfile{Symbol: NormalizeAshareSymbol(symbol)}, errors.New("A 股数据源未启用")
		}
		profile, err := s.ashare.FetchCompanyProfile(ctx, symbol)
		if err != nil {
			return profile, err
		}
		return ensureProfileName(ctx, s.db, profile), nil
	}

	failures := []string{}
	if s.alphaVantage != nil && s.alphaVantage.Configured() {
		profile, err := s.alphaVantage.FetchCompanyProfile(ctx, symbol)
		if err == nil {
			return s.localizeCompanyProfile(ctx, ensureProfileName(ctx, s.db, profile)), nil
		}
		failures = append(failures, "Alpha Vantage: "+err.Error())
	} else {
		failures = append(failures, "Alpha Vantage: not configured")
	}

	if s.yfinance != nil && s.yfinance.Configured() {
		profile, err := s.yfinance.FetchCompanyProfile(ctx, symbol)
		if err == nil {
			return s.localizeCompanyProfile(ctx, ensureProfileName(ctx, s.db, profile)), nil
		}
		failures = append(failures, "yfinance: "+err.Error())
	} else {
		failures = append(failures, "yfinance: not configured")
	}

	if s.fmp != nil && s.fmp.Configured() {
		profile, err := s.fmp.FetchCompanyProfile(ctx, symbol)
		if err == nil {
			return s.localizeCompanyProfile(ctx, ensureProfileName(ctx, s.db, profile)), nil
		}
		failures = append(failures, "FMP: "+err.Error())
	} else {
		failures = append(failures, "FMP: not configured")
	}

	return CompanyProfile{}, errors.New(strings.Join(failures, "；"))
}

func (s *AnalysisService) KLine(ctx context.Context, symbol string, period string) (KLineData, error) {
	symbol = normalizeSymbol(symbol)
	period, err := normalizePeriod(period)
	if err != nil {
		return KLineData{}, err
	}

	result, err := s.fetchCandles(ctx, symbol, period)
	if err != nil {
		return KLineData{}, err
	}
	candles := trimCandles(normalizeCandles(result.candles), candleLimit(period))
	return KLineData{
		Symbol:   symbol,
		Period:   period,
		Source:   result.source,
		DataAsOf: lastCandleDate(candles),
		Candles:  candles,
	}, nil
}

func (s *AnalysisService) RS(ctx context.Context, symbol string, period string) (RSData, error) {
	symbol = normalizeSymbol(symbol)
	period, err := normalizePeriod(period)
	if err != nil {
		return RSData{}, err
	}

	stockResult, err := s.fetchCandles(ctx, symbol, period)
	if err != nil {
		return RSData{}, err
	}
	benchmarkResult, err := s.fetchCandles(ctx, ashareBenchmarkForSymbol(symbol), period)
	if err != nil {
		return RSData{}, err
	}
	stockCandles := trimCandles(normalizeCandles(stockResult.candles), candleLimit(period)+rsWindow(period))
	benchmarkCandles := trimCandles(normalizeCandles(benchmarkResult.candles), candleLimit(period)+rsWindow(period))
	line := buildRSLine(stockCandles, benchmarkCandles, period)
	if len(line) > candleLimit(period) {
		line = line[len(line)-candleLimit(period):]
	}

	source := stockResult.source
	if benchmarkResult.source != "" && benchmarkResult.source != source {
		source = strings.TrimSpace(source + " / " + benchmarkResult.source)
	}
	return RSData{
		Symbol:   symbol,
		Period:   period,
		Source:   source,
		DataAsOf: lastRSDate(line),
		RSLine:   line,
	}, nil
}

func (s *AnalysisService) EPSQuarterly(ctx context.Context, symbol string) (EPSData, error) {
	symbol = normalizeSymbol(symbol)
	if IsAshareSymbol(symbol) {
		canonical := NormalizeAshareSymbol(symbol)
		if s.ashare == nil || !s.ashare.Configured() {
			return EPSData{Symbol: canonical, Quarters: []EPSQuarter{}}, errors.New("A 股数据源未启用")
		}
		return s.ashare.FetchEPSQuarterly(ctx, canonical)
	}
	failures := []string{}

	if s.sec != nil && s.sec.Configured() {
		data, err := s.sec.FetchEPSQuarterly(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "SEC EDGAR: "+err.Error())
		} else {
			failures = append(failures, "SEC EDGAR: EPS data is empty")
		}
	} else {
		failures = append(failures, "SEC EDGAR: not configured")
	}

	if s.fmp != nil && s.fmp.Configured() {
		data, err := s.fetchFMPQuarterlyEPS(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "FMP: "+err.Error())
		} else {
			failures = append(failures, "FMP: EPS data is empty")
		}
	} else {
		failures = append(failures, "FMP: not configured")
	}

	if s.yfinance != nil && s.yfinance.Configured() {
		data, err := s.yfinance.FetchEPSQuarterly(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "yfinance: "+err.Error())
		} else {
			failures = append(failures, "yfinance: EPS data is empty")
		}
	} else {
		failures = append(failures, "yfinance: not configured")
	}

	return EPSData{Symbol: symbol, Quarters: []EPSQuarter{}}, fmt.Errorf("EPS data unavailable (%s)", strings.Join(failures, "; "))
}

func (s *AnalysisService) fetchFMPQuarterlyEPS(ctx context.Context, symbol string) (EPSData, error) {
	data := EPSData{Symbol: symbol, Source: "Financial Modeling Prep", Quarters: []EPSQuarter{}}
	rows, err := s.fmp.fetchIncomeStatements(ctx, symbol, "quarter", "16")
	if err != nil {
		return data, err
	}
	rows = sortStatementsDesc(rows)
	limit := min(12, len(rows))
	for i := 0; i < limit; i++ {
		row := rows[i]
		eps := round2(statementEPS(row))
		growth := eps
		sufficient := false
		if i+4 < len(rows) {
			base := statementEPS(rows[i+4])
			if base != 0 {
				growth = round2((statementEPS(row) - base) / math.Abs(base) * 100)
				sufficient = true
			}
		}
		data.Quarters = append(data.Quarters, EPSQuarter{
			Period:         statementQuarter(row),
			EPS:            eps,
			EPSYoYGrowth:   growth,
			DataSufficient: sufficient,
			ReportDate:     row.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data, nil
}

func (s *AnalysisService) RevenueAnnual(ctx context.Context, symbol string) (RevenueData, error) {
	symbol = normalizeSymbol(symbol)
	if IsAshareSymbol(symbol) {
		canonical := NormalizeAshareSymbol(symbol)
		if s.ashare == nil || !s.ashare.Configured() {
			return RevenueData{Symbol: canonical, Years: []RevenueYear{}}, errors.New("A 股数据源未启用")
		}
		return s.ashare.FetchRevenueAnnual(ctx, canonical)
	}
	failures := []string{}

	if s.sec != nil && s.sec.Configured() {
		data, err := s.sec.FetchRevenueAnnual(ctx, symbol)
		if err == nil && len(data.Years) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "SEC EDGAR: "+err.Error())
		} else {
			failures = append(failures, "SEC EDGAR: annual revenue data is empty")
		}
	} else {
		failures = append(failures, "SEC EDGAR: not configured")
	}

	if s.fmp != nil && s.fmp.Configured() {
		data, err := s.fetchFMPAnnualRevenue(ctx, symbol)
		if err == nil && len(data.Years) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "FMP: "+err.Error())
		} else {
			failures = append(failures, "FMP: annual revenue data is empty")
		}
	} else {
		failures = append(failures, "FMP: not configured")
	}

	if s.yfinance != nil && s.yfinance.Configured() {
		data, err := s.yfinance.FetchRevenueAnnual(ctx, symbol)
		if err == nil && len(data.Years) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "yfinance: "+err.Error())
		} else {
			failures = append(failures, "yfinance: annual revenue data is empty")
		}
	} else {
		failures = append(failures, "yfinance: not configured")
	}

	return RevenueData{Symbol: symbol, Years: []RevenueYear{}}, fmt.Errorf("annual revenue data unavailable (%s)", strings.Join(failures, "; "))
}

func (s *AnalysisService) fetchFMPAnnualRevenue(ctx context.Context, symbol string) (RevenueData, error) {
	data := RevenueData{Symbol: symbol, Source: "Financial Modeling Prep", Years: []RevenueYear{}}
	rows, err := s.fmp.fetchIncomeStatements(ctx, symbol, "", "6")
	if err != nil {
		return data, err
	}
	rows = sortStatementsDesc(rows)
	limit := min(5, len(rows))
	for i := 0; i < limit; i++ {
		row := rows[i]
		growth := 0.0
		sufficient := false
		if i+1 < len(rows) && rows[i+1].Revenue != 0 {
			growth = round2((row.Revenue - rows[i+1].Revenue) / math.Abs(rows[i+1].Revenue) * 100)
			sufficient = true
		}
		data.Years = append(data.Years, RevenueYear{
			Year:             statementYear(row),
			Revenue:          row.Revenue,
			RevenueYoYGrowth: growth,
			DataSufficient:   sufficient,
		})
	}
	if len(rows) > 0 {
		data.DataAsOf = rows[0].Date
	}
	return data, nil
}

func (s *AnalysisService) GrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	symbol = normalizeSymbol(symbol)
	if IsAshareSymbol(symbol) {
		canonical := NormalizeAshareSymbol(symbol)
		if s.ashare == nil || !s.ashare.Configured() {
			return GrossMarginData{Symbol: canonical, Quarters: []GrossMarginQuarter{}}, errors.New("A 股数据源未启用")
		}
		return s.ashare.FetchGrossMarginQuarterly(ctx, canonical)
	}
	failures := []string{}

	if s.sec != nil && s.sec.Configured() {
		data, err := s.sec.FetchGrossMarginQuarterly(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "SEC EDGAR: "+err.Error())
		} else {
			failures = append(failures, "SEC EDGAR: gross margin data is empty")
		}
	} else {
		failures = append(failures, "SEC EDGAR: not configured")
	}

	if s.fmp != nil && s.fmp.Configured() {
		data, err := s.fetchFMPGrossMarginQuarterly(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "FMP: "+err.Error())
		} else {
			failures = append(failures, "FMP: gross margin data is empty")
		}
	} else {
		failures = append(failures, "FMP: not configured")
	}

	if s.yfinance != nil && s.yfinance.Configured() {
		data, err := s.yfinance.FetchGrossMarginQuarterly(ctx, symbol)
		if err == nil && len(data.Quarters) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, "yfinance: "+err.Error())
		} else {
			failures = append(failures, "yfinance: gross margin data is empty")
		}
	} else {
		failures = append(failures, "yfinance: not configured")
	}

	return GrossMarginData{Symbol: symbol, Quarters: []GrossMarginQuarter{}}, fmt.Errorf("gross margin data unavailable (%s)", strings.Join(failures, "; "))
}

func (s *AnalysisService) SecondLayerData(ctx context.Context, symbol string) (SecondLayerData, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return SecondLayerData{}, errors.New("股票代码不能为空")
	}
	if IsAshareSymbol(symbol) {
		canonical := NormalizeAshareSymbol(symbol)
		if s.ashare == nil || !s.ashare.Configured() {
			return SecondLayerData{"symbol": canonical}, errors.New("A 股数据源未启用")
		}
		return s.ashare.FetchSecondLayerData(ctx, canonical)
	}

	result := SecondLayerData{"symbol": symbol}
	failures := []string{}
	if s.yfinance != nil && s.yfinance.Configured() {
		data, err := s.yfinance.FetchSecondLayerData(ctx, symbol)
		if err == nil && len(data) > 0 {
			for key, value := range data {
				result[key] = value
			}
		}
		if err != nil {
			failures = append(failures, "yfinance: "+err.Error())
		} else {
			failures = append(failures, "yfinance: second-layer data is empty")
		}
	} else {
		failures = append(failures, "yfinance: not configured")
	}

	if s.sec != nil && s.sec.Configured() {
		data, err := s.sec.FetchSecondLayerSignals(ctx, symbol)
		if err == nil && len(data) > 0 {
			result["sec"] = data
		} else if err != nil {
			failures = append(failures, "SEC EDGAR: "+err.Error())
		}
	} else {
		failures = append(failures, "SEC EDGAR: not configured")
	}

	if len(result) > 1 {
		return result, nil
	}
	return result, fmt.Errorf("second-layer data unavailable (%s)", strings.Join(failures, "; "))
}

func (s *AnalysisService) fetchFMPGrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	data := GrossMarginData{Symbol: symbol, Source: "Financial Modeling Prep", Quarters: []GrossMarginQuarter{}}
	rows, err := s.fmp.fetchIncomeStatements(ctx, symbol, "quarter", "12")
	if err != nil {
		return data, err
	}
	rows = sortStatementsDesc(rows)
	limit := min(12, len(rows))
	for i := 0; i < limit; i++ {
		row := rows[i]
		margin := row.GrossProfitRatio
		if margin == 0 && row.Revenue != 0 {
			margin = row.GrossProfit / row.Revenue
		}
		if math.Abs(margin) <= 1 {
			margin *= 100
		}
		data.Quarters = append(data.Quarters, GrossMarginQuarter{
			Period:      statementQuarter(row),
			GrossMargin: round2(margin),
			ReportDate:  row.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data, nil
}

func (s *AnalysisService) fetchCandles(ctx context.Context, symbol string, period string) (candleSourceResult, error) {
	if IsAshareSymbol(symbol) {
		if s.ashare == nil || !s.ashare.Configured() {
			return candleSourceResult{candles: []Candle{}}, errors.New("A 股数据源未启用")
		}
		candles, err := s.ashare.FetchCandles(ctx, symbol, period)
		if err != nil {
			return candleSourceResult{candles: []Candle{}}, err
		}
		return candleSourceResult{source: ashareMarketDataSource, candles: candles}, nil
	}
	var lastErr error
	if s.stooq != nil && s.stooq.Configured() {
		if candles, err := s.stooq.FetchCandles(ctx, symbol, period); err == nil && len(candles) > 0 {
			return candleSourceResult{source: "Stooq", candles: candles}, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if s.polygon != nil && s.polygon.Configured() {
		if candles, err := s.polygon.FetchCandles(ctx, symbol, period); err == nil && len(candles) > 0 {
			return candleSourceResult{source: "Polygon.io", candles: candles}, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if s.fmp != nil && s.fmp.Configured() {
		if candles, err := s.fmp.FetchCandles(ctx, symbol, period); err == nil && len(candles) > 0 {
			return candleSourceResult{source: "Financial Modeling Prep", candles: candles}, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if s.yahoo != nil && s.yahoo.Configured() {
		if candles, err := s.yahoo.FetchCandles(ctx, symbol, period); err == nil && len(candles) > 0 {
			return candleSourceResult{source: "Yahoo Finance", candles: candles}, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if s.yfinance != nil && s.yfinance.Configured() {
		if candles, err := s.yfinance.FetchCandles(ctx, symbol, period); err == nil && len(candles) > 0 {
			return candleSourceResult{source: "yfinance", candles: candles}, nil
		} else if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return candleSourceResult{candles: []Candle{}}, lastErr
	}
	return candleSourceResult{candles: []Candle{}}, nil
}

func (c *PolygonClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if !c.Configured() {
		return nil, errors.New("POLYGON_API_KEY 未配置")
	}
	timespan := map[string]string{
		PeriodDaily:   "day",
		PeriodWeekly:  "week",
		PeriodMonthly: "month",
	}[period]
	to := time.Now().UTC()
	from := candleStart(period, to)
	endpoint := fmt.Sprintf("/v2/aggs/ticker/%s/range/1/%s/%s/%s", url.PathEscape(symbol), timespan, from.Format("2006-01-02"), to.Format("2006-01-02"))
	query := url.Values{}
	query.Set("adjusted", "true")
	query.Set("sort", "asc")
	query.Set("limit", "50000")

	var payload struct {
		Results []struct {
			Open   float64 `json:"o"`
			High   float64 `json:"h"`
			Low    float64 `json:"l"`
			Close  float64 `json:"c"`
			Volume float64 `json:"v"`
			Time   int64   `json:"t"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, endpoint, query, &payload); err != nil {
		return nil, err
	}
	candles := make([]Candle, 0, len(payload.Results))
	for _, row := range payload.Results {
		if row.Time == 0 || row.Open == 0 || row.High == 0 || row.Low == 0 || row.Close == 0 {
			continue
		}
		candles = append(candles, Candle{
			Date:   time.UnixMilli(row.Time).UTC().Format("2006-01-02"),
			Open:   round2(row.Open),
			High:   round2(row.High),
			Low:    round2(row.Low),
			Close:  round2(row.Close),
			Volume: int64(math.Round(row.Volume)),
		})
	}
	return candles, nil
}

func (c *FMPClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if !c.Configured() {
		return nil, errors.New("FMP_API_KEY 未配置")
	}
	to := time.Now().UTC()
	from := candleStart(period, to)
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("from", from.Format("2006-01-02"))
	query.Set("to", to.Format("2006-01-02"))

	var rows []struct {
		Date   string  `json:"date"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}
	if err := c.getJSON(ctx, "/stable/historical-price-eod/full", query, &rows); err != nil || len(rows) == 0 {
		legacyQuery := url.Values{}
		legacyQuery.Set("from", from.Format("2006-01-02"))
		legacyQuery.Set("to", to.Format("2006-01-02"))
		var legacy struct {
			Historical []struct {
				Date   string  `json:"date"`
				Open   float64 `json:"open"`
				High   float64 `json:"high"`
				Low    float64 `json:"low"`
				Close  float64 `json:"close"`
				Volume float64 `json:"volume"`
			} `json:"historical"`
		}
		if legacyErr := c.getJSON(ctx, fmt.Sprintf("/api/v3/historical-price-full/%s", symbol), legacyQuery, &legacy); legacyErr != nil {
			if err != nil {
				return nil, err
			}
			return nil, legacyErr
		}
		rows = make([]struct {
			Date   string  `json:"date"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		}, 0, len(legacy.Historical))
		for _, row := range legacy.Historical {
			rows = append(rows, struct {
				Date   string  `json:"date"`
				Open   float64 `json:"open"`
				High   float64 `json:"high"`
				Low    float64 `json:"low"`
				Close  float64 `json:"close"`
				Volume float64 `json:"volume"`
			}(row))
		}
	}

	daily := make([]Candle, 0, len(rows))
	for _, row := range rows {
		if row.Date == "" || row.Open == 0 || row.High == 0 || row.Low == 0 || row.Close == 0 {
			continue
		}
		daily = append(daily, Candle{
			Date:   row.Date,
			Open:   round2(row.Open),
			High:   round2(row.High),
			Low:    round2(row.Low),
			Close:  round2(row.Close),
			Volume: int64(math.Round(row.Volume)),
		})
	}
	daily = normalizeCandles(daily)
	if period == PeriodDaily {
		return daily, nil
	}
	return aggregateCandles(daily, period), nil
}

func (c *YahooClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if !c.Configured() {
		return nil, errors.New("Yahoo Finance 兜底未启用")
	}
	rangeValue := map[string]string{
		PeriodDaily:   "2y",
		PeriodWeekly:  "3y",
		PeriodMonthly: "7y",
	}[period]
	interval := map[string]string{
		PeriodDaily:   "1d",
		PeriodWeekly:  "1wk",
		PeriodMonthly: "1mo",
	}[period]
	endpoint := fmt.Sprintf("%s/v8/finance/chart/%s?range=%s&interval=%s&events=history&includeAdjustedClose=true", c.baseURL, url.PathEscape(symbol), rangeValue, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AlphaLens/1.0")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Yahoo Finance 请求失败：HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*float64 `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Chart.Error != nil || len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, errors.New("Yahoo Finance 返回为空")
	}

	result := payload.Chart.Result[0]
	quote := result.Indicators.Quote[0]
	limit := min(len(result.Timestamp), min(len(quote.Open), min(len(quote.High), min(len(quote.Low), min(len(quote.Close), len(quote.Volume))))))
	candles := make([]Candle, 0, limit)
	for i := 0; i < limit; i++ {
		if quote.Open[i] == nil || quote.High[i] == nil || quote.Low[i] == nil || quote.Close[i] == nil || quote.Volume[i] == nil {
			continue
		}
		candles = append(candles, Candle{
			Date:   time.Unix(result.Timestamp[i], 0).UTC().Format("2006-01-02"),
			Open:   round2(*quote.Open[i]),
			High:   round2(*quote.High[i]),
			Low:    round2(*quote.Low[i]),
			Close:  round2(*quote.Close[i]),
			Volume: int64(math.Round(*quote.Volume[i])),
		})
	}
	return candles, nil
}

func (c *YFinanceClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if err := c.ensureAvailable(); err != nil {
		return nil, err
	}
	var payload struct {
		Error   string   `json:"error"`
		Candles []Candle `json:"candles"`
	}
	if err := c.runJSON(ctx, &payload, "candles", symbol, period); err != nil {
		return nil, err
	}
	if payload.Error != "" {
		return nil, errors.New(payload.Error)
	}
	return payload.Candles, nil
}

func normalizeSymbol(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizePeriod(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PeriodDaily:
		return PeriodDaily, nil
	case PeriodWeekly:
		return PeriodWeekly, nil
	case PeriodMonthly:
		return PeriodMonthly, nil
	default:
		return "", errors.New("不支持的周期")
	}
}

func candleLimit(period string) int {
	switch period {
	case PeriodWeekly:
		return 104
	case PeriodMonthly:
		return 60
	default:
		return 365
	}
}

func rsWindow(period string) int {
	switch period {
	case PeriodWeekly:
		return 52
	case PeriodMonthly:
		return 12
	default:
		return 252
	}
}

func candleStart(period string, to time.Time) time.Time {
	switch period {
	case PeriodWeekly:
		return to.AddDate(-3, 0, 0)
	case PeriodMonthly:
		return to.AddDate(-7, 0, 0)
	default:
		return to.AddDate(-2, 0, 0)
	}
}

func normalizeCandles(candles []Candle) []Candle {
	out := make([]Candle, 0, len(candles))
	seen := map[string]Candle{}
	for _, candle := range candles {
		if candle.Date == "" || candle.Open == 0 || candle.High == 0 || candle.Low == 0 || candle.Close == 0 {
			continue
		}
		seen[candle.Date] = candle
	}
	for _, candle := range seen {
		out = append(out, candle)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date
	})
	return out
}

func trimCandles(candles []Candle, limit int) []Candle {
	if len(candles) <= limit {
		return candles
	}
	return candles[len(candles)-limit:]
}

func aggregateCandles(daily []Candle, period string) []Candle {
	if period == PeriodDaily {
		return daily
	}
	grouped := make([]Candle, 0)
	currentKey := ""
	var current Candle
	for _, candle := range daily {
		date, err := time.Parse("2006-01-02", candle.Date)
		if err != nil {
			continue
		}
		key := date.Format("2006-01")
		if period == PeriodWeekly {
			year, week := date.ISOWeek()
			key = fmt.Sprintf("%04d-W%02d", year, week)
		}
		if currentKey != key {
			if currentKey != "" {
				grouped = append(grouped, current)
			}
			currentKey = key
			current = candle
			continue
		}
		current.Date = candle.Date
		current.Close = candle.Close
		current.Volume += candle.Volume
		if candle.High > current.High {
			current.High = candle.High
		}
		if candle.Low < current.Low {
			current.Low = candle.Low
		}
	}
	if currentKey != "" {
		grouped = append(grouped, current)
	}
	return grouped
}

func lastCandleDate(candles []Candle) string {
	if len(candles) == 0 {
		return ""
	}
	return candles[len(candles)-1].Date
}

func buildRSLine(stockCandles []Candle, benchmarkCandles []Candle, period string) []RSPoint {
	window := rsWindow(period)
	if len(stockCandles) <= window || len(benchmarkCandles) <= window {
		return []RSPoint{}
	}
	benchmark := map[string]int{}
	for index, candle := range benchmarkCandles {
		benchmark[candle.Date] = index
	}

	line := make([]RSPoint, 0, len(stockCandles)-window)
	for index := window; index < len(stockCandles); index++ {
		current := stockCandles[index]
		stockBase := stockCandles[index-window]
		benchmarkIndex, ok := benchmark[current.Date]
		if !ok || benchmarkIndex < window {
			continue
		}
		benchmarkCurrent := benchmarkCandles[benchmarkIndex]
		benchmarkBase := benchmarkCandles[benchmarkIndex-window]
		if stockBase.Close == 0 || benchmarkBase.Close == 0 {
			continue
		}
		stockReturn := (current.Close - stockBase.Close) / math.Abs(stockBase.Close) * 100
		benchmarkReturn := (benchmarkCurrent.Close - benchmarkBase.Close) / math.Abs(benchmarkBase.Close) * 100
		line = append(line, RSPoint{Date: current.Date, Value: round2(rsScore(stockReturn, benchmarkReturn))})
	}
	return line
}

func rsScore(stockReturn float64, benchmarkReturn float64) float64 {
	const (
		scoreRange   = 98
		neutralSlope = 0.8
	)

	relativeReturn := stockReturn - benchmarkReturn
	scale := scoreRange / (math.Pi * neutralSlope)
	value := 50 + (scoreRange/math.Pi)*math.Atan(relativeReturn/scale)
	return math.Max(1, math.Min(99, value))
}

func lastRSDate(points []RSPoint) string {
	if len(points) == 0 {
		return ""
	}
	return points[len(points)-1].Date
}

func sortStatementsDesc(rows []incomeStatement) []incomeStatement {
	out := append([]incomeStatement(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date > out[j].Date
	})
	return out
}

func statementYear(row incomeStatement) int {
	if row.CalendarYear != "" {
		if value, err := strconv.Atoi(row.CalendarYear); err == nil {
			return value
		}
	}
	if len(row.Date) >= 4 {
		if value, err := strconv.Atoi(row.Date[:4]); err == nil {
			return value
		}
	}
	return 0
}

func statementQuarter(row incomeStatement) string {
	year := statementYear(row)
	if row.Period != "" && row.Period != "FY" {
		return fmt.Sprintf("%d%s", year, strings.ToUpper(row.Period))
	}
	date, err := time.Parse("2006-01-02", row.Date)
	if err != nil {
		return fmt.Sprintf("%dQ?", year)
	}
	quarter := int((date.Month()-1)/3) + 1
	return fmt.Sprintf("%dQ%d", year, quarter)
}

func ensureProfileName(ctx context.Context, db *gorm.DB, profile CompanyProfile) CompanyProfile {
	profile.Symbol = normalizeSymbol(profile.Symbol)
	profile.Description = cleanDescription(profile.Description)
	profile.Sector = strings.TrimSpace(profile.Sector)
	profile.Industry = strings.TrimSpace(profile.Industry)
	profile.Website = strings.TrimSpace(profile.Website)
	if profile.Name != "" || db == nil || profile.Symbol == "" {
		return profile
	}

	var stock models.Stock
	if err := db.WithContext(ctx).Where("symbol = ?", profile.Symbol).Order("market asc").First(&stock).Error; err == nil {
		profile.Name = stock.Name
	}
	return profile
}

func (s *AnalysisService) localizeCompanyProfile(ctx context.Context, profile CompanyProfile) CompanyProfile {
	if s.translator == nil || !s.translator.Configured() {
		return profile
	}

	translated := false
	if value, err := s.translator.TranslateToChinese(ctx, profile.Description); err == nil && strings.TrimSpace(value) != "" {
		if value != profile.Description {
			translated = true
		}
		profile.Description = value
	}
	if value, err := s.translator.TranslateToChinese(ctx, profile.Sector); err == nil && strings.TrimSpace(value) != "" {
		if value != profile.Sector {
			translated = true
		}
		profile.Sector = value
	}
	if value, err := s.translator.TranslateToChinese(ctx, profile.Industry); err == nil && strings.TrimSpace(value) != "" {
		if value != profile.Industry {
			translated = true
		}
		profile.Industry = value
	}
	if translated && profile.Source != "" && !strings.Contains(profile.Source, "中文翻译") {
		profile.Source = profile.Source + " · 中文翻译"
	}
	return profile
}

func cleanDescription(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
