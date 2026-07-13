package config

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Host                    string
	Port                    string
	AppEnv                  string
	APIAuthToken            string
	SidecarSharedSecret     string
	RunDatabaseMigrations   bool
	MigrateOnly             bool
	DatabaseURL             string
	AlphaVantageAPIKey      string
	FMPAPIKey               string
	PolygonAPIKey           string
	EnableFreeUniverse      bool
	EnableSECData           bool
	EnableInstitutionalData bool
	SECUserAgent            string
	EnableStooqData         bool
	EnableYahooFallback     bool
	EnableYFinanceFallback  bool
	EnableProfileTranslate  bool
	YFinancePython          string
	AIStreamURL             string
	AIAnalysisUpstreamURL   string
	AIAnalysisTimeout       time.Duration
	BrokerSyncUpstreamURL   string
	BrokerCredentialsKey    string
	LLMCredentialsKey       string
	BrokerLongPortOAuth     BrokerOAuthConfig
	BrokerTigerOAuth        BrokerOAuthConfig
	BrokerFutuGateway       BrokerGatewayConfig
	BrokerIBKRGateway       BrokerGatewayConfig
	FrontendURL             string
	AllowedOrigins          []string
	RequestTimeout          time.Duration
	ScanDelay               time.Duration
	MaxConcurrentRequests   int
}

// BrokerOAuthConfig is server-only configuration for an OAuth broker app.
// It must never be sent to the browser.
type BrokerOAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	AccountURL   string
	RedirectURL  string
	Scopes       []string
}

func (c BrokerOAuthConfig) Enabled() bool {
	return strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.AuthorizeURL) != "" &&
		strings.TrimSpace(c.TokenURL) != "" &&
		strings.TrimSpace(c.RedirectURL) != ""
}

// BrokerGatewayConfig contains fixed, server-controlled loopback settings.
type BrokerGatewayConfig struct {
	Host     string
	Port     string
	ClientID string
	Account  string
}

func Load() Config {
	_ = godotenv.Load(".env", "backend/.env")
	appEnv := strings.ToLower(getEnv("APP_ENV", "development"))
	futuGateway := loadBrokerGateway("BROKER_FUTU", "11111")
	ibkrGateway := loadBrokerGateway("BROKER_IBKR", "7497")
	ibkrGateway.ClientID = getEnv("BROKER_IBKR_CLIENT_ID", "9901")
	brokerCredentialsKey := strings.TrimSpace(os.Getenv("BROKER_CREDENTIALS_ENCRYPTION_KEY"))
	llmCredentialsKey := strings.TrimSpace(os.Getenv("LLM_CREDENTIALS_ENCRYPTION_KEY"))
	if llmCredentialsKey == "" {
		llmCredentialsKey = brokerCredentialsKey
	}

	return Config{
		Host:                    getEnv("HOST", "127.0.0.1"),
		Port:                    getEnv("PORT", "8080"),
		AppEnv:                  appEnv,
		APIAuthToken:            strings.TrimSpace(os.Getenv("API_AUTH_TOKEN")),
		SidecarSharedSecret:     strings.TrimSpace(os.Getenv("SIDECAR_SHARED_SECRET")),
		RunDatabaseMigrations:   getEnvBool("RUN_DATABASE_MIGRATIONS", appEnv != "production"),
		MigrateOnly:             getEnvBool("MIGRATE_ONLY", false),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		AlphaVantageAPIKey:      os.Getenv("ALPHA_VANTAGE_API_KEY"),
		FMPAPIKey:               os.Getenv("FMP_API_KEY"),
		PolygonAPIKey:           os.Getenv("POLYGON_API_KEY"),
		EnableFreeUniverse:      getEnvBool("ENABLE_FREE_UNIVERSE", true),
		EnableSECData:           getEnvBool("ENABLE_SEC_DATA", true),
		EnableInstitutionalData: getEnvBool("ENABLE_INSTITUTIONAL_DATA", true),
		SECUserAgent:            getEnv("SEC_USER_AGENT", "CANSLIMAgent/1.0 (local research; set SEC_USER_AGENT for contact)"),
		EnableStooqData:         getEnvBool("ENABLE_STOOQ_DATA", true),
		EnableYahooFallback:     getEnvBool("ENABLE_YAHOO_FALLBACK", true),
		EnableYFinanceFallback:  getEnvBool("ENABLE_YFINANCE_FALLBACK", true),
		EnableProfileTranslate:  getEnvBool("ENABLE_PROFILE_TRANSLATION", true),
		YFinancePython:          getEnv("YFINANCE_PYTHON", "python"),
		AIStreamURL:             os.Getenv("AI_STREAM_URL"),
		AIAnalysisUpstreamURL:   getFirstEnv([]string{"AI_ANALYSIS_UPSTREAM_URL", "AI_ANALYSIS_SIDECAR_URL"}, "http://127.0.0.1:8765"),
		AIAnalysisTimeout:       time.Duration(getEnvInt("AI_ANALYSIS_TIMEOUT_SECONDS", 1200)) * time.Second,
		BrokerSyncUpstreamURL:   getEnv("BROKER_SYNC_UPSTREAM_URL", "http://127.0.0.1:8765"),
		BrokerCredentialsKey:    brokerCredentialsKey,
		LLMCredentialsKey:       llmCredentialsKey,
		BrokerLongPortOAuth:     loadLongPortOAuthConfig(),
		BrokerTigerOAuth:        loadBrokerOAuthConfig("BROKER_TIGER"),
		BrokerFutuGateway:       futuGateway,
		BrokerIBKRGateway:       ibkrGateway,
		FrontendURL:             getEnv("FRONTEND_URL", "http://127.0.0.1:5173"),
		AllowedOrigins:          splitOrigins(getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		RequestTimeout:          time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 20)) * time.Second,
		ScanDelay:               time.Duration(getEnvInt("SCAN_DELAY_MS", 80)) * time.Millisecond,
		MaxConcurrentRequests:   getEnvInt("MAX_CONCURRENT_REQUESTS", 64),
	}
}

