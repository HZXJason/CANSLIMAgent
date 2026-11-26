package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	RegimeConfirmedUptrend     = "confirmed_uptrend"
	RegimeUptrendUnderPressure = "uptrend_under_pressure"
	RegimeRallyAttempt         = "rally_attempt"
	RegimeCorrection           = "correction"
	RegimeUnavailable          = "unavailable"
	RegimeInsufficientData     = "insufficient_data"
)

type CandleFetcher interface {
	KLine(ctx context.Context, symbol, period string) (KLineData, error)
}

type IndexRegime struct {
	Symbol           string   `json:"symbol"`
	Name             string   `json:"name"`
	Status           string   `json:"status"`
	Regime           string   `json:"regime"`
	RegimeLabel      string   `json:"regime_label"`
	MaxExposurePct   int      `json:"max_exposure_percent"`
	Close            *float64 `json:"close"`
	EMA21            *float64 `json:"ema_21"`
	SMA50            *float64 `json:"sma_50"`
	SMA200           *float64 `json:"sma_200"`
	ATR14Percent     *float64 `json:"atr_14_percent"`
	DistributionDays int      `json:"distribution_days"`
	Above21EMA       *bool    `json:"above_21ema"`
	Above50SMA       *bool    `json:"above_50sma"`
	Above200SMA      *bool    `json:"above_200sma"`
	DataAsOf         string   `json:"data_as_of"`
	Source           string   `json:"source"`
	Error            string   `json:"error,omitempty"`
}

type MarketRegimeReport struct {
	Market           string        `json:"market"`
	Status           string        `json:"status"`
	Regime           string        `json:"regime"`
	RegimeLabel      string        `json:"regime_label"`
	MaxExposurePct   int           `json:"max_exposure_percent"`
	DistributionDays int           `json:"distribution_days"`
	Guidance         string        `json:"guidance"`
	DataAsOf         string        `json:"data_as_of"`
	Indices          []IndexRegime `json:"indices"`
}

type MarketRegimeService struct {
	fetcher CandleFetcher
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	cache   map[string]cachedMarketRegime
}

type cachedMarketRegime struct {
	report    MarketRegimeReport
	expiresAt time.Time
}

func NewMarketRegimeService(fetcher CandleFetcher) *MarketRegimeService {
	return &MarketRegimeService{
		fetcher: fetcher,
		ttl:     5 * time.Minute,
		now:     time.Now,
		cache:   make(map[string]cachedMarketRegime),
	}
}

func (s *MarketRegimeService) Get(ctx context.Context, market string) MarketRegimeReport {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market != "CN" {
		market = "US"
	}

	s.mu.Lock()
	if cached, ok := s.cache[market]; ok && s.now().Before(cached.expiresAt) {
		s.mu.Unlock()
		return cached.report
	}
	s.mu.Unlock()

	type benchmarkSpec struct {
		symbol string
		name   string
	}
	specs := []benchmarkSpec{
		{symbol: "SPY", name: "标普 500 ETF"},
		{symbol: "QQQ", name: "纳斯达克 100 ETF"},
	}
	if market == "CN" {
		specs = []benchmarkSpec{
			{symbol: "000300.SH", name: "沪深 300"},
			{symbol: "399006.SZ", name: "创业板指"},
		}
	}

	indices := make([]IndexRegime, 0, len(specs))
	for _, spec := range specs {
		if s.fetcher == nil {
			indices = append(indices, IndexRegime{
				Symbol: spec.symbol, Name: spec.name, Status: RegimeUnavailable,
				Regime: RegimeUnavailable, RegimeLabel: regimeLabel(RegimeUnavailable),
				Error: "行情数据源未初始化",
			})
			continue
		}
		kline, err := s.fetcher.KLine(ctx, spec.symbol, PeriodDaily)
		if err != nil {
			indices = append(indices, IndexRegime{
				Symbol: spec.symbol, Name: spec.name, Status: RegimeUnavailable,
				Regime: RegimeUnavailable, RegimeLabel: regimeLabel(RegimeUnavailable),
				Error: err.Error(),
			})
			continue
		}
		regime := ComputeIndexRegime(spec.symbol, spec.name, kline.Candles)
		regime.Source = kline.Source
		if regime.DataAsOf == "" {
			regime.DataAsOf = kline.DataAsOf
		}
		indices = append(indices, regime)
	}

	report := aggregateMarketRegime(market, indices)

	s.mu.Lock()
	s.cache[market] = cachedMarketRegime{report: report, expiresAt: s.now().Add(s.ttl)}
	s.mu.Unlock()

	return report
}

