package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SECClient struct {
	enabled     bool
	userAgent   string
	dataBaseURL string
	secBaseURL  string
	client      *http.Client
	minInterval time.Duration

	requestMu sync.Mutex
	lastReq   time.Time

	tickerMu  sync.Mutex
	tickerMap map[string]int
}

type secCompanyFacts struct {
	CIK        int                              `json:"cik"`
	EntityName string                           `json:"entityName"`
	Facts      map[string]map[string]secFactTag `json:"facts"`
}

type secFactTag struct {
	Units map[string][]secFactValue `json:"units"`
}

type secFactValue struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
	Frame string  `json:"frame"`
}

type secPeriodValue struct {
	Year    int
	Quarter int
	Date    string
	Filed   string
	Frame   string
	Value   float64
}

var (
	secEPSTags = []string{
		"EarningsPerShareDiluted",
		"EarningsPerShareBasic",
		"EarningsPerShareDilutedContinuingOperations",
		"EarningsPerShareBasicContinuingOperations",
	}
	secRevenueTags = []string{
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"Revenues",
		"SalesRevenueNet",
		"SalesRevenueGoodsNet",
	}
	secGrossProfitTags = []string{
		"GrossProfit",
	}
)

func NewSECClient(enabled bool, userAgent string, timeout time.Duration) *SECClient {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		userAgent = "CANSLIMAgent/1.0 (set SEC_USER_AGENT for contact)"
	}
	return &SECClient{
		enabled:     enabled,
		userAgent:   userAgent,
		dataBaseURL: "https://data.sec.gov",
		secBaseURL:  "https://www.sec.gov",
		client:      &http.Client{Timeout: timeout},
		minInterval: 120 * time.Millisecond,
	}
}

func (c *SECClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *SECClient) FetchEPSGrowth(ctx context.Context, symbol string) (quarterGrowth *float64, quarterSufficient bool, annualGrowth []float64, yearsAvailable int, annualSufficient bool, dataAsOf time.Time, err error) {
	if !c.Configured() {
		return nil, false, nil, 0, false, time.Time{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}

	epsData := buildSECEPSQuarterly(symbol, facts)
	if len(epsData.Quarters) > 0 {
		latest := epsData.Quarters[0]
		value := latest.EPSYoYGrowth
		if !latest.DataSufficient {
			value = latest.EPS
		}
		quarterGrowth = &value
		quarterSufficient = latest.DataSufficient
		if parsed, parseErr := time.Parse("2006-01-02", latest.ReportDate); parseErr == nil {
			dataAsOf = parsed
		}
	}

	annualGrowth, annualSufficient = buildSECAnnualEPSGrowth(facts)
	return quarterGrowth, quarterGrowth != nil && quarterSufficient, annualGrowth, len(annualGrowth), annualSufficient, dataAsOf, nil
}

func (c *SECClient) FetchEPSQuarterly(ctx context.Context, symbol string) (EPSData, error) {
	if !c.Configured() {
		return EPSData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return EPSData{}, err
	}
	data := buildSECEPSQuarterly(symbol, facts)
	if len(data.Quarters) == 0 {
		return data, errors.New("SEC EDGAR EPS data is empty")
	}
	return data, nil
}

func (c *SECClient) FetchRevenueAnnual(ctx context.Context, symbol string) (RevenueData, error) {
	if !c.Configured() {
		return RevenueData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return RevenueData{}, err
	}
	data := buildSECRevenueAnnual(symbol, facts)
	if len(data.Years) == 0 {
		return data, errors.New("SEC EDGAR annual revenue data is empty")
	}
	return data, nil
}

func (c *SECClient) FetchGrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	if !c.Configured() {
		return GrossMarginData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return GrossMarginData{}, err
	}
	data := buildSECGrossMarginQuarterly(symbol, facts)
	if len(data.Quarters) == 0 {
		return data, errors.New("SEC EDGAR gross margin data is empty")
	}
	return data, nil
}

