package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"
)

type FMPClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type StockCandidate struct {
	Symbol string
	Name   string
}

func NewFMPClient(apiKey string, timeout time.Duration) *FMPClient {
	return &FMPClient{
		apiKey:  apiKey,
		baseURL: "https://financialmodelingprep.com",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *FMPClient) Configured() bool {
	return c != nil && c.apiKey != ""
}

func (c *FMPClient) FetchUniverse(ctx context.Context, market string) ([]StockCandidate, error) {
	if !c.Configured() {
		return nil, errors.New("FMP_API_KEY 未配置")
	}

	endpoints := []string{}
	switch market {
	case "SP500":
		endpoints = []string{"/stable/sp500-constituent", "/api/v3/sp500_constituent"}
	case "NASDAQ100":
		endpoints = []string{"/stable/nasdaq-constituent", "/api/v3/nasdaq_constituent"}
	default:
		return nil, fmt.Errorf("%s 暂无 FMP 成分股接口，使用本地种子数据", market)
	}

	var rows []struct {
		Symbol string `json:"symbol"`
		Name   string `json:"name"`
	}
	var lastErr error
	for _, endpoint := range endpoints {
		rows = nil
		if err := c.getJSON(ctx, endpoint, url.Values{}, &rows); err != nil {
			lastErr = err
			continue
		}
		if len(rows) > 0 {
			break
		}
	}

	result := make([]StockCandidate, 0, len(rows))
	for _, row := range rows {
		if row.Symbol == "" || row.Name == "" {
			continue
		}
		result = append(result, StockCandidate{Symbol: row.Symbol, Name: row.Name})
	}
	if len(result) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New("FMP 成分股返回为空")
	}
	return result, nil
}

type incomeStatement struct {
	Date             string  `json:"date"`
	CalendarYear     string  `json:"calendarYear"`
	Period           string  `json:"period"`
	Revenue          float64 `json:"revenue"`
	GrossProfit      float64 `json:"grossProfit"`
	GrossProfitRatio float64 `json:"grossProfitRatio"`
	EPS              float64 `json:"eps"`
	EPSDiluted       float64 `json:"epsDiluted"`
	EPSDilutedLegacy float64 `json:"epsdiluted"`
}

func (c *FMPClient) FetchEPSGrowth(ctx context.Context, symbol string) (quarterGrowth *float64, quarterSufficient bool, annualGrowth []float64, yearsAvailable int, annualSufficient bool, dataAsOf time.Time, err error) {
	if !c.Configured() {
		return nil, false, nil, 0, false, time.Time{}, errors.New("FMP_API_KEY 未配置")
	}

	quarters, err := c.fetchIncomeStatements(ctx, symbol, "quarter", "8")
	if err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}

	if len(quarters) > 0 {
		dataAsOf, _ = time.Parse("2006-01-02", quarters[0].Date)
	}

	qSufficient := len(quarters) >= 5
	if len(quarters) > 0 {
		latest := statementEPS(quarters[0])
		if latest != 0 {
			value := round2(latest)
			quarterGrowth = &value
		}
	}
	if qSufficient {
		latest := statementEPS(quarters[0])
		base := statementEPS(quarters[4])
		if base != 0 {
			value := round2((latest - base) / math.Abs(base) * 100)
			quarterGrowth = &value
		} else {
			qSufficient = false
		}
	}

	annual, err := c.fetchIncomeStatements(ctx, symbol, "", "5")
	if err != nil {
		return quarterGrowth, qSufficient && quarterGrowth != nil, nil, 0, false, dataAsOf, err
	}

	aSufficient := len(annual) >= 4
	growth := make([]float64, 0, 3)
	for i := 0; i < len(annual) && len(growth) < 3; i++ {
		current := statementEPS(annual[i])
		if i+1 >= len(annual) {
			aSufficient = false
			if current != 0 {
				growth = append(growth, round2(current))
			}
			continue
		}
		previous := statementEPS(annual[i+1])
		if previous == 0 {
			aSufficient = false
			if current != 0 {
				growth = append(growth, round2(current))
			}
			continue
		}
		growth = append(growth, round2((current-previous)/math.Abs(previous)*100))
	}
	if len(growth) < 3 {
		aSufficient = false
	}

	return quarterGrowth, qSufficient && quarterGrowth != nil, growth, len(growth), aSufficient && len(growth) >= 3, dataAsOf, nil
}

