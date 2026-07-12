package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                    string
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
	BrokerLongPortOAuth     BrokerOAuthConfig
	BrokerTigerOAuth        BrokerOAuthConfig
	BrokerFutuGateway       BrokerGatewayConfig
	BrokerIBKRGateway       BrokerGatewayConfig
	AllowedOrigins          []string
	RequestTimeout          time.Duration
	ScanDelay               time.Duration
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
		strings.TrimSpace(c.ClientSecret) != "" &&
		strings.TrimSpace(c.AuthorizeURL) != "" &&
		strings.TrimSpace(c.TokenURL) != "" &&
		strings.TrimSpace(c.AccountURL) != "" &&
		strings.TrimSpace(c.RedirectURL) != ""
}

// BrokerGatewayConfig contains fixed, server-controlled loopback settings.
type BrokerGatewayConfig struct {
	Host     string
	Port     string
	ClientID string
}

func Load() Config {
	_ = godotenv.Load(".env", "backend/.env")

	return Config{
		Port:                    getEnv("PORT", "8080"),
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
		BrokerCredentialsKey:    strings.TrimSpace(os.Getenv("BROKER_CREDENTIALS_ENCRYPTION_KEY")),
		BrokerLongPortOAuth:     loadBrokerOAuthConfig("BROKER_LONGPORT"),
		BrokerTigerOAuth:        loadBrokerOAuthConfig("BROKER_TIGER"),
		BrokerFutuGateway: BrokerGatewayConfig{
			Host: getEnv("BROKER_FUTU_HOST", "127.0.0.1"),
			Port: getEnv("BROKER_FUTU_PORT", "11111"),
		},
		BrokerIBKRGateway: BrokerGatewayConfig{
			Host:     getEnv("BROKER_IBKR_HOST", "127.0.0.1"),
			Port:     getEnv("BROKER_IBKR_PORT", "7497"),
			ClientID: getEnv("BROKER_IBKR_CLIENT_ID", "9901"),
		},
		AllowedOrigins: splitOrigins(getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		RequestTimeout: time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 20)) * time.Second,
		ScanDelay:      time.Duration(getEnvInt("SCAN_DELAY_MS", 80)) * time.Millisecond,
	}
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
