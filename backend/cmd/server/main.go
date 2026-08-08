package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alphalens/canslimagent/backend/config"
	"github.com/alphalens/canslimagent/backend/internal/db"
	"github.com/alphalens/canslimagent/backend/internal/handlers"
	"github.com/alphalens/canslimagent/backend/internal/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const (
	internalAPITokenHeader = "X-Internal-API-Token" // #nosec G101 -- header name, not a credential
	requestIDHeader        = "X-Request-ID"
	maxRequestBodyBytes    = 4 << 20
)

func main() {
	cfg := config.Load()
	if err := validateRuntimeSecurity(cfg); err != nil {
		log.Fatalf("安全配置无效：%v", err)
	}
	binding.EnableDecoderDisallowUnknownFields = true

	database, err := db.ConnectPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("数据库初始化失败：%v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败：%v", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.Printf("数据库关闭异常：%v", closeErr)
		}
	}()
	if cfg.RunDatabaseMigrations {
		if err := db.Migrate(database); err != nil {
			log.Fatalf("数据库迁移失败：%v", err)
		}
	}
	if cfg.MigrateOnly {
		if !cfg.RunDatabaseMigrations {
			log.Fatal("MIGRATE_ONLY requires RUN_DATABASE_MIGRATIONS=true")
		}
		log.Println("数据库迁移完成")
		return
	}

	alphaVantageClient := services.NewAlphaVantageClient(cfg.AlphaVantageAPIKey, cfg.RequestTimeout)
	translationClient := services.NewTranslationClient(cfg.EnableProfileTranslate, cfg.RequestTimeout)
	fmpClient := services.NewFMPClient(cfg.FMPAPIKey, cfg.RequestTimeout)
	polygonClient := services.NewPolygonClient(cfg.PolygonAPIKey, cfg.RequestTimeout)
	freeUniverseClient := services.NewFreeUniverseClient(cfg.EnableFreeUniverse, cfg.RequestTimeout)
	ashareClient := services.NewAshareClient(cfg.EnableAshareData, cfg.RequestTimeout)
	secClient := services.NewSECClient(cfg.EnableSECData, cfg.SECUserAgent, cfg.RequestTimeout)
	institutionalClient := services.NewInstitutionalClient(cfg.EnableInstitutionalData, cfg.RequestTimeout)
	stooqClient := services.NewStooqClient(cfg.EnableStooqData, cfg.RequestTimeout)
	yahooClient := services.NewYahooClient(cfg.EnableYahooFallback, cfg.RequestTimeout)
	yfinanceClient := services.NewYFinanceClient(cfg.EnableYFinanceFallback, cfg.YFinancePython, cfg.RequestTimeout)

	scanner := services.NewScannerService(
		database,
		freeUniverseClient,
		ashareClient,
		secClient,
		institutionalClient,
		stooqClient,
		fmpClient,
		polygonClient,
		yahooClient,
		yfinanceClient,
		cfg.ScanDelay,
	)
	analysisService := services.NewAnalysisService(database, alphaVantageClient, translationClient, ashareClient, secClient, stooqClient, fmpClient, polygonClient, yahooClient, yfinanceClient)
	screenerHandler := handlers.NewScreenerHandler(scanner)
	presetHandler := handlers.NewPresetHandler(database)
	chartsHandler := handlers.NewChartsHandler(analysisService, scanner)
	analysisHandler := handlers.NewAnalysisHandler(analysisService, cfg.AIStreamURL, cfg.RequestTimeout)
	llmVault, llmVaultErr := services.NewCredentialVault(cfg.LLMCredentialsKey)
	if llmVaultErr != nil {
		log.Printf("LLM 密钥存储未启用：%v", llmVaultErr)
	}
	llmCredentialStore := services.NewLLMCredentialStore(database, llmVault)
	llmCredentialHandler := handlers.NewLLMCredentialHandler(llmCredentialStore)
	aiAnalysisUpstream := services.NewHTTPAIAnalysisUpstream(cfg.AIAnalysisUpstreamURL, cfg.AIAnalysisTimeout, cfg.SidecarSharedSecret)
	aiAnalysisHandler := handlers.NewAIAnalysisHandlerWithUpstreamAndJournal(aiAnalysisUpstream, cfg.AIAnalysisTimeout, database).WithCredentialStore(llmCredentialStore)
	journalReflectionUpstream := services.NewHTTPJournalReflectionUpstream(cfg.AIAnalysisUpstreamURL, 2*time.Minute, cfg.SidecarSharedSecret)
	journalHandler := handlers.NewJournalHandlerWithReflection(database, journalReflectionUpstream)
	tradeReviewHandler := handlers.NewTradeReviewHandler(database)
	tradePlanHandler := handlers.NewTradePlanHandler(database)
	preTradeAIReviewer := services.NewHTTPPreTradeAIReviewer(cfg.AIAnalysisUpstreamURL, 2*time.Minute, cfg.SidecarSharedSecret)
	preTradeReviewHandler := handlers.NewPreTradeReviewHandler(database, preTradeAIReviewer, llmCredentialStore)
	playbookSuggester := services.NewHTTPPlaybookSuggester(cfg.AIAnalysisUpstreamURL, 2*time.Minute, cfg.SidecarSharedSecret)
	profileHandler := handlers.NewProfileHandler(database, playbookSuggester).WithCredentialStore(llmCredentialStore)
	decisionSpineHandler := handlers.NewDecisionSpineHandler(database)
	todayHandler := handlers.NewTodayHandler(database)
	screenDefinitionHandler := handlers.NewScreenDefinitionHandler(database)
	brokerVault, brokerVaultErr := services.NewBrokerCredentialVault(cfg.BrokerCredentialsKey)
	if brokerVaultErr != nil {
		log.Printf("券商凭据存储未启用：%v", brokerVaultErr)
	}
	brokerSyncUpstream := services.NewHTTPBrokerSyncUpstream(cfg.BrokerSyncUpstreamURL, 45*time.Second, cfg.SidecarSharedSecret)
	brokerGatewayClient := services.NewHTTPBrokerGatewayClient(cfg.BrokerSyncUpstreamURL, map[string]config.BrokerGatewayConfig{
		"futu": cfg.BrokerFutuGateway,
		"ibkr": cfg.BrokerIBKRGateway,
	}).WithSharedSecret(cfg.SidecarSharedSecret)
	brokerAuthorizer := services.NewHTTPBrokerAuthorizer(map[string]config.BrokerOAuthConfig{
		"longport": cfg.BrokerLongPortOAuth,
		"tiger":    cfg.BrokerTigerOAuth,
	}, brokerVault, brokerGatewayClient, cfg.RequestTimeout)
	brokerHandler := handlers.NewBrokerHandlerWithFrontendURL(database, brokerVault, brokerSyncUpstream, brokerAuthorizer, cfg.FrontendURL)

	router := gin.New()
	router.Use(safeAccessLogger(), gin.Recovery())
	router.Use(requestSecurityHeaders(), requestID(), limitRequestBody(maxRequestBodyBytes), internalAPIAuth(cfg.APIAuthToken), concurrencyLimit(cfg.MaxConcurrentRequests))
	router.Use(cors.New(cors.Config{
		AllowOrigins:        cfg.AllowedOrigins,
		AllowOriginFunc:     configuredOriginAllowed(cfg.AllowedOrigins),
		AllowMethods:        apiAllowedMethods(),
		AllowHeaders:        []string{"Origin", "Content-Type", "Accept", "Authorization", "Cache-Control", "Pragma", "X-Requested-With", requestIDHeader},
		ExposeHeaders:       []string{"Content-Length", "X-Analysis-Run-ID", requestIDHeader},
		AllowCredentials:    false,
		AllowPrivateNetwork: false,
	}))

	api := router.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err != nil {
				handlers.Fail(c, http.StatusServiceUnavailable, "database unavailable")
				return
			}
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
		api.PATCH("/stocks/:symbol/ai-analysis", screenerHandler.MarkAIAnalysisCompleted)
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
		api.POST("/analysis/ai/models", aiAnalysisHandler.DiscoverModels)
		api.POST("/analysis/:symbol/ai", aiAnalysisHandler.Stream)
		api.GET("/llm/credentials/:provider", llmCredentialHandler.Status)
		api.PUT("/llm/credentials/:provider", llmCredentialHandler.Save)
		api.DELETE("/llm/credentials/:provider", llmCredentialHandler.Delete)
		api.GET("/presets", presetHandler.List)
		api.GET("/today", todayHandler.Get)
		api.GET("/screen-definitions", screenDefinitionHandler.List)
		api.POST("/screen-definitions", screenDefinitionHandler.Create)
		api.POST("/presets", presetHandler.Create)
		api.DELETE("/presets/:id", presetHandler.Delete)
		api.GET("/journal", journalHandler.List)
		api.POST("/journal", journalHandler.Create)
		api.GET("/journal/export", journalHandler.Export)
		api.POST("/journal/reflections", journalHandler.Reflect)
		api.GET("/journal/reflections/latest", journalHandler.LatestReflection)
		api.GET("/journal/:id", journalHandler.Get)
		api.PATCH("/journal/:id", journalHandler.Patch)
		api.DELETE("/journal/:id", journalHandler.Delete)
		api.GET("/brokers", brokerHandler.List)
		api.POST("/brokers/connect/:broker", brokerHandler.Connect)
		api.GET("/brokers/oauth/:broker/callback", brokerHandler.OAuthCallback)
		api.POST("/brokers/detect/:broker", brokerHandler.Detect)
		api.DELETE("/brokers/:id", brokerHandler.Delete)
		api.POST("/brokers/:id/sync", brokerHandler.Sync)
		api.POST("/brokers/:id/import", brokerHandler.Import)
		api.GET("/trade-plans", tradePlanHandler.List)
		api.POST("/trade-plans", tradePlanHandler.Create)
		api.GET("/trade-plans/:id", tradePlanHandler.Get)
		api.PATCH("/trade-plans/:id", tradePlanHandler.Patch)
		api.POST("/trade-plans/:id/execute", tradePlanHandler.Execute)
		handlers.RegisterPreTradeReviewRoutes(api, preTradeReviewHandler)
		handlers.RegisterTradeReviewRoutes(api, tradeReviewHandler)
		handlers.RegisterProfileRoutes(api, profileHandler)
		handlers.RegisterDecisionSpineRoutes(api, decisionSpineHandler)
	}

	log.Printf("AlphaLens 后端已启动：http://%s", net.JoinHostPort(cfg.Host, cfg.Port))

	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败：%v", err)
		}
	}()

	<-quit
	log.Println("正在关闭服务...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("服务关闭异常：%v", err)
	}
	log.Println("服务已安全关闭")
}

