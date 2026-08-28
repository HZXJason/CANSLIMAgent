package handlers

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TodayHandler struct {
	db              *gorm.DB
	calendar        todayCalendarProvider
	calendarTimeout time.Duration
	now             func() time.Time
}

type todayCalendarProvider interface {
	Configured() bool
	FetchWeekEvents(context.Context, string, time.Time, time.Time, []string) (services.CalendarFetchResult, error)
	FetchMonthEvents(context.Context, string, time.Time, time.Time, []string) (services.CalendarFetchResult, error)
}

type todayItem struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Href     string `json:"href"`
	Priority int    `json:"priority"`
}

type todayResponse struct {
	Market         string             `json:"market"`
	Timezone       string             `json:"timezone"`
	LocalDate      string             `json:"local_date"`
	Session        string             `json:"session"`
	CalendarStatus string             `json:"calendar_status"`
	DataAsOf       *time.Time         `json:"data_as_of"`
	WeekCalendar   todayWeekCalendar  `json:"week_calendar"`
	MonthCalendar  todayMonthCalendar `json:"month_calendar"`
	Items          []todayItem        `json:"items"`
}

type todayCalendarEvent struct {
	ID           string  `json:"id"`
	Date         string  `json:"date"`
	Time         *string `json:"time"`
	Kind         string  `json:"kind"`
	Title        string  `json:"title"`
	Subtitle     string  `json:"subtitle"`
	Symbol       *string `json:"symbol"`
	Importance   string  `json:"importance"`
	Source       string  `json:"source"`
	Href         *string `json:"href"`
	Result       *string `json:"result"`
	MarketImpact *string `json:"market_impact"`
	ImpactReason *string `json:"impact_reason"`
	LiveURL      *string `json:"live_url"`
}

type todayWeekCalendar struct {
	WeekStart string                    `json:"week_start"`
	WeekEnd   string                    `json:"week_end"`
	Status    string                    `json:"status"`
	Sources   []services.CalendarSource `json:"sources"`
	FetchedAt *time.Time                `json:"fetched_at"`
	Events    []todayCalendarEvent      `json:"events"`
}

type todayMonthCalendar struct {
	MonthStart string                    `json:"month_start"`
	MonthEnd   string                    `json:"month_end"`
	Status     string                    `json:"status"`
	Sources    []services.CalendarSource `json:"sources"`
	FetchedAt  *time.Time                `json:"fetched_at"`
	Events     []todayCalendarEvent      `json:"events"`
}

func NewTodayHandler(database *gorm.DB) *TodayHandler {
	return NewTodayHandlerWithCalendarProvider(database, nil)
}

func NewTodayHandlerWithCalendarProvider(database *gorm.DB, provider todayCalendarProvider) *TodayHandler {
	return &TodayHandler{db: database, calendar: provider, calendarTimeout: 6 * time.Second, now: time.Now}
}

