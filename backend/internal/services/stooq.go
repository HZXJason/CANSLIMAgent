package services

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StooqClient struct {
	enabled bool
	baseURL string
	client  *http.Client

	disabledMu sync.RWMutex
	disabled   bool
}

func NewStooqClient(enabled bool, timeout time.Duration) *StooqClient {
	return &StooqClient{
		enabled: enabled,
		baseURL: "https://stooq.com",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *StooqClient) Configured() bool {
	if c == nil || !c.enabled {
		return false
	}
	c.disabledMu.RLock()
	defer c.disabledMu.RUnlock()
	return !c.disabled
}

func (c *StooqClient) FetchPriceSignal(ctx context.Context, symbol string, asOf time.Time) (priceSignal, error) {
	if !c.Configured() {
		return priceSignal{}, errors.New("Stooq 数据源未启用")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	daily, err := c.fetchDailyCandles(ctx, symbol, asOf.AddDate(-1, -1, 0), asOf)
	if err != nil {
		return priceSignal{}, err
	}
	if len(daily) < 60 {
		return priceSignal{Sufficient: false}, nil
	}

	first := daily[0]
	last := daily[len(daily)-1]
	volumeWindow := daily[max(0, len(daily)-51) : len(daily)-1]
	volumeTotal := 0.0
	for _, candle := range volumeWindow {
		volumeTotal += float64(candle.Volume)
	}
	avgVolume := volumeTotal / float64(len(volumeWindow))
	if avgVolume == 0 || first.Close == 0 {
		return priceSignal{Sufficient: false}, nil
	}

	volumeRatio := round2(float64(last.Volume) / avgVolume * 100)
	twelveMonthReturn := round2((last.Close - first.Close) / math.Abs(first.Close) * 100)
	dataAsOf, _ := time.Parse("2006-01-02", last.Date)
	return priceSignal{
		VolumeRatio:       &volumeRatio,
		TwelveMonthReturn: &twelveMonthReturn,
		Sufficient:        true,
		DataAsOf:          &dataAsOf,
	}, nil
}

func (c *StooqClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if !c.Configured() {
		return nil, errors.New("Stooq 数据源未启用")
	}
	to := time.Now().UTC()
	from := candleStart(period, to)
	daily, err := c.fetchDailyCandles(ctx, symbol, from, to)
	if err != nil {
		return nil, err
	}
	if period == PeriodDaily {
		return daily, nil
	}
	return aggregateCandles(daily, period), nil
}

func (c *StooqClient) fetchDailyCandles(ctx context.Context, symbol string, from time.Time, to time.Time) ([]Candle, error) {
	query := url.Values{}
	query.Set("s", stooqSymbol(symbol))
	query.Set("i", "d")
	query.Set("d1", from.Format("20060102"))
	query.Set("d2", to.Format("20060102"))
	endpoint := c.baseURL + "/q/d/l/?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 CANSLIMAgent/1.0")
	req.Header.Set("Accept", "text/csv,*/*")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			c.disable()
		}
		return nil, fmt.Errorf("Stooq 请求失败：HTTP %d", resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if len(header) < 6 || !strings.EqualFold(strings.TrimSpace(header[0]), "Date") {
		c.disable()
		return nil, errors.New("Stooq 返回非 CSV 行情数据")
	}

	candles := []Candle{}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) < 6 {
			continue
		}
		open, ok := parseStooqFloat(row[1])
		if !ok {
			continue
		}
		high, ok := parseStooqFloat(row[2])
		if !ok {
			continue
		}
		low, ok := parseStooqFloat(row[3])
		if !ok {
			continue
		}
		closeValue, ok := parseStooqFloat(row[4])
		if !ok {
			continue
		}
		volume, _ := strconv.ParseInt(strings.TrimSpace(row[5]), 10, 64)
		candles = append(candles, Candle{
			Date:   strings.TrimSpace(row[0]),
			Open:   round2(open),
			High:   round2(high),
			Low:    round2(low),
			Close:  round2(closeValue),
			Volume: volume,
		})
	}
	candles = normalizeCandles(candles)
	if len(candles) == 0 {
		return nil, errors.New("Stooq 行情数据为空")
	}
	return candles, nil
}

func (c *StooqClient) disable() {
	c.disabledMu.Lock()
	defer c.disabledMu.Unlock()
	c.disabled = true
}

func parseStooqFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func stooqSymbol(symbol string) string {
	symbol = strings.ToLower(strings.TrimSpace(symbol))
	symbol = strings.NewReplacer(".", "-", "/", "-").Replace(symbol)
	if !strings.Contains(symbol, ".") {
		symbol += ".us"
	}
	return symbol
}
