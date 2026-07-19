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
)

const (
	defaultAshareDataCenterURL = "https://datacenter-web.eastmoney.com/api/data/v1/get"
	defaultAshareTencentURL    = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"

	ashareBasicInfoReport      = "RPT_F10_ORG_BASICINFO"
	ashareMainFinanceReport    = "RPT_F10_FINANCE_MAINFINADATA"
	ashareQuarterFinanceReport = "RPT_F10_QTR_MAINFINADATA"
	ashareDataCenterPageSize   = 500
	ashareResponseBodyLimit    = 32 << 20
	ashareProfileSource        = "东方财富 F10"
	ashareMarketDataSource     = "腾讯证券"
	ashareBenchmarkSymbol      = "000300.SH"
)

// AshareClient owns the data route for mainland China A shares. Symbols at
// this boundary use Eastmoney's exchange suffixes (.SH/.SZ/.BJ); provider
// specific symbol conversion is kept inside the client.
type AshareClient struct {
	enabled       bool
	client        *http.Client
	dataCenterURL string
	tencentURL    string
}

func NewAshareClient(enabled bool, timeout time.Duration) *AshareClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &AshareClient{
		enabled:       enabled,
		client:        &http.Client{Timeout: timeout},
		dataCenterURL: defaultAshareDataCenterURL,
		tencentURL:    defaultAshareTencentURL,
	}
}

func (c *AshareClient) Configured() bool {
	return c != nil && c.enabled && c.client != nil
}

func IsAshareSymbol(value string) bool {
	return NormalizeAshareSymbol(value) != ""
}

// NormalizeAshareSymbol returns a canonical Eastmoney-style symbol. Numeric
// inference is intentionally limited to known A-share code ranges; callers
// should otherwise provide an exchange suffix to avoid cross-market ambiguity.
func NormalizeAshareSymbol(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	if len(value) >= 8 && (strings.HasPrefix(value, "SH") || strings.HasPrefix(value, "SZ") || strings.HasPrefix(value, "BJ")) {
		exchange := value[:2]
		code := value[2:]
		if len(code) == 7 && (code[0] == '.' || code[0] == '-' || code[0] == ':') {
			code = code[1:]
		}
		if isSixDigitCode(code) {
			value = code + "." + exchange
		}
	}
	if strings.HasSuffix(value, ".SS") {
		value = strings.TrimSuffix(value, ".SS") + ".SH"
	}
	if value == ashareBenchmarkSymbol {
		return value
	}

	if len(value) == 6 && isSixDigitCode(value) {
		exchange := ashareExchangeForCode(value)
		if exchange == "" {
			return ""
		}
		value += "." + exchange
	}

	code, exchange, ok := strings.Cut(value, ".")
	if !ok || !isSixDigitCode(code) {
		return ""
	}
	if ashareExchangeForCode(code) != exchange {
		return ""
	}
	return code + "." + exchange
}

func ashareExchangeForCode(code string) string {
	if !isSixDigitCode(code) {
		return ""
	}
	switch {
	case strings.HasPrefix(code, "600"), strings.HasPrefix(code, "601"), strings.HasPrefix(code, "603"), strings.HasPrefix(code, "605"), strings.HasPrefix(code, "688"), strings.HasPrefix(code, "689"):
		return "SH"
	case strings.HasPrefix(code, "000"), strings.HasPrefix(code, "001"), strings.HasPrefix(code, "002"), strings.HasPrefix(code, "003"), strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"):
		return "SZ"
	case strings.HasPrefix(code, "920"), strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		return "BJ"
	default:
		return ""
	}
}

func isSixDigitCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func ashareBenchmarkForSymbol(symbol string) string {
	if IsAshareSymbol(symbol) {
		return ashareBenchmarkSymbol
	}
	return "SPY"
}

