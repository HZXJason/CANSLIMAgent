package handlers

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type deskStubFetcher struct{}

func (deskStubFetcher) KLine(_ context.Context, symbol, _ string) (services.KLineData, error) {
	candles := make([]services.Candle, 0, 60)
	price := 100.0
	for i := 0; i < 60; i++ {
		price += 0.5
		candles = append(candles, services.Candle{
			Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02"),
			Open:   price * 0.99,
			High:   price * 1.01,
			Low:    price * 0.98,
			Close:  price,
			Volume: 1_000_000,
		})
	}
	return services.KLineData{Symbol: symbol, Source: "stub", DataAsOf: candles[len(candles)-1].Date, Candles: candles}, nil
}

func TestDeskReturnsMarketRegimeAndPortfolioSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.User{}, &models.TradingProfile{}, &models.TradePlan{}, &models.Trade{},
		&models.TradeJournalEntry{}, &models.TradeReview{}, &models.PreTradeRiskReview{},
	); err != nil {
		t.Fatal(err)
	}
	user := models.DefaultUser()
	_ = database.Create(&user).Error
	_ = database.Create(&models.TradePlan{Ticker: "NVDA", Direction: models.TradePlanDirectionBuy, Status: models.TradePlanStatusWaiting, PlanDate: time.Now().UTC()}).Error

	regimes := services.NewMarketRegimeService(deskStubFetcher{})
	router := gin.New()
	RegisterDeskRoutes(router.Group("/api/v1"), NewDeskHandler(database, regimes))

	resp := performJSON(router, http.MethodGet, "/api/v1/desk?market=US", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("desk = %d %s", resp.Code, resp.Body.String())
	}
	for _, needle := range []string{`"confirmed_uptrend"`, `"waiting_plans":[{`, `"NVDA"`, `"disciplined_r_loop_rate"`} {
		if !bytes.Contains(resp.Body.Bytes(), []byte(needle)) {
			t.Fatalf("desk missing %s: %s", needle, resp.Body.String())
		}
	}
}