func (h *TodayHandler) Get(c *gin.Context) {
	market := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("market", "US")))
	zone := "America/New_York"
	if market == "CN" {
		zone = "Asia/Shanghai"
	} else if market != "US" {
		Fail(c, http.StatusBadRequest, "market must be US or CN")
		return
	}
	location, _ := time.LoadLocation(zone)
	now := h.now().In(location)
	weekStart, weekEnd := currentWorkWeek(now)
	monthStart, monthEnd := currentMonth(now)
	response := todayResponse{
		Market: market, Timezone: zone, LocalDate: now.Format("2006-01-02"), Session: marketSession(market, now), CalendarStatus: "weekday_only_unverified", Items: []todayItem{},
		WeekCalendar:  todayWeekCalendar{WeekStart: weekStart.Format("2006-01-02"), WeekEnd: weekEnd.Format("2006-01-02"), Status: "local_only", Sources: []services.CalendarSource{}, Events: []todayCalendarEvent{}},
		MonthCalendar: todayMonthCalendar{MonthStart: monthStart.Format("2006-01-02"), MonthEnd: monthEnd.Format("2006-01-02"), Status: "local_only", Sources: []services.CalendarSource{}, Events: []todayCalendarEvent{}},
	}
	relevantSymbols := map[string]struct{}{}

	stockMarkets := []string{models.MarketSP500, models.MarketNASDAQ100, models.MarketRUSSELL2000}
	if market == "CN" {
		stockMarkets = []string{models.MarketSSEMain, models.MarketSZSEMain, models.MarketSTAR, models.MarketChiNext, models.MarketBSE}
	}
	var analyzedSymbols []string
	if err := h.db.WithContext(c).
		Model(&models.Stock{}).
		Joins("JOIN stock_metrics ON stock_metrics.stock_id = stocks.id").
		Where("stocks.market IN ? AND stock_metrics.ai_analysis_at IS NOT NULL", stockMarkets).
		Order("stock_metrics.ai_analysis_at DESC").
		Pluck("stocks.symbol", &analyzedSymbols).Error; err != nil {
		log.Printf("已分析标的读取失败（market=%s）：%v", market, err)
	}
	for _, symbol := range analyzedSymbols {
		relevantSymbols[strings.ToUpper(strings.TrimSpace(symbol))] = struct{}{}
	}
	var metric models.StockMetric
	if err := h.db.WithContext(c).Joins("JOIN stocks ON stocks.id = stock_metrics.stock_id").Where("stocks.market IN ?", stockMarkets).Order("stock_metrics.data_as_of desc").First(&metric).Error; err == nil {
		response.DataAsOf = &metric.DataAsOf
	}
	var plans []models.TradePlan
	if err := h.db.WithContext(c).Where("status IN ?", []string{models.TradePlanStatusDraft, models.TradePlanStatusWaiting}).Order("updated_at desc").Limit(40).Find(&plans).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "today plans could not be loaded")
		return
	}
	for _, plan := range plans {
		if !todayTradeMarketMatches(market, inferTradeMarket(plan.Ticker)) {
			continue
		}
		kind, priority := "waiting_plan", todayPriority(response.Session, "waiting_plan")
		var count int64
		h.db.WithContext(c).Model(&models.PreTradeRiskReview{}).Where("trade_plan_id = ?", plan.ID).Count(&count)
		if count == 0 {
			kind, priority = "unreviewed_plan", todayPriority(response.Session, "unreviewed_plan")
		}
		response.Items = append(response.Items, todayItem{ID: plan.ID.String(), Kind: kind, Title: plan.Ticker + " 交易计划", Subtitle: plan.Status, Href: "/trade-plans/" + plan.ID.String(), Priority: priority})
		relevantSymbols[strings.ToUpper(strings.TrimSpace(plan.Ticker))] = struct{}{}
		if plan.ValidUntil != nil && dateWithinWorkWeek(*plan.ValidUntil, weekStart, weekEnd) {
			symbol := strings.ToUpper(strings.TrimSpace(plan.Ticker))
			href := "/trade-plans/" + plan.ID.String()
			response.WeekCalendar.Events = append(response.WeekCalendar.Events, todayCalendarEvent{ID: "plan-" + plan.ID.String(), Date: plan.ValidUntil.Format("2006-01-02"), Kind: "plan_deadline", Title: symbol + " 计划到期", Subtitle: "到期前重新核查交易条件", Symbol: &symbol, Importance: "medium", Source: "AlphaLens", Href: &href})
		}
		if plan.ValidUntil != nil && dateWithinRange(*plan.ValidUntil, monthStart, monthEnd) {
			symbol := strings.ToUpper(strings.TrimSpace(plan.Ticker))
			href := "/trade-plans/" + plan.ID.String()
			response.MonthCalendar.Events = append(response.MonthCalendar.Events, todayCalendarEvent{ID: "plan-" + plan.ID.String(), Date: plan.ValidUntil.Format("2006-01-02"), Kind: "plan_deadline", Title: symbol + " 计划到期", Subtitle: "到期前重新核查交易条件", Symbol: &symbol, Importance: "medium", Source: "AlphaLens", Href: &href})
		}
	}
	var entries []models.TradeJournalEntry
	if err := h.db.WithContext(c).Order("executed_at desc").Limit(40).Find(&entries).Error; err != nil {
		Fail(c, http.StatusInternalServerError, "today executions could not be loaded")
		return
	}
	for _, entry := range entries {
		if !todayTradeMarketMatches(market, entry.Market) {
			continue
		}
		var completed int64
		h.db.WithContext(c).Model(&models.TradeReview{}).Where("journal_entry_id = ? AND completed_at IS NOT NULL", entry.ID).Count(&completed)
		if completed > 0 {
			continue
		}
		relevantSymbols[strings.ToUpper(strings.TrimSpace(entry.Symbol))] = struct{}{}
		kind := "pending_review"
		if entry.TradePlanID == nil {
			kind = "off_plan_execution"
		}
		response.Items = append(response.Items, todayItem{ID: entry.ID.String(), Kind: kind, Title: entry.Symbol + " 成交待复盘", Subtitle: entry.Source, Href: "/journal/" + entry.ID.String(), Priority: todayPriority(response.Session, kind)})
	}
	var candidates []models.CandidateSnapshot
	h.db.WithContext(c).Where("created_at >= ? AND market IN ?", h.now().Add(-24*time.Hour), stockMarkets).Order("created_at desc").Limit(12).Find(&candidates)
	for _, candidate := range candidates {
		response.Items = append(response.Items, todayItem{ID: candidate.ID.String(), Kind: "candidate", Title: candidate.Symbol + " 候选", Subtitle: candidate.ScreenName, Href: "/analysis/" + candidate.Symbol, Priority: todayPriority(response.Session, "candidate")})
		relevantSymbols[strings.ToUpper(strings.TrimSpace(candidate.Symbol))] = struct{}{}
	}
	if h.calendar != nil && h.calendar.Configured() {
		symbols := make([]string, 0, len(relevantSymbols))
		for symbol := range relevantSymbols {
			if symbol != "" {
				symbols = append(symbols, symbol)
			}
		}
		sort.Strings(symbols)
		calendarContext, cancelCalendar := context.WithTimeout(c.Request.Context(), h.calendarTimeout)
		external, calendarErr := h.calendar.FetchMonthEvents(calendarContext, market, monthStart, monthEnd, symbols)
		cancelCalendar()
		fetchedAt := h.now().UTC()
		response.WeekCalendar.FetchedAt = &fetchedAt
		response.WeekCalendar.Sources = external.Sources
		response.MonthCalendar.FetchedAt = &fetchedAt
		response.MonthCalendar.Sources = external.Sources
		if calendarErr == nil && len(external.Sources) > 0 {
			response.WeekCalendar.Status = "live"
			response.MonthCalendar.Status = "live"
		} else if len(external.Events) > 0 || len(external.Sources) > 0 {
			response.WeekCalendar.Status = "partial"
			response.MonthCalendar.Status = "partial"
		}
		if calendarErr != nil {
			log.Printf("本周事件日历降级（market=%s）：%v", market, calendarErr)
		}
		for index, event := range external.Events {
			if !dateWithinRange(event.Date, monthStart, monthEnd) {
				continue
			}
			var clock, symbol, href *string
			if event.Time != "" {
				value := event.Time
				clock = &value
			}
			if event.Symbol != "" {
				value := event.Symbol
				symbol = &value
				link := "/analysis/" + event.Symbol
				href = &link
			}
			var result, impact, reason, liveURL *string
			if event.Result != "" {
				value := event.Result
				result = &value
			}
			if event.MarketImpact != "" {
				value := event.MarketImpact
				impact = &value
			}
			if event.ImpactReason != "" {
				value := event.ImpactReason
				reason = &value
			}
			if event.LiveURL != "" {
				value := event.LiveURL
				liveURL = &value
			}
			calendarEvent := todayCalendarEvent{ID: event.Kind + "-" + event.Date.Format("20060102") + "-" + event.Symbol + "-" + strconv.Itoa(index), Date: event.Date.Format("2006-01-02"), Time: clock, Kind: event.Kind, Title: event.Title, Subtitle: event.Subtitle, Symbol: symbol, Importance: event.Importance, Source: event.Source, Href: href, Result: result, MarketImpact: impact, ImpactReason: reason, LiveURL: liveURL}
			response.MonthCalendar.Events = append(response.MonthCalendar.Events, calendarEvent)
			if dateWithinWorkWeek(event.Date, weekStart, weekEnd) {
				response.WeekCalendar.Events = append(response.WeekCalendar.Events, calendarEvent)
			}
		}
	}
	sort.SliceStable(response.WeekCalendar.Events, func(i, j int) bool {
		left, right := response.WeekCalendar.Events[i], response.WeekCalendar.Events[j]
		if left.Date != right.Date {
			return left.Date < right.Date
		}
		leftTime, rightTime := "", ""
		if left.Time != nil {
			leftTime = *left.Time
		}
		if right.Time != nil {
			rightTime = *right.Time
		}
		return leftTime < rightTime
	})
	sort.SliceStable(response.MonthCalendar.Events, func(i, j int) bool {
		left, right := response.MonthCalendar.Events[i], response.MonthCalendar.Events[j]
		if left.Date != right.Date {
			return left.Date < right.Date
		}
		leftTime, rightTime := "", ""
		if left.Time != nil {
			leftTime = *left.Time
		}
		if right.Time != nil {
			rightTime = *right.Time
		}
		return leftTime < rightTime
	})
	sortTodayItems(response.Items)
	OK(c, response)
}

