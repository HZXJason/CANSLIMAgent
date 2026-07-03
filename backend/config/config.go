package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	FMPAPIKey              string
	PolygonAPIKey          string
	EnableFreeUniverse     bool
	EnableSECData          bool
	SECUserAgent           string
	EnableStooqData        bool
	EnableYahooFallback    bool
	EnableYFinanceFallback bool
	YFinancePython         string
	AIStreamURL            string
	AllowedOrigins         []string
	RequestTimeout         time.Duration
	ScanDelay              time.Duration
}

func Load() Config {
	_ = godotenv.Load(".env", "backend/.env")

	return Config{
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		FMPAPIKey:              os.Getenv("FMP_API_KEY"),
		PolygonAPIKey:          os.Getenv("POLYGON_API_KEY"),
		EnableFreeUniverse:     getEnvBool("ENABLE_FREE_UNIVERSE", true),
		EnableSECData:          getEnvBool("ENABLE_SEC_DATA", true),
		SECUserAgent:           getEnv("SEC_USER_AGENT", "CANSLIMAgent/1.0 (local research; set SEC_USER_AGENT for contact)"),
		EnableStooqData:        getEnvBool("ENABLE_STOOQ_DATA", true),
		EnableYahooFallback:    getEnvBool("ENABLE_YAHOO_FALLBACK", true),
		EnableYFinanceFallback: getEnvBool("ENABLE_YFINANCE_FALLBACK", true),
		YFinancePython:         getEnv("YFINANCE_PYTHON", "python"),
		AIStreamURL:            os.Getenv("AI_STREAM_URL"),
		AllowedOrigins:         splitOrigins(getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		RequestTimeout:         time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 20)) * time.Second,
		ScanDelay:              time.Duration(getEnvInt("SCAN_DELAY_MS", 80)) * time.Millisecond,
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
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
