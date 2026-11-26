package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type stubCandleFetcher struct {
	data map[string]KLineData
	err  error
}

func (f stubCandleFetcher) KLine(_ context.Context, symbol, _ string) (KLineData, error) {
	if f.err != nil {
		return KLineData{}, f.err
	}
	value, ok := f.data[symbol]
	if !ok {
		return KLineData{}, errors.New("missing stub symbol")
	}
	return value, nil
}

func makeTrendCandles(startClose, step float64, count int, distDayIndices map[int]bool) []Candle {
	candles := make([]Candle, 0, count)
	price := startClose
	for i := 0; i < count; i++ {
		vol := int64(1_000_000)
		if i > 0 {
			if distDayIndices[i] {
				price = price * 0.992
				vol = 1_800_000
			} else {
				price = price + step
			}
		}
		if price <= 1 {
			price = 1
		}
		candles = append(candles, Candle{
			Date:   fmt.Sprintf("2026-%02d-%02d", (i/28)+1, (i%28)+1),
			Open:   price * 0.995,
			High:   price * 1.01,
			Low:    price * 0.985,
			Close:  price,
			Volume: vol,
		})
	}
	return candles
}

func TestComputeIndexRegimeDetectsUptrendPressureAndCorrection(t *testing.T) {
	uptrend := ComputeIndexRegime("SPY", "S&P 500", makeTrendCandles(100, 0.5, 60, nil))
	if uptrend.Status != "ok" || uptrend.Regime != RegimeConfirmedUptrend || uptrend.MaxExposurePct != 100 {
		t.Fatalf("expected confirmed uptrend, got %+v", uptrend)
	}

	dist := map[int]bool{45: true, 47: true, 49: true, 51: true, 53: true}
	pressured := ComputeIndexRegime("QQQ", "Nasdaq 100", makeTrendCandles(100, 0.6, 60, dist))
	if pressured.Status != "ok" || pressured.Regime != RegimeUptrendUnderPressure || pressured.DistributionDays < 5 || pressured.MaxExposurePct != 50 {
		t.Fatalf("expected uptrend under pressure, got %+v", pressured)
	}

	correction := ComputeIndexRegime("000300.SH", "CSI 300", makeTrendCandles(200, -1.5, 60, nil))
	if correction.Status != "ok" || correction.Regime != RegimeCorrection || correction.MaxExposurePct != 0 {
		t.Fatalf("expected correction, got %+v", correction)
	}
}

func TestMarketRegimeServiceHonestDegradation(t *testing.T) {
	svc := NewMarketRegimeService(stubCandleFetcher{err: errors.New("upstream timeout")})
	report := svc.Get(context.Background(), "US")
	if report.Status != RegimeUnavailable || report.MaxExposurePct != 0 || len(report.Indices) != 2 {
		t.Fatalf("expected honest unavailable status, got %+v", report)
	}
}
