package handlers

import (
	"testing"
	"time"
)

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
