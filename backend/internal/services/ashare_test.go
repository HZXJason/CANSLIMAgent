package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

func TestNormalizeAshareSymbol(t *testing.T) {
	tests := map[string]string{
		"600519.SH": "600519.SH",
		"600519.ss": "600519.SH",
		"SH600519":  "600519.SH",
		"SH.600519": "600519.SH",
		"SH-600519": "600519.SH",
		"SZ:300750": "300750.SZ",
		"sz300750":  "300750.SZ",
		"BJ920000":  "920000.BJ",
		"600519":    "600519.SH",
		"300750":    "300750.SZ",
		"920000":    "920000.BJ",
		"430047":    "430047.BJ",
		"830799":    "830799.BJ",
		"000300.SH": "000300.SH",
		"AAPL":      "",
		"123456":    "",
		"123456.SZ": "",
		"900901.SH": "",
		"200002.SZ": "",
		"600519.SZ": "",
		"SZ.600519": "",
		"600519.US": "",
	}
	for input, want := range tests {
		if got := NormalizeAshareSymbol(input); got != want {
			t.Errorf("NormalizeAshareSymbol(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAshareTradeMarketUsesExactProviderValues(t *testing.T) {
	tests := map[string]string{
		models.MarketSSEMain:  "上交所主板",
		models.MarketSZSEMain: "深交所主板",
		models.MarketSTAR:     "上交所科创板",
		models.MarketChiNext:  "深交所创业板",
		models.MarketBSE:      "北京证券交易所",
	}
	for market, want := range tests {
		got, ok := ashareTradeMarket(market)
		if !ok || got != want {
			t.Errorf("ashareTradeMarket(%q) = %q, %v; want %q, true", market, got, ok, want)
		}
	}
}

func TestAshareUniverseRequestsEveryExactProviderMarket(t *testing.T) {
	tests := []struct {
		market      string
		tradeMarket string
		symbol      string
	}{
		{market: models.MarketSSEMain, tradeMarket: "上交所主板", symbol: "600519.SH"},
		{market: models.MarketSZSEMain, tradeMarket: "深交所主板", symbol: "000001.SZ"},
		{market: models.MarketSTAR, tradeMarket: "上交所科创板", symbol: "688001.SH"},
		{market: models.MarketChiNext, tradeMarket: "深交所创业板", symbol: "300750.SZ"},
		{market: models.MarketBSE, tradeMarket: "北京证券交易所", symbol: "920000.BJ"},
	}
	for _, test := range tests {
		t.Run(test.market, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantFilter := fmt.Sprintf(`(SECURITY_TYPE="A股")(LISTING_STATE="0")(TRADE_MARKET="%s")`, test.tradeMarket)
				if got := r.URL.Query().Get("filter"); got != wantFilter {
					t.Errorf("filter = %q, want %q", got, wantFilter)
				}
				code := strings.Split(test.symbol, ".")[0]
				writeAshareTestResponse(t, w, 1, []map[string]interface{}{{
					"SECUCODE": test.symbol, "SECURITY_CODE": code, "SECURITY_NAME_ABBR": "test",
					"TRADE_MARKET": test.tradeMarket, "LISTING_STATE": "0", "SECURITY_TYPE": "A股",
				}})
			}))
			defer server.Close()

			client := newTestAshareClient(server)
			candidates, err := client.FetchUniverse(context.Background(), test.market)
			if err != nil {
				t.Fatalf("FetchUniverse: %v", err)
			}
			if len(candidates) != 1 || candidates[0].Symbol != test.symbol {
				t.Fatalf("candidates = %#v", candidates)
			}
		})
	}
}