type ashareBasicInfoRow struct {
	SecurityCode        string `json:"SECURITY_CODE"`
	SecurityCodeWithEx  string `json:"SECUCODE"`
	SecurityName        string `json:"SECURITY_NAME_ABBR"`
	OrganizationName    string `json:"ORG_NAME"`
	OrganizationProfile string `json:"ORG_PROFIE"`
	ShortProfile        string `json:"ORG_PROFILE"`
	MainBusiness        string `json:"MAIN_BUSINESS"`
	IndustryPath        string `json:"EM2016"`
	Sector              string `json:"BOARD_NAME_1LEVEL"`
	Industry            string `json:"BOARD_NAME_2LEVEL"`
	CSRCIndustry        string `json:"CSRC_INDUSTRY_NAME"`
	Website             string `json:"ORG_WEB"`
	TradeMarket         string `json:"TRADE_MARKET"`
	ListingState        string `json:"LISTING_STATE"`
	SecurityType        string `json:"SECURITY_TYPE"`
	Currency            string `json:"CURRENCY"`
	EmployeeCount       *int64 `json:"TOTAL_NUM"`
	OrganizationForm    string `json:"ORG_FORM"`
	ControlHolder       string `json:"CONTROL_HOLDER"`
	RealController      string `json:"REAL_CONTROLER"`
	MaxDate             string `json:"MAX_DATE"`
}

type ashareMainFinanceRow struct {
	SecurityCodeWithEx string   `json:"SECUCODE"`
	ReportDate         string   `json:"REPORT_DATE"`
	ReportType         string   `json:"REPORT_TYPE"`
	EPS                *float64 `json:"EPSJB"`
	EPSYoYGrowth       *float64 `json:"EPSJBTZ"`
	Revenue            *float64 `json:"TOTALOPERATEREVE"`
	RevenueYoYGrowth   *float64 `json:"TOTALOPERATEREVETZ"`
	GrossProfit        *float64 `json:"MLR"`
	GrossMargin        *float64 `json:"XSMLL"`
}

type ashareQuarterFinanceRow struct {
	SecurityCodeWithEx string   `json:"SECUCODE"`
	ReportDate         string   `json:"REPORT_DATE"`
	SeasonLabel        string   `json:"SEASON_LABEL"`
	EPS                *float64 `json:"EPSJB"`
	Revenue            *float64 `json:"TOTALOPERATEREVE"`
	RevenueYoYGrowth   *float64 `json:"TOTALOPERATEREVETZ"`
	GrossProfit        *float64 `json:"GROSS_PROFIT"`
	GrossMargin        *float64 `json:"GROSS_PROFIT_RATIO"`
}

type eastmoneyDataResponse[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Result  *struct {
		Pages int `json:"pages"`
		Count int `json:"count"`
		Data  []T `json:"data"`
	} `json:"result"`
}

type eastmoneyDataRequest struct {
	ReportName string
	Columns    string
	Filter     string
	SortColumn string
	SortType   string
	PageSize   int
}

func (c *AshareClient) FetchUniverse(ctx context.Context, market string) ([]StockCandidate, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}
	tradeMarket, ok := ashareTradeMarket(market)
	if !ok {
		return nil, fmt.Errorf("%s 不是受支持的 A 股板块", market)
	}

	filter := fmt.Sprintf(`(SECURITY_TYPE="A股")(LISTING_STATE="0")(TRADE_MARKET="%s")`, tradeMarket)
	rows, err := fetchEastmoneyRows[ashareBasicInfoRow](ctx, c, eastmoneyDataRequest{
		ReportName: ashareBasicInfoReport,
		Columns:    "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,TRADE_MARKET,LISTING_STATE,SECURITY_TYPE",
		Filter:     filter,
		SortColumn: "SECURITY_CODE",
		SortType:   "1",
		PageSize:   ashareDataCenterPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("获取 A 股股票池失败：%w", err)
	}

	candidates := make([]StockCandidate, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		symbol := NormalizeAshareSymbol(firstNonEmpty(row.SecurityCodeWithEx, row.SecurityCode))
		name := strings.TrimSpace(row.SecurityName)
		if symbol == "" || name == "" || row.SecurityType != "A股" || row.ListingState != "0" || row.TradeMarket != tradeMarket {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		candidates = append(candidates, StockCandidate{Symbol: symbol, Name: name})
	}
	if len(candidates) == 0 {
		return nil, errors.New("A 股股票池返回为空")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Symbol < candidates[j].Symbol })
	return candidates, nil
}

func ashareTradeMarket(market string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case models.MarketSSEMain:
		return "上交所主板", true
	case models.MarketSZSEMain:
		return "深交所主板", true
	case models.MarketSTAR:
		return "上交所科创板", true
	case models.MarketChiNext:
		return "深交所创业板", true
	case models.MarketBSE:
		return "北京证券交易所", true
	default:
		return "", false
	}
}