func ComputeIndexRegime(symbol, name string, rawCandles []Candle) IndexRegime {
	result := IndexRegime{
		Symbol:      symbol,
		Name:        name,
		Status:      RegimeInsufficientData,
		Regime:      RegimeInsufficientData,
		RegimeLabel: regimeLabel(RegimeInsufficientData),
	}
	candles := make([]Candle, 0, len(rawCandles))
	for _, candle := range rawCandles {
		if candle.Close > 0 && candle.Date != "" {
			candles = append(candles, candle)
		}
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].Date < candles[j].Date })
	if len(candles) < 50 {
		if len(candles) > 0 {
			last := candles[len(candles)-1]
			closeVal := round2(last.Close)
			result.Close = &closeVal
			result.DataAsOf = last.Date
		}
		return result
	}

	last := candles[len(candles)-1]
	closeVal := round2(last.Close)
	ema21 := round2(computeEMA(candles, 21))
	sma50 := round2(computeSMA(candles, 50))
	above21 := last.Close >= ema21
	above50 := last.Close >= sma50
	distDays := countDistributionDays(candles, 25)

	result.Status = "ok"
	result.DataAsOf = last.Date
	result.Close = &closeVal
	result.EMA21 = &ema21
	result.SMA50 = &sma50
	result.Above21EMA = &above21
	result.Above50SMA = &above50
	result.DistributionDays = distDays

	if len(candles) >= 200 {
		sma200 := round2(computeSMA(candles, 200))
		above200 := last.Close >= sma200
		result.SMA200 = &sma200
		result.Above200SMA = &above200
	}
	if atr := ComputeATR14(candles); atr > 0 && last.Close > 0 {
		atrPct := round2(atr / last.Close * 100)
		result.ATR14Percent = &atrPct
	}

	switch {
	case last.Close > sma50 && ema21 >= sma50 && distDays <= 4:
		result.Regime = RegimeConfirmedUptrend
		result.MaxExposurePct = 100
	case (last.Close > sma50 && distDays >= 5) || (last.Close > sma50 && ema21 < sma50):
		result.Regime = RegimeUptrendUnderPressure
		result.MaxExposurePct = 50
	case last.Close <= sma50 && last.Close > ema21:
		result.Regime = RegimeRallyAttempt
		result.MaxExposurePct = 25
	default:
		result.Regime = RegimeCorrection
		result.MaxExposurePct = 0
	}
	result.RegimeLabel = regimeLabel(result.Regime)
	return result
}

func ComputeATR14(rawCandles []Candle) float64 {
	candles := make([]Candle, 0, len(rawCandles))
	for _, candle := range rawCandles {
		if candle.Close > 0 && candle.High > 0 && candle.Low > 0 {
			candles = append(candles, candle)
		}
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].Date < candles[j].Date })
	if len(candles) < 15 {
		return 0
	}
	trs := make([]float64, 0, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		prevClose := candles[i-1].Close
		highLow := candles[i].High - candles[i].Low
		highPrev := math.Abs(candles[i].High - prevClose)
		lowPrev := math.Abs(candles[i].Low - prevClose)
		tr := math.Max(highLow, math.Max(highPrev, lowPrev))
		trs = append(trs, tr)
	}
	if len(trs) < 14 {
		return 0
	}
	var sum float64
	for i := len(trs) - 14; i < len(trs); i++ {
		sum += trs[i]
	}
	return sum / 14.0
}

func computeSMA(candles []Candle, period int) float64 {
	if len(candles) < period || period <= 0 {
		return 0
	}
	var sum float64
	for i := len(candles) - period; i < len(candles); i++ {
		sum += candles[i].Close
	}
	return sum / float64(period)
}

