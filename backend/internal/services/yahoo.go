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
	"time"
)

type YahooClient struct {
	enabled bool
	baseURL string
	client  *http.Client
}

func NewYahooClient(enabled bool, timeout time.Duration) *YahooClient {
	return &YahooClient{
		enabled: enabled,
		baseURL: "https://query1.finance.yahoo.com",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *YahooClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *YahooClient) FetchPriceSignal(ctx context.Context, symbol string, _ time.Time) (priceSignal, error) {
	if !c.Configured() {
		return priceSignal{}, errors.New("Yahoo Finance 兜底未启用")
	}

	endpoint := fmt.Sprintf("%s/v8/finance/chart/%s?range=13mo&interval=1d&events=history&includeAdjustedClose=true", c.baseURL, url.PathEscape(symbol))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return priceSignal{}, err
	}
	req.Header.Set("User-Agent", "AlphaLens/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return priceSignal{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return priceSignal{}, fmt.Errorf("Yahoo Finance 请求失败：HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close  []*float64 `json:"close"`
						Volume []*float64 `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(&payload); err != nil {
		return priceSignal{}, err
	}
	if payload.Chart.Error != nil || len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		return priceSignal{}, errors.New("Yahoo Finance 返回为空")
	}

	result := payload.Chart.Result[0]
	quote := result.Indicators.Quote[0]
	points := normalizePricePoints(result.Timestamp, quote.Close, quote.Volume)
	if len(points) < 60 {
		return priceSignal{Sufficient: false}, nil
	}

	first := points[0]
	last := points[len(points)-1]
	if first.close == 0 {
		return priceSignal{Sufficient: false}, nil
	}

	volumeWindow := points[max(0, len(points)-51) : len(points)-1]
	volumeTotal := 0.0
	for _, point := range volumeWindow {
		volumeTotal += point.volume
	}
	avgVolume := volumeTotal / float64(len(volumeWindow))
	if avgVolume == 0 {
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

type pricePoint struct {
	timestamp int64
	close     float64
	volume    float64
}

func normalizePricePoints(timestamps []int64, closes []*float64, volumes []*float64) []pricePoint {
	limit := min(len(timestamps), min(len(closes), len(volumes)))
	points := make([]pricePoint, 0, limit)
	for i := 0; i < limit; i++ {
		if closes[i] == nil || volumes[i] == nil {
			continue
		}
		points = append(points, pricePoint{
			timestamp: timestamps[i],
			close:     *closes[i],
			volume:    *volumes[i],
		})
	}
	return points
}