func (c *SECClient) fetchCompanyFacts(ctx context.Context, symbol string) (secCompanyFacts, error) {
	cik, err := c.lookupCIK(ctx, symbol)
	if err != nil {
		return secCompanyFacts{}, err
	}

	var payload secCompanyFacts
	path := fmt.Sprintf("/api/xbrl/companyfacts/CIK%010d.json", cik)
	if err := c.getJSON(ctx, c.dataBaseURL+path, &payload); err != nil {
		return secCompanyFacts{}, err
	}
	if payload.CIK == 0 {
		payload.CIK = cik
	}
	return payload, nil
}

func (c *SECClient) lookupCIK(ctx context.Context, symbol string) (int, error) {
	symbol = normalizeSECTicker(symbol)
	if symbol == "" {
		return 0, errors.New("股票代码不能为空")
	}

	c.tickerMu.Lock()
	defer c.tickerMu.Unlock()

	if c.tickerMap == nil {
		var rows map[string]struct {
			CIKStr int    `json:"cik_str"`
			Ticker string `json:"ticker"`
			Title  string `json:"title"`
		}
		if err := c.getJSON(ctx, c.secBaseURL+"/files/company_tickers.json", &rows); err != nil {
			return 0, err
		}
		c.tickerMap = make(map[string]int, len(rows))
		for _, row := range rows {
			ticker := normalizeSECTicker(row.Ticker)
			if ticker != "" && row.CIKStr > 0 {
				c.tickerMap[ticker] = row.CIKStr
			}
		}
	}
	if cik, ok := c.tickerMap[symbol]; ok {
		return cik, nil
	}
	return 0, fmt.Errorf("SEC EDGAR 未找到 %s 对应的 CIK", symbol)
}