func fetchEastmoneyRows[T any](ctx context.Context, client *AshareClient, request eastmoneyDataRequest) ([]T, error) {
	pageSize := request.PageSize
	if pageSize <= 0 || pageSize > ashareDataCenterPageSize {
		pageSize = ashareDataCenterPageSize
	}
	rows := []T{}
	pages := 1
	for page := 1; page <= pages; page++ {
		query := url.Values{}
		query.Set("reportName", request.ReportName)
		query.Set("columns", request.Columns)
		query.Set("filter", request.Filter)
		query.Set("pageNumber", strconv.Itoa(page))
		query.Set("pageSize", strconv.Itoa(pageSize))
		if request.SortColumn != "" {
			query.Set("sortColumns", request.SortColumn)
			query.Set("sortTypes", request.SortType)
		}

		var payload eastmoneyDataResponse[T]
		if err := client.getJSON(ctx, client.dataCenterURL, query, &payload); err != nil {
			return nil, err
		}
		if !payload.Success || payload.Result == nil {
			message := strings.TrimSpace(payload.Message)
			if message == "" {
				message = "数据中心返回为空"
			}
			return nil, errors.New(message)
		}
		if payload.Result.Pages > pages {
			pages = payload.Result.Pages
		}
		rows = append(rows, payload.Result.Data...)
		if len(payload.Result.Data) == 0 {
			break
		}
	}
	return rows, nil
}

func (c *AshareClient) FetchPriceSignal(ctx context.Context, symbol string, _ time.Time) (priceSignal, error) {
	if err := c.ensureConfigured(); err != nil {
		return priceSignal{}, err
	}
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return priceSignal{}, errors.New("A 股股票代码格式无效")
	}
	candles, err := c.fetchTencentCandles(ctx, canonical, PeriodDaily, 320)
	if err != nil {
		return priceSignal{}, err
	}
	if len(candles) < 60 {
		return priceSignal{Sufficient: false}, nil
	}
	first := candles[max(0, len(candles)-253)]
	last := candles[len(candles)-1]
	if first.Close == 0 {
		return priceSignal{Sufficient: false}, nil
	}
	volumeWindow := candles[max(0, len(candles)-51) : len(candles)-1]
	if len(volumeWindow) == 0 {
		return priceSignal{Sufficient: false}, nil
	}
	volumeTotal := float64(0)
	for _, candle := range volumeWindow {
		volumeTotal += float64(candle.Volume)
	}
	averageVolume := volumeTotal / float64(len(volumeWindow))
	if averageVolume <= 0 {
		return priceSignal{Sufficient: false}, nil
	}
	volumeRatio := round2(float64(last.Volume) / averageVolume * 100)
	twelveMonthReturn := round2((last.Close - first.Close) / math.Abs(first.Close) * 100)
	dataAsOf, _ := time.Parse("2006-01-02", last.Date)
	return priceSignal{
		VolumeRatio:       &volumeRatio,
		TwelveMonthReturn: &twelveMonthReturn,
		Sufficient:        true,
		DataAsOf:          &dataAsOf,
		Source:            ashareMarketDataSource,
	}, nil
}

func (c *AshareClient) FetchCandles(ctx context.Context, symbol string, period string) ([]Candle, error) {
	if err := c.ensureConfigured(); err != nil {
		return nil, err
	}
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return nil, errors.New("A 股股票代码格式无效")
	}
	count := map[string]int{
		PeriodDaily:   800,
		PeriodWeekly:  260,
		PeriodMonthly: 120,
	}[period]
	if count == 0 {
		return nil, errors.New("不支持的周期")
	}
	return c.fetchTencentCandles(ctx, canonical, period, count)
}

type tencentKlineResponse struct {
	Code int                        `json:"code"`
	Msg  string                     `json:"msg"`
	Data map[string]json.RawMessage `json:"data"`
}

type tencentKlineSeries struct {
	Day      [][]json.RawMessage `json:"day"`
	Week     [][]json.RawMessage `json:"week"`
	Month    [][]json.RawMessage `json:"month"`
	QFQDay   [][]json.RawMessage `json:"qfqday"`
	QFQWeek  [][]json.RawMessage `json:"qfqweek"`
	QFQMonth [][]json.RawMessage `json:"qfqmonth"`
}