func TestAshareFetchUniversePagesAndKeepsCanonicalSymbols(t *testing.T) {
	var seenPages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("reportName"); got != ashareBasicInfoReport {
			t.Errorf("reportName = %q", got)
		}
		wantFilter := `(SECURITY_TYPE="A股")(LISTING_STATE="0")(TRADE_MARKET="上交所科创板")`
		if got := r.URL.Query().Get("filter"); got != wantFilter {
			t.Errorf("filter = %q, want %q", got, wantFilter)
		}
		page := r.URL.Query().Get("pageNumber")
		seenPages = append(seenPages, page)
		data := []map[string]interface{}{}
		if page == "1" {
			data = append(data, map[string]interface{}{
				"SECUCODE": "688001.SH", "SECURITY_CODE": "688001", "SECURITY_NAME_ABBR": "华兴源创",
				"TRADE_MARKET": "上交所科创板", "LISTING_STATE": "0", "SECURITY_TYPE": "A股",
			})
		} else {
			data = append(data,
				map[string]interface{}{
					"SECUCODE": "688001.SH", "SECURITY_CODE": "688001", "SECURITY_NAME_ABBR": "华兴源创",
					"TRADE_MARKET": "上交所科创板", "LISTING_STATE": "0", "SECURITY_TYPE": "A股",
				},
				map[string]interface{}{
					"SECUCODE": "688002.SH", "SECURITY_CODE": "688002", "SECURITY_NAME_ABBR": "睿创微纳",
					"TRADE_MARKET": "上交所科创板", "LISTING_STATE": "0", "SECURITY_TYPE": "A股",
				},
			)
		}
		writeAshareTestResponse(t, w, 2, data)
	}))
	defer server.Close()

	client := newTestAshareClient(server)
	candidates, err := client.FetchUniverse(context.Background(), models.MarketSTAR)
	if err != nil {
		t.Fatalf("FetchUniverse: %v", err)
	}
	if got, want := strings.Join(seenPages, ","), "1,2"; got != want {
		t.Fatalf("pages = %q, want %q", got, want)
	}
	if len(candidates) != 2 || candidates[0].Symbol != "688001.SH" || candidates[1].Symbol != "688002.SH" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestAshareTencentCandlesAndPriceSignal(t *testing.T) {
	baseDate := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows := make([][]string, 0, 61)
	for index := 0; index < 61; index++ {
		closeValue := 10 + float64(index)/10
		volume := 1000
		if index == 60 {
			volume = 2000
		}
		rows = append(rows, []string{
			baseDate.AddDate(0, 0, index).Format("2006-01-02"),
			fmt.Sprintf("%.2f", closeValue-0.1),
			fmt.Sprintf("%.2f", closeValue),
			fmt.Sprintf("%.2f", closeValue+0.2),
			fmt.Sprintf("%.2f", closeValue-0.2),
			strconv.Itoa(volume),
		})
	}

	var forbiddenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/forbidden") {
			forbiddenCalls.Add(1)
			http.Error(w, "US provider must not be called", http.StatusInternalServerError)
			return
		}
		param := r.URL.Query().Get("param")
		if param != "sh600519,day,,,320,qfq" && param != "sh600519,day,,,800,qfq" {
			t.Errorf("unexpected param = %q", param)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{"sh600519": map[string]interface{}{"qfqday": rows}},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := newTestAshareClient(server)
	client.tencentURL = server.URL
	signal, err := client.FetchPriceSignal(context.Background(), "600519.SH", time.Time{})
	if err != nil {
		t.Fatalf("FetchPriceSignal: %v", err)
	}
	if !signal.Sufficient || signal.VolumeRatio == nil || *signal.VolumeRatio != 200 {
		t.Fatalf("price signal = %#v", signal)
	}
	if signal.TwelveMonthReturn == nil || *signal.TwelveMonthReturn != 60 {
		t.Fatalf("return = %#v, want 60", signal.TwelveMonthReturn)
	}
	if signal.Source != ashareMarketDataSource || signal.DataAsOf == nil || signal.DataAsOf.Format("2006-01-02") != "2025-03-02" {
		t.Fatalf("source/as-of = %q / %#v", signal.Source, signal.DataAsOf)
	}

	stooq := NewStooqClient(true, time.Second)
	stooq.baseURL = server.URL + "/forbidden/stooq"
	stooq.client = server.Client()
	yahoo := NewYahooClient(true, time.Second)
	yahoo.baseURL = server.URL + "/forbidden/yahoo"
	yahoo.client = server.Client()
	fmp := NewFMPClient("test-key", time.Second)
	fmp.baseURL = server.URL + "/forbidden/fmp"
	fmp.client = server.Client()
	polygon := NewPolygonClient("test-key", time.Second)
	polygon.baseURL = server.URL + "/forbidden/polygon"
	polygon.client = server.Client()
	analysis := &AnalysisService{ashare: client, stooq: stooq, yahoo: yahoo, fmp: fmp, polygon: polygon}
	if _, err := analysis.fetchCandles(context.Background(), "600519.SH", PeriodDaily); err != nil {
		t.Fatalf("A-share analysis candle route: %v", err)
	}
	scanner := &ScannerService{ashare: client, yahoo: yahoo, stooq: stooq, fmp: fmp, polygon: polygon}
	if _, ok := scanner.fetchPriceSignal(context.Background(), "600519.SH", time.Time{}); !ok {
		t.Fatal("A-share scanner price route was unavailable")
	}
	if forbiddenCalls.Load() != 0 {
		t.Fatalf("A-share price routes made %d US-provider requests", forbiddenCalls.Load())
	}
}

