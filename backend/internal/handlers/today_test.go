package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type blockingTodayCalendarProvider struct{}

func (blockingTodayCalendarProvider) Configured() bool { return true }

func (blockingTodayCalendarProvider) FetchWeekEvents(ctx context.Context, _ string, _, _ time.Time, _ []string) (services.CalendarFetchResult, error) {
	<-ctx.Done()
	return services.CalendarFetchResult{}, ctx.Err()
}
func (blockingTodayCalendarProvider) FetchMonthEvents(ctx context.Context, _ string, _, _ time.Time, _ []string) (services.CalendarFetchResult, error) {
	<-ctx.Done()
	return services.CalendarFetchResult{}, ctx.Err()
}

type capturingTodayCalendarProvider struct {
	symbols []string
}

func (*capturingTodayCalendarProvider) Configured() bool { return true }

func (provider *capturingTodayCalendarProvider) FetchWeekEvents(_ context.Context, _ string, _, _ time.Time, symbols []string) (services.CalendarFetchResult, error) {
	provider.symbols = append([]string(nil), symbols...)
	return services.CalendarFetchResult{Sources: []services.CalendarSource{{Name: "Yahoo Finance", URL: "https://finance.yahoo.com"}}}, nil
}
func (provider *capturingTodayCalendarProvider) FetchMonthEvents(_ context.Context, _ string, _, _ time.Time, symbols []string) (services.CalendarFetchResult, error) {
	provider.symbols = append([]string(nil), symbols...)
	return services.CalendarFetchResult{Sources: []services.CalendarSource{{Name: "Yahoo Finance", URL: "https://finance.yahoo.com"}}}, nil
}

func TestMarketSessionAndTaskPriority(t *testing.T) {
	zone, _ := time.LoadLocation("America/New_York")
	cases := []struct {
		hour    int
		weekend bool
		want    string
	}{{8, false, "premarket"}, {10, false, "intraday"}, {17, false, "postmarket"}, {10, true, "non_trading"}}
	for _, tc := range cases {
		day := 3
		if tc.weekend {
			day = 2
		}
		got := marketSession("US", time.Date(2026, 8, day, tc.hour, 0, 0, 0, zone))
		if got != tc.want {
			t.Fatalf("hour %d: got %s want %s", tc.hour, got, tc.want)
		}
	}
	if todayPriority("premarket", "candidate") >= todayPriority("premarket", "pending_review") {
		t.Fatal("premarket should prioritize candidates")
	}
	if todayPriority("postmarket", "pending_review") >= todayPriority("postmarket", "candidate") {
		t.Fatal("postmarket should prioritize review")
	}
	if !todayTradeMarketMatches("CN", "china_a") || todayTradeMarketMatches("CN", "us") {
		t.Fatal("today items must stay within the selected asset market")
	}
}

