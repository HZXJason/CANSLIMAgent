package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type InstitutionalSignal struct {
	Delta      *int
	Sufficient bool
	DataAsOf   *time.Time
}

type InstitutionalClient struct {
	enabled bool
	baseURL string
	client  *http.Client
}

type nasdaqInstitutionalPayload struct {
	Data *struct {
		ActivePositions     nasdaqInstitutionalPositionGroup `json:"activePositions"`
		NewSoldOutPositions nasdaqInstitutionalPositionGroup `json:"newSoldOutPositions"`
	} `json:"data"`
	Status struct {
		RCode int `json:"rCode"`
	} `json:"status"`
}

type nasdaqInstitutionalPositionGroup struct {
	AsOf *string                          `json:"asOf"`
	Rows []nasdaqInstitutionalPositionRow `json:"rows"`
}

type nasdaqInstitutionalPositionRow struct {
	Positions string `json:"positions"`
	Holders   string `json:"holders"`
	Shares    string `json:"shares"`
}

func NewInstitutionalClient(enabled bool, timeout time.Duration) *InstitutionalClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &InstitutionalClient{
		enabled: enabled,
		baseURL: "https://api.nasdaq.com/api/company",
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *InstitutionalClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *InstitutionalClient) FetchInstitutionalSignal(ctx context.Context, symbol string) (InstitutionalSignal, error) {
	if !c.Configured() {
		return InstitutionalSignal{Sufficient: false}, errors.New("机构持仓数据源未启用")
	}

	symbol = normalizeNasdaqInstitutionalSymbol(symbol)
	if symbol == "" {
		return InstitutionalSignal{Sufficient: false}, errors.New("股票代码不能为空")
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/" + url.PathEscape(symbol) + "/institutional-holdings?limit=0&offset=0&type=TOTAL"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return InstitutionalSignal{Sufficient: false}, err
	}
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CANSLIMAgent/1.0)")

	resp, err := c.client.Do(req)
	if err != nil {
		return InstitutionalSignal{Sufficient: false}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return InstitutionalSignal{Sufficient: false}, fmt.Errorf("Nasdaq 机构持仓请求失败：HTTP %d", resp.StatusCode)
	}

	var payload nasdaqInstitutionalPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return InstitutionalSignal{Sufficient: false}, err
	}
	if payload.Data == nil {
		return InstitutionalSignal{Sufficient: false}, errors.New("Nasdaq 机构持仓返回为空")
	}

	signal, ok := buildInstitutionalSignal(payload.Data.ActivePositions, payload.Data.NewSoldOutPositions)
	if !ok {
		return InstitutionalSignal{Sufficient: false}, errors.New("Nasdaq 机构持仓缺少增减持字段")
	}
	return signal, nil
}

func buildInstitutionalSignal(active nasdaqInstitutionalPositionGroup, newSold nasdaqInstitutionalPositionGroup) (InstitutionalSignal, bool) {
	counts := map[string]int{}
	for _, row := range append(active.Rows, newSold.Rows...) {
		key := normalizeInstitutionalPosition(row.Positions)
		if key == "" || key == "totalinstitutionalshares" {
			continue
		}
		holders, ok := parseInstitutionalCount(row.Holders)
		if !ok {
			continue
		}
		counts[key] = holders
	}

	positive, positiveOK := sumInstitutionalCounts(counts, "increasedpositions", "newpositions")
	negative, negativeOK := sumInstitutionalCounts(counts, "decreasedpositions", "soldoutpositions")
	if !positiveOK && !negativeOK {
		return InstitutionalSignal{}, false
	}

	delta := positive - negative
	signal := InstitutionalSignal{Delta: &delta, Sufficient: true}
	if dataAsOf := parseInstitutionalAsOf(active.AsOf, newSold.AsOf); dataAsOf != nil {
		signal.DataAsOf = dataAsOf
	}
	return signal, true
}

func sumInstitutionalCounts(counts map[string]int, keys ...string) (int, bool) {
	sum := 0
	found := false
	for _, key := range keys {
		value, ok := counts[key]
		if !ok {
			continue
		}
		sum += value
		found = true
	}
	return sum, found
}

func parseInstitutionalCount(value string) (int, bool) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" || cleaned == "--" || strings.EqualFold(cleaned, "N/A") {
		return 0, false
	}
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = strings.ReplaceAll(cleaned, "%", "")
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseInstitutionalAsOf(values ...*string) *time.Time {
	layouts := []string{"01/02/2006", "2006-01-02", "Jan 2, 2006", "January 2, 2006"}
	for _, value := range values {
		if value == nil {
			continue
		}
		raw := strings.TrimSpace(*value)
		if raw == "" {
			continue
		}
		for _, layout := range layouts {
			parsed, err := time.Parse(layout, raw)
			if err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func normalizeInstitutionalPosition(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", "/", "", ".", "")
	return replacer.Replace(value)
}

func normalizeNasdaqInstitutionalSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", ".")
	return value
}
