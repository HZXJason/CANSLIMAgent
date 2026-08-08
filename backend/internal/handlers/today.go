package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TodayHandler struct {
	db  *gorm.DB
	now func() time.Time
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
	Market         string      `json:"market"`
	Timezone       string      `json:"timezone"`
	LocalDate      string      `json:"local_date"`
	Session        string      `json:"session"`
	CalendarStatus string      `json:"calendar_status"`
	DataAsOf       *time.Time  `json:"data_as_of"`
	Items          []todayItem `json:"items"`
}

func NewTodayHandler(database *gorm.DB) *TodayHandler {
	return &TodayHandler{db: database, now: time.Now}
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
	response := todayResponse{Market: market, Timezone: zone, LocalDate: now.Format("2006-01-02"), Session: marketSession(market, now), CalendarStatus: "weekday_only_unverified", Items: []todayItem{}}

	stockMarkets := []string{models.MarketSP500, models.MarketNASDAQ100, models.MarketRUSSELL2000}
	if market == "CN" {
		stockMarkets = []string{models.MarketSSEMain, models.MarketSZSEMain, models.MarketSTAR, models.MarketChiNext, models.MarketBSE}
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
	}
	sortTodayItems(response.Items)
	OK(c, response)
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
