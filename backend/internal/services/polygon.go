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

type PolygonClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewPolygonClient(apiKey string, timeout time.Duration) *PolygonClient {
	return &PolygonClient{
		apiKey:  apiKey,
		baseURL: "https://api.polygon.io",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *PolygonClient) Configured() bool {
	return c != nil && c.apiKey != ""
}

type priceSignal struct {
	VolumeRatio       *float64
	TwelveMonthReturn *float64
	Sufficient        bool
	DataAsOf          *time.Time
}

func (c *PolygonClient) FetchPriceSignal(ctx context.Context, symbol string, asOf time.Time) (priceSignal, error) {
	if !c.Configured() {
		return priceSignal{}, errors.New("POLYGON_API_KEY 未配置")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	from := asOf.AddDate(-1, -1, 0).Format("2006-01-02")
	to := asOf.Format("2006-01-02")
	endpoint := fmt.Sprintf("/v2/aggs/ticker/%s/range/1/day/%s/%s", url.PathEscape(symbol), from, to)
	query := url.Values{}
	query.Set("adjusted", "true")
	query.Set("sort", "asc")
	query.Set("limit", "50000")

	var payload struct {
		Results []struct {
			Close  float64 `json:"c"`
			Volume float64 `json:"v"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, endpoint, query, &payload); err != nil {
		return priceSignal{}, err
	}
	if len(payload.Results) < 60 {
		return priceSignal{Sufficient: false}, nil
	}

	last := payload.Results[len(payload.Results)-1]
	windowStart := len(payload.Results) - 51
	volumeTotal := 0.0
	for _, row := range payload.Results[windowStart : len(payload.Results)-1] {
		volumeTotal += row.Volume
	}
	avgVolume := volumeTotal / 50
	if avgVolume == 0 || payload.Results[0].Close == 0 {
		return priceSignal{Sufficient: false}, nil
	}

	volumeRatio := round2(last.Volume / avgVolume * 100)
	twelveMonthReturn := round2((last.Close - payload.Results[0].Close) / math.Abs(payload.Results[0].Close) * 100)
	return priceSignal{
		VolumeRatio:       &volumeRatio,
		TwelveMonthReturn: &twelveMonthReturn,
		Sufficient:        true,
		DataAsOf:          &asOf,
	}, nil
}

func (c *PolygonClient) getJSON(ctx context.Context, path string, query url.Values, target interface{}) error {
	u := c.baseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Polygon 请求失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(target)
}
