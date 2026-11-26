package services

import (
	"errors"
	"math"
	"strings"
)

type PositionSizeInput struct {
	Symbol                  string   `json:"symbol"`
	Direction               string   `json:"direction"`
	AccountEquity           float64  `json:"account_equity"`
	AccountCurrency         string   `json:"account_currency"`
	StandardRiskPercent     float64  `json:"standard_risk_percent"`
	MaxPositionPercent      float64  `json:"max_position_percent"`
	MaxPortfolioHeatPercent float64  `json:"max_portfolio_heat_percent"`
	CurrentOpenHeatAmount   float64  `json:"current_open_heat_amount"`
	RiskMultiple            float64  `json:"risk_multiple"`
	EntryPrice              float64  `json:"entry_price"`
	StopLoss                float64  `json:"stop_loss"`
	TakeProfit              *float64 `json:"take_profit"`
	ATR14                   *float64 `json:"atr_14"`
	MarketRegime            string   `json:"market_regime"`
}

type PositionSizeWarning struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type PositionSizeResult struct {
	Symbol               string                `json:"symbol"`
	LotSize              int                   `json:"lot_size"`
	MinLotSize           int                   `json:"min_lot_size"`
	OneRAmount           float64               `json:"one_r_amount"`
	TargetRiskAmount     float64               `json:"target_risk_amount"`
	PerShareRisk         float64               `json:"per_share_risk"`
	SuggestedQuantity    float64               `json:"suggested_quantity"`
	PositionValue        float64               `json:"position_value"`
	PositionPercent      float64               `json:"position_percent"`
	ActualRiskAmount     float64               `json:"actual_risk_amount"`
	ActualRiskPercent    float64               `json:"actual_risk_percent"`
	ActualRiskR          float64               `json:"actual_risk_r"`
	StopDistancePercent  float64               `json:"stop_distance_percent"`
	StopATRMultiple      *float64              `json:"stop_atr_multiple"`
	RewardRiskRatio      *float64              `json:"reward_risk_ratio"`
	CurrentOpenHeatPct   float64               `json:"current_open_heat_percent"`
	ProjectedHeatAmount  float64               `json:"projected_heat_amount"`
	ProjectedHeatPercent float64               `json:"projected_heat_percent"`
	Warnings             []PositionSizeWarning `json:"warnings"`
}

func MarketLotRules(symbol string) (minLot int, step int) {
	upper := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.HasPrefix(upper, "688") || strings.HasPrefix(upper, "689") {
		if strings.HasSuffix(upper, ".SH") || !strings.Contains(upper, ".") {
			return 200, 1
		}
	}
	if strings.HasSuffix(upper, ".SH") || strings.HasSuffix(upper, ".SZ") || strings.HasSuffix(upper, ".BJ") {
		return 100, 100
	}
	return 1, 1
}

