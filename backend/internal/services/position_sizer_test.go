package services

import "testing"

func TestCalculatePositionSizeUSAndAShareLots(t *testing.T) {
	atr := 4.0
	tp := 125.0
	us, err := CalculatePositionSize(PositionSizeInput{
		Symbol:                  "NVDA",
		Direction:               "buy",
		AccountEquity:           100000,
		StandardRiskPercent:     1.0,
		MaxPositionPercent:      20.0,
		MaxPortfolioHeatPercent: 5.0,
		CurrentOpenHeatAmount:   4500,
		RiskMultiple:            1.0,
		EntryPrice:              100,
		StopLoss:                95,
		TakeProfit:              &tp,
		ATR14:                   &atr,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1R = $1000, risk/share = $5 => 200 shares ($20,000 = 20%)
	if us.SuggestedQuantity != 200 || us.ActualRiskAmount != 1000 || us.ProjectedHeatPercent != 5.5 {
		t.Fatalf("unexpected US sizing: %+v", us)
	}
	if len(us.Warnings) == 0 || us.Warnings[len(us.Warnings)-1].Code != "portfolio_heat_exceeded" {
		t.Fatalf("expected portfolio_heat_exceeded warning, got %+v", us.Warnings)
	}

	cn, err := CalculatePositionSize(PositionSizeInput{
		Symbol:              "600519.SH",
		Direction:           "buy",
		AccountEquity:       500000,
		StandardRiskPercent: 1.0,
		MaxPositionPercent:  25.0,
		RiskMultiple:        1.0,
		EntryPrice:          100,
		StopLoss:            92,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1R = 5000 / 8 = 625 => rounded down to 600 shares (100-share board lot)
	if cn.MinLotSize != 100 || cn.SuggestedQuantity != 600 {
		t.Fatalf("unexpected A-share lot sizing: %+v", cn)
	}

	star, err := CalculatePositionSize(PositionSizeInput{
		Symbol:              "688981.SH",
		Direction:           "buy",
		AccountEquity:       200000,
		StandardRiskPercent: 1.0,
		MaxPositionPercent:  20.0,
		RiskMultiple:        1.0,
		EntryPrice:          50,
		StopLoss:            46,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1R = 2000 / 4 = 500 shares; STAR min lot 200, step 1 => 500 shares
	if star.MinLotSize != 200 || star.LotSize != 1 || star.SuggestedQuantity != 500 {
		t.Fatalf("unexpected STAR lot sizing: %+v", star)
	}
}