func apiAllowedMethods() []string {
	return []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
}

func configuredOriginAllowed(allowedOrigins []string) func(string) bool {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if normalized := strings.TrimRight(strings.TrimSpace(origin), "/"); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}

	return func(origin string) bool {
		_, ok := allowed[strings.TrimRight(strings.TrimSpace(origin), "/")]
		return ok
	}
}

func validateRuntimeSecurity(cfg config.Config) error {
	production := cfg.AppEnv == "production"
	if production && len(cfg.APIAuthToken) < 32 {
		return fmt.Errorf("production requires API_AUTH_TOKEN with at least 32 characters")
	}
	if production && len(cfg.SidecarSharedSecret) < 32 {
		return fmt.Errorf("production requires SIDECAR_SHARED_SECRET with at least 32 characters")
	}
	if !isLoopbackBindHost(cfg.Host) && len(cfg.APIAuthToken) < 32 {
		return fmt.Errorf("non-loopback HOST requires API_AUTH_TOKEN with at least 32 characters")
	}
	return nil
}

func isLoopbackBindHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func internalAPIAuth(token string) gin.HandlerFunc {
	token = strings.TrimSpace(token)
	return func(c *gin.Context) {
		if token == "" || c.Request.URL.Path == "/api/v1/health" || isOAuthCallbackPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		provided := c.GetHeader(internalAPITokenHeader)
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func limitRequestBody(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func requestSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if len(id) < 8 || len(id) > 128 {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err == nil {
				id = hex.EncodeToString(raw[:])
			} else {
				id = fmt.Sprintf("%d", time.Now().UnixNano())
			}
		}
		c.Header(requestIDHeader, id)
		c.Set("request_id", id)
		c.Next()
	}
}

func concurrencyLimit(limit int) gin.HandlerFunc {
	if limit < 1 {
		limit = 64
	}
	permits := make(chan struct{}, limit)
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/api/v1/health" {
			c.Next()
			return
		}
		select {
		case permits <- struct{}{}:
			defer func() { <-permits }()
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "server is busy"})
		}
	}
}

// safeAccessLogger keeps Gin's default access logging for ordinary requests
// while preventing one-time OAuth codes and state values from reaching logs.
func safeAccessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		_ = json.NewEncoder(gin.DefaultWriter).Encode(struct {
			Timestamp  string `json:"timestamp"`
			RequestID  string `json:"request_id,omitempty"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			Status     int    `json:"status"`
			DurationMS int64  `json:"duration_ms"`
		}{
			Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
			RequestID:  c.GetString("request_id"),
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			Status:     c.Writer.Status(),
			DurationMS: time.Since(started).Milliseconds(),
		})
	}
}

func isOAuthCallbackPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 7 &&
		parts[1] == "api" &&
		parts[2] == "v1" &&
		parts[3] == "brokers" &&
		parts[4] == "oauth" &&
		parts[5] != "" &&
		parts[6] == "callback"
}