func loadLongPortOAuthConfig() BrokerOAuthConfig {
	oauth := loadBrokerOAuthConfig("BROKER_LONGPORT")
	oauth.AuthorizeURL = getEnv("BROKER_LONGPORT_AUTHORIZE_URL", "https://openapi.longbridge.com/oauth2/authorize")
	oauth.TokenURL = getEnv("BROKER_LONGPORT_TOKEN_URL", "https://openapi.longbridge.com/oauth2/token")
	oauth.RedirectURL = getEnv("BROKER_LONGPORT_REDIRECT_URL", "http://127.0.0.1:"+getEnv("PORT", "8080")+"/api/v1/brokers/oauth/longport/callback")
	if len(oauth.Scopes) == 0 {
		// Longbridge documents scope=3 for the OAuth authorization-code flow.
		oauth.Scopes = []string{"3"}
	}
	return oauth
}

func loadBrokerGateway(prefix, defaultPort string) BrokerGatewayConfig {
	host := getEnv(prefix+"_HOST", "127.0.0.1")
	port := getEnv(prefix+"_PORT", defaultPort)
	if !isLoopbackHost(host) {
		host = "127.0.0.1"
	}
	if !isValidPort(port) {
		port = defaultPort
	}
	return BrokerGatewayConfig{Host: host, Port: port, Account: strings.TrimSpace(os.Getenv(prefix + "_ACCOUNT"))}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func isValidPort(port string) bool {
	value, err := strconv.Atoi(strings.TrimSpace(port))
	return err == nil && value >= 1 && value <= 65535
}

func loadBrokerOAuthConfig(prefix string) BrokerOAuthConfig {
	return BrokerOAuthConfig{
		ClientID:     strings.TrimSpace(os.Getenv(prefix + "_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv(prefix + "_CLIENT_SECRET")),
		AuthorizeURL: strings.TrimSpace(os.Getenv(prefix + "_AUTHORIZE_URL")),
		TokenURL:     strings.TrimSpace(os.Getenv(prefix + "_TOKEN_URL")),
		AccountURL:   strings.TrimSpace(os.Getenv(prefix + "_ACCOUNT_URL")),
		RedirectURL:  strings.TrimSpace(os.Getenv(prefix + "_REDIRECT_URL")),
		Scopes:       splitScopes(os.Getenv(prefix + "_SCOPES")),
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getFirstEnv(keys []string, fallback string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func splitOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			origins = append(origins, part)
		}
	}
	return origins
}

func splitScopes(raw string) []string {
	return splitOrigins(raw)
}