func currentWorkWeek(local time.Time) (time.Time, time.Time) {
	daysSinceMonday := (int(local.Weekday()) + 6) % 7
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).AddDate(0, 0, -daysSinceMonday)
	return start, start.AddDate(0, 0, 4)
}

func currentMonth(local time.Time) (time.Time, time.Time) {
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	return start, start.AddDate(0, 1, -1)
}

func dateWithinRange(value, start, end time.Time) bool {
	date := value.Format("2006-01-02")
	return date >= start.Format("2006-01-02") && date <= end.Format("2006-01-02")
}

func dateWithinWorkWeek(value, start, end time.Time) bool {
	date := value.Format("2006-01-02")
	return date >= start.Format("2006-01-02") && date <= end.Format("2006-01-02")
}

func todayTradeMarketMatches(assetMarket, tradeMarket string) bool {
	if assetMarket == "CN" {
		return tradeMarket == models.TradeMarketChinaA
	}
	return tradeMarket == models.TradeMarketUS
}

func marketSession(market string, local time.Time) string {
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return "non_trading"
	}
	minutes := local.Hour()*60 + local.Minute()
	if market == "US" {
		if minutes >= 240 && minutes < 570 {
			return "premarket"
		}
		if minutes >= 570 && minutes < 960 {
			return "intraday"
		}
		if minutes >= 960 && minutes < 1200 {
			return "postmarket"
		}
	} else {
		if minutes >= 420 && minutes < 570 {
			return "premarket"
		}
		if minutes >= 570 && minutes < 900 {
			return "intraday"
		}
		if minutes >= 900 && minutes < 1080 {
			return "postmarket"
		}
	}
	return "non_trading"
}

func todayPriority(session, kind string) int {
	orders := map[string]map[string]int{
		"premarket":   {"candidate": 10, "unreviewed_plan": 20, "waiting_plan": 30, "off_plan_execution": 40, "pending_review": 50},
		"intraday":    {"waiting_plan": 10, "unreviewed_plan": 20, "off_plan_execution": 30, "candidate": 40, "pending_review": 50},
		"postmarket":  {"off_plan_execution": 10, "pending_review": 20, "unreviewed_plan": 30, "waiting_plan": 40, "candidate": 50},
		"non_trading": {"pending_review": 10, "off_plan_execution": 20, "candidate": 30, "unreviewed_plan": 40, "waiting_plan": 50},
	}
	if priority, ok := orders[session][kind]; ok {
		return priority
	}
	return 99
}

func sortTodayItems(items []todayItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Priority < items[j-1].Priority; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