func (c *SECClient) getJSON(ctx context.Context, endpoint string, target interface{}) error {
	if err := c.waitForSECSlot(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SEC EDGAR 请求失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *SECClient) waitForSECSlot(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	if !c.lastReq.IsZero() {
		wait := c.minInterval - time.Since(c.lastReq)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.lastReq = time.Now()
	return nil
}

func buildSECEPSQuarterly(symbol string, facts secCompanyFacts) EPSData {
	data := EPSData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Quarters: []EPSQuarter{}}
	values := selectSECPeriodValues(facts, secEPSTags, []string{"USD/shares"}, true)
	limit := min(12, len(values))
	byPeriod := make(map[string]secPeriodValue, len(values))
	for _, value := range values {
		byPeriod[secQuarterKey(value.Year, value.Quarter)] = value
	}

	for i := 0; i < limit; i++ {
		value := values[i]
		growth := round2(value.Value)
		sufficient := false
		if previous, ok := byPeriod[secQuarterKey(value.Year-1, value.Quarter)]; ok && previous.Value != 0 {
			growth = round2((value.Value - previous.Value) / math.Abs(previous.Value) * 100)
			sufficient = true
		}
		data.Quarters = append(data.Quarters, EPSQuarter{
			Period:         fmt.Sprintf("%dQ%d", value.Year, value.Quarter),
			EPS:            round2(value.Value),
			EPSYoYGrowth:   growth,
			DataSufficient: sufficient,
			ReportDate:     value.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data
}

func buildSECRevenueAnnual(symbol string, facts secCompanyFacts) RevenueData {
	data := RevenueData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Years: []RevenueYear{}}
	values := selectSECPeriodValues(facts, secRevenueTags, []string{"USD"}, false)
	limit := min(5, len(values))
	byYear := make(map[int]secPeriodValue, len(values))
	for _, value := range values {
		byYear[value.Year] = value
	}

	for i := 0; i < limit; i++ {
		value := values[i]
		growth := 0.0
		sufficient := false
		if previous, ok := byYear[value.Year-1]; ok && previous.Value != 0 {
			growth = round2((value.Value - previous.Value) / math.Abs(previous.Value) * 100)
			sufficient = true
		}
		data.Years = append(data.Years, RevenueYear{
			Year:             value.Year,
			Revenue:          value.Value,
			RevenueYoYGrowth: growth,
			DataSufficient:   sufficient,
		})
	}
	if len(values) > 0 {
		data.DataAsOf = values[0].Date
	}
	return data
}

func buildSECGrossMarginQuarterly(symbol string, facts secCompanyFacts) GrossMarginData {
	data := GrossMarginData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Quarters: []GrossMarginQuarter{}}
	revenue := selectSECPeriodValues(facts, secRevenueTags, []string{"USD"}, true)
	grossProfit := selectSECPeriodValues(facts, secGrossProfitTags, []string{"USD"}, true)
	grossByPeriod := make(map[string]secPeriodValue, len(grossProfit))
	for _, value := range grossProfit {
		grossByPeriod[secQuarterKey(value.Year, value.Quarter)] = value
	}

	for _, revenueValue := range revenue {
		if len(data.Quarters) >= 12 {
			break
		}
		if revenueValue.Value == 0 {
			continue
		}
		grossValue, ok := grossByPeriod[secQuarterKey(revenueValue.Year, revenueValue.Quarter)]
		if !ok {
			continue
		}
		data.Quarters = append(data.Quarters, GrossMarginQuarter{
			Period:      fmt.Sprintf("%dQ%d", revenueValue.Year, revenueValue.Quarter),
			GrossMargin: round2(grossValue.Value / revenueValue.Value * 100),
			ReportDate:  revenueValue.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data
}

func buildSECAnnualEPSGrowth(facts secCompanyFacts) ([]float64, bool) {
	values := selectSECPeriodValues(facts, secEPSTags, []string{"USD/shares"}, false)
	byYear := make(map[int]secPeriodValue, len(values))
	for _, value := range values {
		byYear[value.Year] = value
	}

	growth := make([]float64, 0, 3)
	sufficient := len(values) >= 4
	for i := 0; i < len(values) && len(growth) < 3; i++ {
		current := values[i]
		previous, ok := byYear[current.Year-1]
		if !ok || previous.Value == 0 {
			sufficient = false
			growth = append(growth, round2(current.Value))
			continue
		}
		growth = append(growth, round2((current.Value-previous.Value)/math.Abs(previous.Value)*100))
	}
	if len(growth) < 3 {
		sufficient = false
	}
	return growth, sufficient && len(growth) >= 3
}

func selectSECPeriodValues(facts secCompanyFacts, tags []string, units []string, quarterly bool) []secPeriodValue {
	raw := secFactValues(facts, tags, units)
	selected := make(map[string]secPeriodValue)
	for _, row := range raw {
		value, ok := secValueFromFrame(row, quarterly)
		if !ok {
			continue
		}
		replaceSECPeriodValue(selected, value)
	}
	if len(selected) < secMinimumFrameCount(quarterly) {
		for _, row := range raw {
			value, ok := secValueFromDuration(row, quarterly)
			if !ok {
				continue
			}
			replaceSECPeriodValue(selected, value)
		}
	}

	values := make([]secPeriodValue, 0, len(selected))
	for _, value := range selected {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Date == values[j].Date {
			return values[i].Filed > values[j].Filed
		}
		return values[i].Date > values[j].Date
	})
	return values
}

func secFactValues(facts secCompanyFacts, tags []string, units []string) []secFactValue {
	usGAAP := facts.Facts["us-gaap"]
	out := []secFactValue{}
	for _, tagName := range tags {
		tag, ok := usGAAP[tagName]
		if !ok {
			continue
		}
		if rows, ok := selectSECUnitRows(tag, units); ok {
			out = append(out, rows...)
		}
	}
	return out
}

func selectSECUnitRows(tag secFactTag, units []string) ([]secFactValue, bool) {
	for _, unit := range units {
		if rows, ok := tag.Units[unit]; ok {
			return rows, true
		}
	}
	for unitName, rows := range tag.Units {
		for _, unit := range units {
			if strings.EqualFold(unitName, unit) || strings.Contains(strings.ToLower(unitName), strings.ToLower(unit)) {
				return rows, true
			}
		}
	}
	for _, rows := range tag.Units {
		return rows, true
	}
	return nil, false
}

func secValueFromFrame(row secFactValue, quarterly bool) (secPeriodValue, bool) {
	if !secAcceptedForm(row.Form) || row.End == "" || row.Frame == "" {
		return secPeriodValue{}, false
	}
	year, quarter, ok := parseSECFrame(row.Frame, quarterly)
	if !ok {
		return secPeriodValue{}, false
	}
	return secPeriodValue{
		Year:    year,
		Quarter: quarter,
		Date:    row.End,
		Filed:   row.Filed,
		Frame:   row.Frame,
		Value:   row.Val,
	}, true
}

func secValueFromDuration(row secFactValue, quarterly bool) (secPeriodValue, bool) {
	if !secAcceptedForm(row.Form) || row.Start == "" || row.End == "" {
		return secPeriodValue{}, false
	}
	start, err := time.Parse("2006-01-02", row.Start)
	if err != nil {
		return secPeriodValue{}, false
	}
	end, err := time.Parse("2006-01-02", row.End)
	if err != nil {
		return secPeriodValue{}, false
	}
	days := int(end.Sub(start).Hours()/24) + 1
	year := row.FY
	if year == 0 {
		year = end.Year()
	}
	quarter := 0
	if quarterly {
		if days < 60 || days > 120 {
			return secPeriodValue{}, false
		}
		quarter = int((end.Month()-1)/3) + 1
	} else {
		if days < 300 || days > 380 {
			return secPeriodValue{}, false
		}
	}
	return secPeriodValue{
		Year:    year,
		Quarter: quarter,
		Date:    row.End,
		Filed:   row.Filed,
		Value:   row.Val,
	}, true
}

func replaceSECPeriodValue(values map[string]secPeriodValue, next secPeriodValue) {
	key := strconv.Itoa(next.Year)
	if next.Quarter > 0 {
		key = secQuarterKey(next.Year, next.Quarter)
	}
	current, ok := values[key]
	if !ok || next.Filed > current.Filed || (next.Filed == current.Filed && next.Date > current.Date) {
		values[key] = next
	}
}

func parseSECFrame(frame string, quarterly bool) (int, int, bool) {
	frame = strings.ToUpper(strings.TrimSpace(frame))
	if !strings.HasPrefix(frame, "CY") || len(frame) < 6 {
		return 0, 0, false
	}
	year, err := strconv.Atoi(frame[2:6])
	if err != nil {
		return 0, 0, false
	}
	if quarterly {
		if len(frame) != 8 || frame[6] != 'Q' {
			return 0, 0, false
		}
		quarter := int(frame[7] - '0')
		if quarter < 1 || quarter > 4 {
			return 0, 0, false
		}
		return year, quarter, true
	}
	if len(frame) == 6 {
		return year, 0, true
	}
	return 0, 0, false
}

func secAcceptedForm(form string) bool {
	switch strings.ToUpper(strings.TrimSpace(form)) {
	case "10-K", "10-K/A", "10-Q", "10-Q/A", "20-F", "20-F/A", "40-F", "40-F/A":
		return true
	default:
		return false
	}
}

func secMinimumFrameCount(quarterly bool) int {
	if quarterly {
		return 5
	}
	return 4
}

func secQuarterKey(year int, quarter int) string {
	return fmt.Sprintf("%04dQ%d", year, quarter)
}

func normalizeSECTicker(value string) string {
	return strings.ToUpper(strings.NewReplacer(".", "-", "/", "-").Replace(strings.TrimSpace(value)))
}