func (c *FMPClient) fetchIncomeStatements(ctx context.Context, symbol string, period string, limit string) ([]incomeStatement, error) {
	stableQuery := url.Values{}
	stableQuery.Set("symbol", symbol)
	if period != "" {
		stableQuery.Set("period", period)
	}
	stableQuery.Set("limit", limit)

	legacyQuery := url.Values{}
	if period != "" {
		legacyQuery.Set("period", period)
	}
	legacyQuery.Set("limit", limit)

	attempts := []struct {
		path  string
		query url.Values
	}{
		{path: "/stable/income-statement", query: stableQuery},
		{path: fmt.Sprintf("/api/v3/income-statement/%s", symbol), query: legacyQuery},
	}

	var rows []incomeStatement
	var lastErr error
	for _, attempt := range attempts {
		rows = nil
		if err := c.getJSON(ctx, attempt.path, attempt.query, &rows); err != nil {
			lastErr = err
			continue
		}
		if len(rows) > 0 {
			return rows, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("FMP 财报返回为空")
}

func statementEPS(row incomeStatement) float64 {
	return firstNonZero(row.EPSDiluted, row.EPSDilutedLegacy, row.EPS)
}

func (c *FMPClient) FetchPriceSignal(ctx context.Context, symbol string, asOf time.Time) (priceSignal, error) {
	if !c.Configured() {
		return priceSignal{}, errors.New("FMP_API_KEY 未配置")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	from := asOf.AddDate(-1, -1, 0)
	query := url.Values{}
	query.Set("symbol", symbol)
	query.Set("from", from.Format("2006-01-02"))
	query.Set("to", asOf.Format("2006-01-02"))

	var rows []struct {
		Date   string  `json:"date"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}
	if err := c.getJSON(ctx, "/stable/historical-price-eod/full", query, &rows); err != nil {
		return priceSignal{}, err
	}

	points := make([]pricePoint, 0, len(rows))
	for _, row := range rows {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil || date.Before(from) || date.After(asOf.AddDate(0, 0, 1)) || row.Close == 0 || row.Volume == 0 {
			continue
		}
		points = append(points, pricePoint{
			timestamp: date.Unix(),
			close:     row.Close,
			volume:    row.Volume,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].timestamp < points[j].timestamp
	})
	if len(points) < 60 {
		return priceSignal{Sufficient: false}, nil
	}

	first := points[0]
	last := points[len(points)-1]
	volumeWindow := points[max(0, len(points)-51) : len(points)-1]
	volumeTotal := 0.0
	for _, point := range volumeWindow {
		volumeTotal += point.volume
	}
	avgVolume := volumeTotal / float64(len(volumeWindow))
	if avgVolume == 0 || first.close == 0 {
		return priceSignal{Sufficient: false}, nil
	}

	volumeRatio := round2(last.volume / avgVolume * 100)
	twelveMonthReturn := round2((last.close - first.close) / math.Abs(first.close) * 100)
	dataAsOf := time.Unix(last.timestamp, 0).UTC()
	return priceSignal{
		VolumeRatio:       &volumeRatio,
		TwelveMonthReturn: &twelveMonthReturn,
		Sufficient:        true,
		DataAsOf:          &dataAsOf,
	}, nil
}

func (c *FMPClient) getJSON(ctx context.Context, path string, query url.Values, target interface{}) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("apikey", c.apiKey)
	u := c.baseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("FMP 请求失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func firstNonZero(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