func TestAshareFinancialAndProfileEndpoints(t *testing.T) {
	var forbiddenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/forbidden") {
			forbiddenCalls.Add(1)
			http.Error(w, "US provider must not be called", http.StatusInternalServerError)
			return
		}
		filter := r.URL.Query().Get("filter")
		if filter != `(SECUCODE="600519.SH")` {
			t.Errorf("filter = %q", filter)
		}
		switch r.URL.Query().Get("reportName") {
		case ashareBasicInfoReport:
			writeAshareTestResponse(t, w, 1, []map[string]interface{}{{
				"SECUCODE": "600519.SH", "SECURITY_NAME_ABBR": "贵州茅台", "ORG_NAME": "贵州茅台酒股份有限公司",
				"ORG_PROFIE": "公司主营贵州茅台酒系列产品。", "BOARD_NAME_1LEVEL": "主要消费", "BOARD_NAME_2LEVEL": "白酒",
				"ORG_WEB": "www.moutaichina.com", "TRADE_MARKET": "上交所主板", "CURRENCY": "人民币", "MAX_DATE": "2025-06-30",
			}})
		case ashareMainFinanceReport:
			writeAshareTestResponse(t, w, 1, []map[string]interface{}{
				financeMainRow("2024-12-31", 4, 400),
				financeMainRow("2023-12-31", 3, 300),
				financeMainRow("2022-12-31", 2, 200),
				financeMainRow("2021-12-31", 1, 100),
			})
		case ashareQuarterFinanceReport:
			writeAshareTestResponse(t, w, 1, []map[string]interface{}{
				financeQuarterRow("2025-06-30", "二季度", 2, 200, 40),
				financeQuarterRow("2025-03-31", "一季度", 0.8, 80, 39),
				financeQuarterRow("2024-12-31", "四季度", 3, 300, 38),
				financeQuarterRow("2024-09-30", "三季度", 2.4, 240, 37),
				financeQuarterRow("2024-06-30", "二季度", 1, 100, 36),
				financeQuarterRow("2024-03-31", "一季度", 0.4, 40, 35),
				financeQuarterRow("2023-12-31", "四季度", 2, 200, 34),
				financeQuarterRow("2023-09-30", "三季度", 1.5, 150, 33),
			})
		default:
			http.Error(w, "unexpected report", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := newTestAshareClient(server)

	profile, err := client.FetchCompanyProfile(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchCompanyProfile: %v", err)
	}
	if profile.Name != "贵州茅台" || profile.Sector != "主要消费" || profile.Source != ashareProfileSource || profile.Website != "https://www.moutaichina.com" {
		t.Fatalf("profile = %#v", profile)
	}

	quarterGrowth, quarterSufficient, annualGrowth, years, annualSufficient, dataAsOf, err := client.FetchEPSGrowth(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchEPSGrowth: %v", err)
	}
	if quarterGrowth == nil || *quarterGrowth != 100 || !quarterSufficient || len(annualGrowth) != 3 || years != 3 || !annualSufficient || dataAsOf.Format("2006-01-02") != "2025-06-30" {
		t.Fatalf("EPS growth = %v %v %v %d %v %s", quarterGrowth, quarterSufficient, annualGrowth, years, annualSufficient, dataAsOf)
	}

	revenue, err := client.FetchRevenueAnnual(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchRevenueAnnual: %v", err)
	}
	if len(revenue.Years) != 4 || revenue.Years[0].Year != 2024 || revenue.Years[0].RevenueYoYGrowth != 33.33 {
		t.Fatalf("revenue = %#v", revenue)
	}

	margin, err := client.FetchGrossMarginQuarterly(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchGrossMarginQuarterly: %v", err)
	}
	if len(margin.Quarters) != 8 || margin.Quarters[0].GrossMargin != 40 {
		t.Fatalf("margin = %#v", margin)
	}

	secondLayer, err := client.FetchSecondLayerData(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchSecondLayerData: %v", err)
	}
	if secondLayer["source"] != ashareProfileSource || secondLayer["data_as_of"] != "2025-06-30" {
		t.Fatalf("second layer = %#v", secondLayer)
	}

	alphaVantage := NewAlphaVantageClient("test-key", time.Second)
	alphaVantage.baseURL = server.URL + "/forbidden/alpha-vantage"
	alphaVantage.client = server.Client()
	sec := NewSECClient(true, "test@example.com", time.Second)
	sec.dataBaseURL = server.URL + "/forbidden/sec-data"
	sec.secBaseURL = server.URL + "/forbidden/sec"
	sec.client = server.Client()
	fmp := NewFMPClient("test-key", time.Second)
	fmp.baseURL = server.URL + "/forbidden/fmp"
	fmp.client = server.Client()
	analysis := &AnalysisService{ashare: client, alphaVantage: alphaVantage, sec: sec, fmp: fmp}
	if _, err := analysis.CompanyProfile(context.Background(), "600519.SH"); err != nil {
		t.Fatalf("A-share profile route: %v", err)
	}
	if _, err := analysis.EPSQuarterly(context.Background(), "600519.SH"); err != nil {
		t.Fatalf("A-share EPS route: %v", err)
	}
	if _, err := analysis.RevenueAnnual(context.Background(), "600519.SH"); err != nil {
		t.Fatalf("A-share revenue route: %v", err)
	}
	if _, err := analysis.GrossMarginQuarterly(context.Background(), "600519.SH"); err != nil {
		t.Fatalf("A-share gross-margin route: %v", err)
	}
	if _, err := analysis.SecondLayerData(context.Background(), "600519.SH"); err != nil {
		t.Fatalf("A-share second-layer route: %v", err)
	}
	if forbiddenCalls.Load() != 0 {
		t.Fatalf("A-share fundamental routes made %d US-provider requests", forbiddenCalls.Load())
	}
}

func newTestAshareClient(server *httptest.Server) *AshareClient {
	client := NewAshareClient(true, time.Second)
	client.client = server.Client()
	client.dataCenterURL = server.URL
	client.tencentURL = server.URL
	return client
}

func writeAshareTestResponse(t *testing.T, w http.ResponseWriter, pages int, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "ok",
		"result":  map[string]interface{}{"pages": pages, "count": 1, "data": data},
	}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func financeMainRow(date string, eps float64, revenue float64) map[string]interface{} {
	return map[string]interface{}{
		"SECUCODE": "600519.SH", "REPORT_DATE": date, "REPORT_TYPE": "年报",
		"EPSJB": eps, "TOTALOPERATEREVE": revenue,
	}
}

func financeQuarterRow(date string, season string, eps float64, revenue float64, grossMargin float64) map[string]interface{} {
	return map[string]interface{}{
		"SECUCODE": "600519.SH", "REPORT_DATE": date, "SEASON_LABEL": season,
		"EPSJB": eps, "TOTALOPERATEREVE": revenue, "GROSS_PROFIT_RATIO": grossMargin,
	}
}