func CalculatePositionSize(input PositionSizeInput) (PositionSizeResult, error) {
	symbol := strings.ToUpper(strings.TrimSpace(input.Symbol))
	if symbol == "" {
		return PositionSizeResult{}, errors.New("symbol is required")
	}
	direction := strings.ToLower(strings.TrimSpace(input.Direction))
	if direction == "" {
		direction = "buy"
	}
	if input.AccountEquity <= 0 {
		return PositionSizeResult{}, errors.New("account_equity must be positive")
	}
	if input.EntryPrice <= 0 || input.StopLoss <= 0 {
		return PositionSizeResult{}, errors.New("entry_price and stop_loss must be positive")
	}
	if direction == "buy" && input.StopLoss >= input.EntryPrice {
		return PositionSizeResult{}, errors.New("buy stop_loss must be below entry_price")
	}
	if direction == "sell" && input.StopLoss <= input.EntryPrice {
		return PositionSizeResult{}, errors.New("sell stop_loss must be above entry_price")
	}

	stdRiskPct := input.StandardRiskPercent
	if stdRiskPct <= 0 {
		stdRiskPct = 1.0
	}
	maxPosPct := input.MaxPositionPercent
	if maxPosPct <= 0 {
		maxPosPct = 20.0
	}
	maxHeatPct := input.MaxPortfolioHeatPercent
	if maxHeatPct <= 0 {
		maxHeatPct = 5.0
	}
	riskMult := input.RiskMultiple
	if riskMult <= 0 {
		riskMult = 1.0
	}

	oneR := input.AccountEquity * (stdRiskPct / 100.0)
	targetRisk := oneR * riskMult
	perShareRisk := math.Abs(input.EntryPrice - input.StopLoss)
	rawRiskShares := targetRisk / perShareRisk
	rawCapShares := (input.AccountEquity * (maxPosPct / 100.0)) / input.EntryPrice

	capped := rawCapShares < rawRiskShares
	rawShares := math.Min(rawRiskShares, rawCapShares)

	minLot, step := MarketLotRules(symbol)
	qty := 0.0
	if rawShares >= float64(minLot) {
		if step <= 1 {
			qty = math.Floor(rawShares)
		} else {
			qty = math.Floor(rawShares/float64(step)) * float64(step)
		}
	}

	posValue := qty * input.EntryPrice
	posPct := posValue / input.AccountEquity * 100.0
	actualRisk := qty * perShareRisk
	actualRiskPct := actualRisk / input.AccountEquity * 100.0
	actualRiskR := 0.0
	if oneR > 0 {
		actualRiskR = actualRisk / oneR
	}
	stopDistPct := perShareRisk / input.EntryPrice * 100.0
	currentHeatPct := input.CurrentOpenHeatAmount / input.AccountEquity * 100.0
	projectedHeat := input.CurrentOpenHeatAmount + actualRisk
	projectedHeatPct := projectedHeat / input.AccountEquity * 100.0

	warnings := []PositionSizeWarning{}
	if qty == 0 {
		warnings = append(warnings, PositionSizeWarning{
			Code: "lot_size_insufficient", Severity: "high",
			Message: "按当前 1R 风险预算不足以买入 1 手最小交易单位。",
		})
	}
	if capped {
		warnings = append(warnings, PositionSizeWarning{
			Code: "capped_by_concentration", Severity: "medium",
			Message: "建议股数已被单票最大仓位上限截断，实际风险低于目标 R。",
		})
	}
	var stopATRMult *float64
	if input.ATR14 != nil && *input.ATR14 > 0 {
		value := round2(perShareRisk / *input.ATR14)
		stopATRMult = &value
		if value < 0.5 {
			warnings = append(warnings, PositionSizeWarning{
				Code: "stop_too_tight_atr", Severity: "high",
				Message: "止损距离小于 0.5 倍 ATR(14)，极易被日内正常波动洗出。",
			})
		}
	}
	if stopDistPct > 8.0 {
		warnings = append(warnings, PositionSizeWarning{
			Code: "stop_too_wide_pct", Severity: "medium",
			Message: "止损幅度超过 8%，不符合紧凑枢轴风险控制原则。",
		})
	}
	var rr *float64
	if input.TakeProfit != nil && *input.TakeProfit > 0 {
		reward := math.Abs(*input.TakeProfit - input.EntryPrice)
		if direction == "buy" && *input.TakeProfit <= input.EntryPrice {
			reward = 0
		}
		if direction == "sell" && *input.TakeProfit >= input.EntryPrice {
			reward = 0
		}
		if perShareRisk > 0 {
			ratio := round2(reward / perShareRisk)
			rr = &ratio
			if ratio > 0 && ratio < 2.0 {
				warnings = append(warnings, PositionSizeWarning{
					Code: "reward_risk_below_2r", Severity: "medium",
					Message: "潜在盈亏比低于 2.0R，赔率优势不足。",
				})
			}
		}
	}
	if projectedHeatPct > maxHeatPct {
		warnings = append(warnings, PositionSizeWarning{
			Code: "portfolio_heat_exceeded", Severity: "high",
			Message: "加上本单后组合在途总风险（Portfolio Heat）超出账户上限，建议先将老仓止损移至保本位。",
		})
	}
	if input.MarketRegime == RegimeCorrection && direction == "buy" {
		warnings = append(warnings, PositionSizeWarning{
			Code: "market_regime_correction", Severity: "high",
			Message: "大盘（M）处于下跌回调期，建议总仓位上限 0%，逆势做多风险极高。",
		})
	}

	return PositionSizeResult{
		Symbol:               symbol,
		LotSize:              step,
		MinLotSize:           minLot,
		OneRAmount:           round2(oneR),
		TargetRiskAmount:     round2(targetRisk),
		PerShareRisk:         round2(perShareRisk),
		SuggestedQuantity:    qty,
		PositionValue:        round2(posValue),
		PositionPercent:      round2(posPct),
		ActualRiskAmount:     round2(actualRisk),
		ActualRiskPercent:    round2(actualRiskPct),
		ActualRiskR:          round2(actualRiskR),
		StopDistancePercent:  round2(stopDistPct),
		StopATRMultiple:      stopATRMult,
		RewardRiskRatio:      rr,
		CurrentOpenHeatPct:   round2(currentHeatPct),
		ProjectedHeatAmount:  round2(projectedHeat),
		ProjectedHeatPercent: round2(projectedHeatPct),
		Warnings:             warnings,
	}, nil
}