func TestTodayResponseIncludesCurrentMonthCalendar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.TradePlan{}, &models.PreTradeRiskReview{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.CandidateSnapshot{}); err != nil {
		t.Fatal(err)
	}
	validUntil := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	if err := database.Create(&models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionWatch, Status: models.TradePlanStatusWaiting, PlanDate: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), ValidUntil: &validUntil}).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewTodayHandler(database)
	handler.now = func() time.Time { return time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC) }
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/today?market=US", nil)
	handler.Get(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data struct {
			MonthCalendar *struct {
				MonthStart string `json:"month_start"`
				MonthEnd   string `json:"month_end"`
				Status     string `json:"status"`
				Events     []struct {
					Kind   string `json:"kind"`
					Symbol string `json:"symbol"`
				} `json:"events"`
			} `json:"month_calendar"`
			WeekCalendar *struct {
				WeekStart string `json:"week_start"`
				WeekEnd   string `json:"week_end"`
				Status    string `json:"status"`
				Events    []struct {
					Kind   string `json:"kind"`
					Symbol string `json:"symbol"`
				} `json:"events"`
			} `json:"week_calendar"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.WeekCalendar == nil {
		t.Fatalf("expected an explicit week_calendar contract, got %s", recorder.Body.String())
	}
	if response.Data.MonthCalendar == nil {
		t.Fatalf("expected an explicit month_calendar contract, got %s", recorder.Body.String())
	}
	month := response.Data.MonthCalendar
	if month.MonthStart != "2026-08-01" || month.MonthEnd != "2026-08-31" || month.Status != "local_only" {
		t.Fatalf("unexpected month metadata: %+v", month)
	}
	if len(month.Events) != 1 || month.Events[0].Kind != "plan_deadline" || month.Events[0].Symbol != "NVDA" {
		t.Fatalf("expected the local plan deadline in the month calendar: %+v", month.Events)
	}
	calendar := response.Data.WeekCalendar
	if calendar.WeekStart != "2026-08-03" || calendar.WeekEnd != "2026-08-07" || calendar.Status != "local_only" {
		t.Fatalf("unexpected week metadata: %+v", calendar)
	}
	if len(calendar.Events) != 1 || calendar.Events[0].Kind != "plan_deadline" || calendar.Events[0].Symbol != "NVDA" {
		t.Fatalf("expected the local plan deadline in the calendar: %+v", calendar.Events)
	}
}

func TestTodayCalendarProviderCannotDelayThePageBeyondItsBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.TradePlan{}, &models.PreTradeRiskReview{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.CandidateSnapshot{}); err != nil {
		t.Fatal(err)
	}

	handler := NewTodayHandlerWithCalendarProvider(database, blockingTodayCalendarProvider{})
	handler.calendarTimeout = 20 * time.Millisecond
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/today?market=US", nil)
	started := time.Now()
	handler.Get(requestContext)

	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("calendar degradation took too long: %s", elapsed)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected degraded calendar to preserve the page response, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestTodayCalendarDefaultBudgetCoversOfficialSourceFetches(t *testing.T) {
	handler := NewTodayHandlerWithCalendarProvider(nil, nil)
	if handler.calendarTimeout < 5*time.Second {
		t.Fatalf("calendar timeout = %s, want at least 5s for concurrent official sources", handler.calendarTimeout)
	}
}

func TestTodayCalendarIncludesSymbolsAwaitingTradeReview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.TradePlan{}, &models.PreTradeRiskReview{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.CandidateSnapshot{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.TradeJournalEntry{Symbol: "MU", Market: models.TradeMarketUS, ExecutedAt: time.Now(), Side: models.TradeSideBuy, Source: "manual"}).Error; err != nil {
		t.Fatal(err)
	}

	provider := &capturingTodayCalendarProvider{}
	handler := NewTodayHandlerWithCalendarProvider(database, provider)
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/today?market=US", nil)
	handler.Get(requestContext)

	if len(provider.symbols) != 1 || provider.symbols[0] != "MU" {
		t.Fatalf("expected the pending-review symbol to reach the research calendar, got %v", provider.symbols)
	}
}

func TestTodayCalendarIncludesOnlyStocksWithCompletedAIAnalysis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.TradePlan{}, &models.PreTradeRiskReview{}, &models.TradeJournalEntry{}, &models.TradeReview{}, &models.CandidateSnapshot{}); err != nil {
		t.Fatal(err)
	}
	analyzed := models.Stock{Symbol: "NVDA", Name: "NVIDIA", Market: models.MarketNASDAQ100}
	unanalyzed := models.Stock{Symbol: "AAPL", Name: "Apple", Market: models.MarketNASDAQ100}
	if err := database.Create(&analyzed).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&unanalyzed).Error; err != nil {
		t.Fatal(err)
	}
	analyzedAt := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	if err := database.Create(&models.StockMetric{StockID: analyzed.ID, AIAnalysisAt: &analyzedAt, DataAsOf: analyzedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&models.StockMetric{StockID: unanalyzed.ID, DataAsOf: analyzedAt}).Error; err != nil {
		t.Fatal(err)
	}

	provider := &capturingTodayCalendarProvider{}
	handler := NewTodayHandlerWithCalendarProvider(database, provider)
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/today?market=US", nil)
	handler.Get(requestContext)

	if len(provider.symbols) != 1 || provider.symbols[0] != "NVDA" {
		t.Fatalf("expected only the AI-analyzed symbol to reach the calendar, got %v", provider.symbols)
	}
}
