package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/alphalens/canslimagent/backend/internal/db"
	"github.com/alphalens/canslimagent/backend/internal/handlers"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	database, err := db.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("数据库初始化失败：%v", err)
	}

	alphaVantageClient := services.NewAlphaVantageClient(cfg.AlphaVantageAPIKey, cfg.RequestTimeout)
	translationClient := services.NewTranslationClient(cfg.EnableProfileTranslate, cfg.RequestTimeout)
	fmpClient := services.NewFMPClient(cfg.FMPAPIKey, cfg.RequestTimeout)
	polygonClient := services.NewPolygonClient(cfg.PolygonAPIKey, cfg.RequestTimeout)
	freeUniverseClient := services.NewFreeUniverseClient(cfg.EnableFreeUniverse, cfg.RequestTimeout)
	secClient := services.NewSECClient(cfg.EnableSECData, cfg.SECUserAgent, cfg.RequestTimeout)
	institutionalClient := services.NewInstitutionalClient(cfg.EnableInstitutionalData, cfg.RequestTimeout)
	stooqClient := services.NewStooqClient(cfg.EnableStooqData, cfg.RequestTimeout)
	yahooClient := services.NewYahooClient(cfg.EnableYahooFallback, cfg.RequestTimeout)
	yfinanceClient := services.NewYFinanceClient(cfg.EnableYFinanceFallback, cfg.YFinancePython, cfg.RequestTimeout)

	scanner := services.NewScannerService(
		database,
		freeUniverseClient,
		secClient,
		institutionalClient,
		stooqClient,
		fmpClient,
		polygonClient,
		yahooClient,
		yfinanceClient,
		cfg.ScanDelay,
	)
	analysisService := services.NewAnalysisService(database, alphaVantageClient, translationClient, secClient, stooqClient, fmpClient, polygonClient, yahooClient, yfinanceClient)
	screenerHandler := handlers.NewScreenerHandler(scanner)
	presetHandler := handlers.NewPresetHandler(database)
	chartsHandler := handlers.NewChartsHandler(analysisService)
	analysisHandler := handlers.NewAnalysisHandler(analysisService, cfg.AIStreamURL)
	aiAnalysisHandler := handlers.NewAIAnalysisHandler()
	journalHandler := handlers.NewJournalHandler(database)
	tradePlanHandler := handlers.NewTradePlanHandler(database)

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:        cfg.AllowedOrigins,
		AllowOriginFunc:     allowConfiguredOrLocalOrigin(cfg.AllowedOrigins),
		AllowMethods:        []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:        []string{"Origin", "Content-Type", "Accept", "Authorization", "Cache-Control", "Pragma", "X-Requested-With"},
		ExposeHeaders:       []string{"Content-Length"},
		AllowCredentials:    true,
		AllowPrivateNetwork: true,
	}))

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			handlers.OK(c, gin.H{"status": "ok"})
		})
		api.GET("/screener/scan", screenerHandler.Scan)
		api.GET("/screener/results", screenerHandler.Results)
		api.POST("/screener/refresh", screenerHandler.Refresh)
		api.GET("/screener/refresh/:jobID", screenerHandler.RefreshJob)
		api.GET("/stocks/:symbol/summary", chartsHandler.Summary)
		api.GET("/stocks/:symbol/profile", chartsHandler.Profile)
		api.GET("/stocks/:symbol/kline", chartsHandler.KLine)
		api.GET("/stocks/:symbol/rs", chartsHandler.RS)
		api.GET("/stocks/:symbol/eps/quarterly", chartsHandler.EPSQuarterly)
		api.GET("/stocks/:symbol/revenue/annual", chartsHandler.RevenueAnnual)
		api.GET("/stocks/:symbol/margin/gross/quarterly", chartsHandler.GrossMarginQuarterly)
		api.GET("/stocks/:symbol/second-layer-data", chartsHandler.SecondLayerData)
		api.PATCH("/stocks/:symbol/n-label", screenerHandler.UpdateNLabel)
		api.GET("/analysis/:symbol/*action", func(c *gin.Context) {
			action := c.Param("action")
			if c.Param("symbol") == "ai" && action == "/config" {
				aiAnalysisHandler.Config(c)
				return
			}
			if action == "/stream" {
				analysisHandler.Stream(c)
				return
			}
			c.Status(http.StatusNotFound)
		})
		api.POST("/analysis/:symbol/ai", aiAnalysisHandler.Stream)
		api.GET("/presets", presetHandler.List)
		api.POST("/presets", presetHandler.Create)
		api.DELETE("/presets/:id", presetHandler.Delete)
		api.GET("/journal", journalHandler.List)
		api.POST("/journal", journalHandler.Create)
		api.GET("/journal/:id", journalHandler.Get)
		api.PATCH("/journal/:id", journalHandler.Patch)
		api.GET("/trade-plans", tradePlanHandler.List)
		api.POST("/trade-plans", tradePlanHandler.Create)
		api.GET("/trade-plans/:id", tradePlanHandler.Get)
		api.PATCH("/trade-plans/:id", tradePlanHandler.Patch)
		api.POST("/trade-plans/:id/execute", tradePlanHandler.Execute)
	}

	log.Printf("AlphaLens 后端已启动：http://localhost:%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("服务启动失败：%v", err)
	}
}

func allowConfiguredOrLocalOrigin(allowedOrigins []string) func(string) bool {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimSpace(origin)] = struct{}{}
	}

	return func(origin string) bool {
		if _, ok := allowed[origin]; ok {
			return true
		}

		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return false
		}
		host := parsed.Hostname()
		port := parsed.Port()
		if port == "5173" {
			return true
		}
		return port == "5174" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
	}
}