func (c *AshareClient) fetchTencentCandles(ctx context.Context, symbol string, period string, count int) ([]Candle, error) {
	tencentSymbol, ok := toTencentAshareSymbol(symbol)
	if !ok {
		return nil, errors.New("A 股股票代码格式无效")
	}
	interval := map[string]string{PeriodDaily: "day", PeriodWeekly: "week", PeriodMonthly: "month"}[period]
	query := url.Values{}
	query.Set("param", fmt.Sprintf("%s,%s,,,%d,qfq", tencentSymbol, interval, count))
	var payload tencentKlineResponse
	if err := c.getJSON(ctx, c.tencentURL, query, &payload); err != nil {
		return nil, fmt.Errorf("腾讯证券 K 线请求失败：%w", err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("腾讯证券 K 线返回异常：%s", strings.TrimSpace(payload.Msg))
	}
	raw, ok := payload.Data[tencentSymbol]
	if !ok {
		return nil, errors.New("腾讯证券 K 线返回为空")
	}
	var series tencentKlineSeries
	if err := json.Unmarshal(raw, &series); err != nil {
		return nil, fmt.Errorf("解析腾讯证券 K 线失败：%w", err)
	}
	rows := tencentRowsForPeriod(series, period)
	if len(rows) == 0 {
		return nil, errors.New("腾讯证券 K 线返回为空")
	}
	candles := make([]Candle, 0, len(rows))
	for _, row := range rows {
		candle, ok := parseTencentCandle(row)
		if ok {
			candles = append(candles, candle)
		}
	}
	candles = normalizeCandles(candles)
	if len(candles) == 0 {
		return nil, errors.New("腾讯证券 K 线没有可用数据")
	}
	return candles, nil
}

func tencentRowsForPeriod(series tencentKlineSeries, period string) [][]json.RawMessage {
	switch period {
	case PeriodDaily:
		if len(series.QFQDay) > 0 {
			return series.QFQDay
		}
		return series.Day
	case PeriodWeekly:
		if len(series.QFQWeek) > 0 {
			return series.QFQWeek
		}
		return series.Week
	case PeriodMonthly:
		if len(series.QFQMonth) > 0 {
			return series.QFQMonth
		}
		return series.Month
	default:
		return nil
	}
}

func parseTencentCandle(row []json.RawMessage) (Candle, bool) {
	if len(row) < 6 {
		return Candle{}, false
	}
	date := jsonText(row[0])
	openValue, openOK := jsonFloat(row[1])
	closeValue, closeOK := jsonFloat(row[2])
	highValue, highOK := jsonFloat(row[3])
	lowValue, lowOK := jsonFloat(row[4])
	volume, volumeOK := jsonFloat(row[5])
	if date == "" || !openOK || !closeOK || !highOK || !lowOK || !volumeOK || openValue == 0 || closeValue == 0 || highValue == 0 || lowValue == 0 {
		return Candle{}, false
	}
	return Candle{
		Date:   dateOnly(date),
		Open:   round2(openValue),
		High:   round2(highValue),
		Low:    round2(lowValue),
		Close:  round2(closeValue),
		Volume: int64(math.Round(volume * 100)), // Tencent reports A-share volume in lots.
	}, true
}

func toTencentAshareSymbol(symbol string) (string, bool) {
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return "", false
	}
	code, exchange, _ := strings.Cut(canonical, ".")
	prefix := map[string]string{"SH": "sh", "SZ": "sz", "BJ": "bj"}[exchange]
	return prefix + code, prefix != ""
}

func (c *AshareClient) FetchEPSGrowth(ctx context.Context, symbol string) (quarterGrowth *float64, quarterSufficient bool, annualGrowth []float64, yearsAvailable int, annualSufficient bool, dataAsOf time.Time, err error) {
	quarterly, err := c.FetchEPSQuarterly(ctx, symbol)
	if err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}
	if len(quarterly.Quarters) > 0 {
		latest := quarterly.Quarters[0]
		value := latest.EPS
		if latest.DataSufficient {
			value = latest.EPSYoYGrowth
		}
		quarterGrowth = &value
		quarterSufficient = latest.DataSufficient
		dataAsOf, _ = time.Parse("2006-01-02", latest.ReportDate)
	}

	rows, err := c.fetchMainFinance(ctx, symbol, 24)
	if err != nil {
		return quarterGrowth, quarterSufficient, nil, 0, false, dataAsOf, err
	}
	annualRows := make([]ashareMainFinanceRow, 0, 6)
	for _, row := range rows {
		if row.ReportType == "年报" && row.EPS != nil {
			annualRows = append(annualRows, row)
		}
	}
	annualSufficient = len(annualRows) >= 4
	for index := 0; index < min(3, len(annualRows)); index++ {
		value := *annualRows[index].EPS
		if index+1 < len(annualRows) && annualRows[index+1].EPS != nil && *annualRows[index+1].EPS != 0 {
			value = growthPercent(*annualRows[index].EPS, *annualRows[index+1].EPS)
		} else {
			annualSufficient = false
		}
		annualGrowth = append(annualGrowth, value)
	}
	if len(annualGrowth) < 3 {
		annualSufficient = false
	}
	return quarterGrowth, quarterGrowth != nil && quarterSufficient, annualGrowth, len(annualGrowth), annualSufficient, dataAsOf, nil
}

