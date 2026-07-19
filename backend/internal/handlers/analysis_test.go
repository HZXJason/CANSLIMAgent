package handlers

import (
	"strings"
	"testing"

	"github.com/alphalens/canslimagent/backend/internal/services"
)

func TestFormatRevenueSummaryUsesMarketCurrency(t *testing.T) {
	revenue := services.RevenueData{
		Source: "test",
		Years: []services.RevenueYear{{
			Year:             2025,
			Revenue:          12_345_000_000,
			RevenueYoYGrowth: 8.5,
		}},
	}

	ashare := formatRevenueSummary("600519.SH", revenue)
	if !strings.Contains(ashare, "123.45 亿元人民币") || strings.Contains(ashare, "美元") {
		t.Fatalf("A-share revenue summary = %q", ashare)
	}

	us := formatRevenueSummary("AAPL", revenue)
	if !strings.Contains(us, "123.45 亿美元") {
		t.Fatalf("US revenue summary = %q", us)
	}
}
