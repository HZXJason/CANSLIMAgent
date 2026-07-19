package models

import "testing"

func TestAshareMarketsAreValidAndClassified(t *testing.T) {
	markets := []string{MarketSSEMain, MarketSZSEMain, MarketSTAR, MarketChiNext, MarketBSE}
	for _, market := range markets {
		if !ValidMarket(market) {
			t.Fatalf("ValidMarket(%q) = false", market)
		}
		if !IsAshareMarket(market) {
			t.Fatalf("IsAshareMarket(%q) = false", market)
		}
	}
	if IsAshareMarket(MarketSP500) {
		t.Fatal("SP500 was classified as an A-share market")
	}
}