func (c *AshareClient) FetchEPSQuarterly(ctx context.Context, symbol string) (EPSData, error) {
	rows, err := c.fetchQuarterFinance(ctx, symbol, 16)
	canonical := NormalizeAshareSymbol(symbol)
	data := EPSData{Symbol: canonical, Source: ashareProfileSource, Quarters: []EPSQuarter{}}
	if err != nil {
		return data, err
	}
	for index, row := range rows[:min(12, len(rows))] {
		if row.EPS == nil {
			continue
		}
		growth := *row.EPS
		sufficient := false
		if index+4 < len(rows) && rows[index+4].EPS != nil && *rows[index+4].EPS != 0 {
			growth = growthPercent(*row.EPS, *rows[index+4].EPS)
			sufficient = true
		}
		reportDate := dateOnly(row.ReportDate)
		data.Quarters = append(data.Quarters, EPSQuarter{
			Period:         ashareQuarterLabel(reportDate, row.SeasonLabel),
			EPS:            round2(*row.EPS),
			EPSYoYGrowth:   round2(growth),
			DataSufficient: sufficient,
			ReportDate:     reportDate,
		})
	}
	if len(data.Quarters) == 0 {
		return data, errors.New("东方财富 A 股 EPS 数据为空")
	}
	data.DataAsOf = data.Quarters[0].ReportDate
	return data, nil
}

func (c *AshareClient) FetchRevenueAnnual(ctx context.Context, symbol string) (RevenueData, error) {
	rows, err := c.fetchMainFinance(ctx, symbol, 28)
	canonical := NormalizeAshareSymbol(symbol)
	data := RevenueData{Symbol: canonical, Source: ashareProfileSource, Years: []RevenueYear{}}
	if err != nil {
		return data, err
	}
	annualRows := make([]ashareMainFinanceRow, 0, 7)
	for _, row := range rows {
		if row.ReportType == "年报" && row.Revenue != nil {
			annualRows = append(annualRows, row)
		}
	}
	for index, row := range annualRows[:min(5, len(annualRows))] {
		year := ashareReportYear(row.ReportDate)
		if year == 0 {
			continue
		}
		growth := 0.0
		sufficient := false
		if index+1 < len(annualRows) && annualRows[index+1].Revenue != nil && *annualRows[index+1].Revenue != 0 {
			growth = growthPercent(*row.Revenue, *annualRows[index+1].Revenue)
			sufficient = true
		}
		data.Years = append(data.Years, RevenueYear{
			Year:             year,
			Revenue:          *row.Revenue,
			RevenueYoYGrowth: round2(growth),
			DataSufficient:   sufficient,
		})
	}
	if len(data.Years) == 0 {
		return data, errors.New("东方财富 A 股年度营收数据为空")
	}
	data.DataAsOf = dateOnly(annualRows[0].ReportDate)
	return data, nil
}

func (c *AshareClient) FetchGrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	rows, err := c.fetchQuarterFinance(ctx, symbol, 12)
	canonical := NormalizeAshareSymbol(symbol)
	data := GrossMarginData{Symbol: canonical, Source: ashareProfileSource, Quarters: []GrossMarginQuarter{}}
	if err != nil {
		return data, err
	}
	for _, row := range rows[:min(12, len(rows))] {
		margin := row.GrossMargin
		if margin == nil && row.GrossProfit != nil && row.Revenue != nil && *row.Revenue != 0 {
			value := *row.GrossProfit / *row.Revenue * 100
			margin = &value
		}
		if margin == nil {
			continue
		}
		reportDate := dateOnly(row.ReportDate)
		data.Quarters = append(data.Quarters, GrossMarginQuarter{
			Period:      ashareQuarterLabel(reportDate, row.SeasonLabel),
			GrossMargin: round2(*margin),
			ReportDate:  reportDate,
		})
	}
	if len(data.Quarters) == 0 {
		return data, errors.New("东方财富 A 股毛利率数据为空")
	}
	data.DataAsOf = data.Quarters[0].ReportDate
	return data, nil
}