func computeEMA(candles []Candle, period int) float64 {
	if len(candles) < period || period <= 0 {
		return 0
	}
	k := 2.0 / float64(period+1)
	ema := computeSMA(candles[:period], period)
	for i := period; i < len(candles); i++ {
		ema = candles[i].Close*k + ema*(1-k)
	}
	return ema
}

// countDistributionDays implements O'Neil's trailing 25-session distribution count:
// close drop >= 0.2% on higher volume than prior session, invalidated if index rallies >= 5% above that close.
func countDistributionDays(candles []Candle, window int) int {
	if len(candles) < 2 {
		return 0
	}
	start := len(candles) - window
	if start < 1 {
		start = 1
	}
	count := 0
	for i := start; i < len(candles); i++ {
		prev := candles[i-1]
		curr := candles[i]
		if prev.Close <= 0 || prev.Volume <= 0 || curr.Volume <= 0 {
			continue
		}
		changePct := (curr.Close - prev.Close) / prev.Close
		if changePct > -0.002 || curr.Volume <= prev.Volume {
			continue
		}
		invalidated := false
		for j := i + 1; j < len(candles); j++ {
			if (candles[j].Close-curr.Close)/curr.Close >= 0.05 {
				invalidated = true
				break
			}
		}
		if !invalidated {
			count++
		}
	}
	return count
}

func aggregateMarketRegime(market string, indices []IndexRegime) MarketRegimeReport {
	valid := make([]IndexRegime, 0, len(indices))
	var latestDate string
	for _, idx := range indices {
		if idx.DataAsOf > latestDate {
			latestDate = idx.DataAsOf
		}
		if idx.Status == "ok" {
			valid = append(valid, idx)
		}
	}
	if len(valid) == 0 {
		status := RegimeUnavailable
		for _, idx := range indices {
			if idx.Status == RegimeInsufficientData {
				status = RegimeInsufficientData
			}
		}
		return MarketRegimeReport{
			Market:         market,
			Status:         status,
			Regime:         status,
			RegimeLabel:    regimeLabel(status),
			MaxExposurePct: 0,
			Guidance:       "基准指数日线数据暂不可用，当前无法验证大盘环境（M），建议人工核对指数趋势后再行动。",
			DataAsOf:       latestDate,
			Indices:        indices,
		}
	}

	conservative := valid[0]
	maxDist := valid[0].DistributionDays
	for _, idx := range valid[1:] {
		if idx.DistributionDays > maxDist {
			maxDist = idx.DistributionDays
		}
		if idx.MaxExposurePct < conservative.MaxExposurePct {
			conservative = idx
		}
	}

	return MarketRegimeReport{
		Market:           market,
		Status:           "ok",
		Regime:           conservative.Regime,
		RegimeLabel:      conservative.RegimeLabel,
		MaxExposurePct:   conservative.MaxExposurePct,
		DistributionDays: maxDist,
		Guidance:         regimeGuidance(conservative.Regime),
		DataAsOf:         latestDate,
		Indices:          indices,
	}
}

func regimeLabel(regime string) string {
	switch regime {
	case RegimeConfirmedUptrend:
		return "确认上升趋势"
	case RegimeUptrendUnderPressure:
		return "上升趋势受压"
	case RegimeRallyAttempt:
		return "震荡尝试反弹"
	case RegimeCorrection:
		return "下跌回调期"
	case RegimeInsufficientData:
		return "指数样本不足"
	default:
		return "大盘数据暂不可用"
	}
}

func regimeGuidance(regime string) string {
	switch regime {
	case RegimeConfirmedUptrend:
		return "主指数站稳 50SMA 且派发日可控，可按标准 1R 仓位执行高质量突破剧本。"
	case RegimeUptrendUnderPressure:
		return "派发日累积或短期均线走弱，建议压缩总仓位至 50% 以内，只做高确信 0.5R 试错。"
	case RegimeRallyAttempt:
		return "指数仍处 50SMA 下方反弹试探，建议总仓位控制在 25% 以内并紧守止损。"
	case RegimeCorrection:
		return "指数处于下跌回调期，建议总仓位上限 0%，严控新建突破多单。"
	default:
		return "先核查数据源状态再决定敞口。"
	}
}