func (c *AshareClient) FetchCompanyProfile(ctx context.Context, symbol string) (CompanyProfile, error) {
	row, err := c.fetchBasicInfo(ctx, symbol)
	canonical := NormalizeAshareSymbol(symbol)
	if err != nil {
		return CompanyProfile{Symbol: canonical}, err
	}
	description := firstNonEmpty(row.OrganizationProfile, row.ShortProfile, row.MainBusiness)
	if strings.TrimSpace(description) == "" {
		return CompanyProfile{Symbol: canonical}, errors.New("东方财富 A 股公司简介为空")
	}
	website := strings.TrimSpace(row.Website)
	if website != "" && !strings.HasPrefix(website, "http://") && !strings.HasPrefix(website, "https://") {
		website = "https://" + website
	}
	return CompanyProfile{
		Symbol:      canonical,
		Name:        firstNonEmpty(row.SecurityName, row.OrganizationName),
		Description: cleanDescription(description),
		Sector:      firstNonEmpty(row.Sector, firstIndustryPart(row.IndustryPath)),
		Industry:    firstNonEmpty(row.Industry, row.CSRCIndustry, row.IndustryPath),
		Website:     website,
		Source:      ashareProfileSource,
		UpdatedAt:   time.Now().UTC().Format("2006-01-02"),
	}, nil
}

func (c *AshareClient) FetchSecondLayerData(ctx context.Context, symbol string) (SecondLayerData, error) {
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return SecondLayerData{}, errors.New("A 股股票代码格式无效")
	}
	profile, err := c.fetchBasicInfo(ctx, canonical)
	if err != nil {
		return SecondLayerData{"symbol": canonical}, err
	}
	rows, financeErr := c.fetchQuarterFinance(ctx, canonical, 8)
	quarterly := make([]map[string]interface{}, 0, min(8, len(rows)))
	for _, row := range rows[:min(8, len(rows))] {
		quarterly = append(quarterly, map[string]interface{}{
			"period":             ashareQuarterLabel(dateOnly(row.ReportDate), row.SeasonLabel),
			"report_date":        dateOnly(row.ReportDate),
			"revenue":            row.Revenue,
			"revenue_yoy_growth": row.RevenueYoYGrowth,
			"gross_profit":       row.GrossProfit,
			"gross_margin":       row.GrossMargin,
			"diluted_eps":        row.EPS,
		})
	}
	result := SecondLayerData{
		"symbol":     canonical,
		"name":       firstNonEmpty(profile.SecurityName, profile.OrganizationName),
		"source":     ashareProfileSource,
		"data_as_of": dateOnly(firstNonEmpty(profile.MaxDate, firstQuarterDate(rows))),
		"company": map[string]interface{}{
			"sector":            firstNonEmpty(profile.Sector, firstIndustryPart(profile.IndustryPath)),
			"industry":          firstNonEmpty(profile.Industry, profile.CSRCIndustry, profile.IndustryPath),
			"currency":          profile.Currency,
			"trade_market":      profile.TradeMarket,
			"employee_count":    profile.EmployeeCount,
			"organization_form": profile.OrganizationForm,
			"control_holder":    profile.ControlHolder,
			"real_controller":   profile.RealController,
		},
		"financial_quality": map[string]interface{}{"quarterly": quarterly},
	}
	if financeErr != nil && len(quarterly) == 0 {
		return result, financeErr
	}
	return result, nil
}

func (c *AshareClient) fetchBasicInfo(ctx context.Context, symbol string) (ashareBasicInfoRow, error) {
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return ashareBasicInfoRow{}, errors.New("A 股股票代码格式无效")
	}
	rows, err := fetchEastmoneyRows[ashareBasicInfoRow](ctx, c, eastmoneyDataRequest{
		ReportName: ashareBasicInfoReport,
		Columns:    "SECUCODE,SECURITY_CODE,SECURITY_NAME_ABBR,ORG_NAME,ORG_PROFIE,ORG_PROFILE,MAIN_BUSINESS,EM2016,BOARD_NAME_1LEVEL,BOARD_NAME_2LEVEL,CSRC_INDUSTRY_NAME,ORG_WEB,TRADE_MARKET,LISTING_STATE,SECURITY_TYPE,CURRENCY,TOTAL_NUM,ORG_FORM,CONTROL_HOLDER,REAL_CONTROLER,MAX_DATE",
		Filter:     fmt.Sprintf(`(SECUCODE="%s")`, canonical),
		PageSize:   10,
	})
	if err != nil {
		return ashareBasicInfoRow{}, err
	}
	if len(rows) == 0 {
		return ashareBasicInfoRow{}, errors.New("东方财富 A 股公司资料为空")
	}
	return rows[0], nil
}

func (c *AshareClient) fetchMainFinance(ctx context.Context, symbol string, limit int) ([]ashareMainFinanceRow, error) {
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return nil, errors.New("A 股股票代码格式无效")
	}
	rows, err := fetchEastmoneyRows[ashareMainFinanceRow](ctx, c, eastmoneyDataRequest{
		ReportName: ashareMainFinanceReport,
		Columns:    "SECUCODE,REPORT_DATE,REPORT_TYPE,EPSJB,EPSJBTZ,TOTALOPERATEREVE,TOTALOPERATEREVETZ,MLR,XSMLL",
		Filter:     fmt.Sprintf(`(SECUCODE="%s")`, canonical),
		SortColumn: "REPORT_DATE",
		SortType:   "-1",
		PageSize:   max(limit, 12),
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return dateOnly(rows[i].ReportDate) > dateOnly(rows[j].ReportDate) })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return nil, errors.New("东方财富 A 股主要财务数据为空")
	}
	return rows, nil
}

func (c *AshareClient) fetchQuarterFinance(ctx context.Context, symbol string, limit int) ([]ashareQuarterFinanceRow, error) {
	canonical := NormalizeAshareSymbol(symbol)
	if canonical == "" {
		return nil, errors.New("A 股股票代码格式无效")
	}
	rows, err := fetchEastmoneyRows[ashareQuarterFinanceRow](ctx, c, eastmoneyDataRequest{
		ReportName: ashareQuarterFinanceReport,
		Columns:    "SECUCODE,REPORT_DATE,SEASON_LABEL,EPSJB,TOTALOPERATEREVE,TOTALOPERATEREVETZ,GROSS_PROFIT,GROSS_PROFIT_RATIO",
		Filter:     fmt.Sprintf(`(SECUCODE="%s")`, canonical),
		SortColumn: "REPORT_DATE",
		SortType:   "-1",
		PageSize:   max(limit, 12),
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return dateOnly(rows[i].ReportDate) > dateOnly(rows[j].ReportDate) })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return nil, errors.New("东方财富 A 股季度财务数据为空")
	}
	return rows, nil
}

func (c *AshareClient) getJSON(ctx context.Context, endpoint string, query url.Values, target interface{}) error {
	if err := c.ensureConfigured(); err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	values := parsed.Query()
	for key, entries := range query {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	parsed.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CANSLIMAgent/1.0)")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, ashareResponseBodyLimit)).Decode(target); err != nil {
		return err
	}
	return nil
}

func (c *AshareClient) ensureConfigured() error {
	if !c.Configured() {
		return errors.New("A 股数据源未启用")
	}
	return nil
}

func jsonText(value json.RawMessage) string {
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.Trim(strings.TrimSpace(string(value)), `"`)
}

func jsonFloat(value json.RawMessage) (float64, bool) {
	parsed, err := strconv.ParseFloat(jsonText(value), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func dateOnly(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func growthPercent(current, previous float64) float64 {
	if previous == 0 {
		return 0
	}
	return round2((current - previous) / math.Abs(previous) * 100)
}

func ashareQuarterLabel(reportDate, seasonLabel string) string {
	parsed, err := time.Parse("2006-01-02", dateOnly(reportDate))
	if err != nil {
		return strings.TrimSpace(seasonLabel)
	}
	quarter := int((parsed.Month()-1)/3) + 1
	return fmt.Sprintf("%dQ%d", parsed.Year(), quarter)
}

func ashareReportYear(reportDate string) int {
	parsed, err := time.Parse("2006-01-02", dateOnly(reportDate))
	if err != nil {
		return 0
	}
	return parsed.Year()
}

func firstIndustryPart(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func firstQuarterDate(rows []ashareQuarterFinanceRow) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].ReportDate
}
