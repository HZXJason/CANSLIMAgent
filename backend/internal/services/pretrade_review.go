package services

import (
	"context"
	"math"
	"strings"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

type PreTradeAIReviewInput struct {
	Ticker               string
	Provider             string
	Model                string
	BackendURL           string
	APIKey               string
	PlanSnapshot         map[string]any
	ResearchEvidence     map[string]any
	CurrentEvidence      map[string]any
	BehaviorObservations []map[string]any
	TradingProfile       map[string]any
	TradingPrinciples    []map[string]any
	TradingPlaybooks     []map[string]any
	TradingPolicies      []map[string]any
	MarketRegime         map[string]any
	PositionSizing       map[string]any
	ContextVersion       string
}

type PreTradeEnvironment struct {
	MarketRegime         string
	ATR14                *float64
	OpenHeatPercent      *float64
	ProjectedHeatPercent *float64
	MaxHeatPercent       *float64
}

func BuildEnvironmentPreTradeReview(plan models.TradePlan, profile models.TradingProfile, env PreTradeEnvironment) DeterministicPreTradeReview {
	result := BuildPersonalizedPreTradeReview(plan, profile)
	if env.MarketRegime == "correction" && plan.Direction == models.TradePlanDirectionBuy {
		result.HardConflicts = append(result.HardConflicts, newPlanFinding(
			"market_regime_correction", "high", "大盘（M）处于下跌回调期，建议总仓位上限 0%，逆势做多风险极高。", "direction",
			map[string]any{"market_regime": env.MarketRegime, "source_type": "market_regime"},
		))
	} else if (env.MarketRegime == "uptrend_under_pressure" || env.MarketRegime == "rally_attempt") && plan.Direction == models.TradePlanDirectionBuy {
		result.RiskWarnings = append(result.RiskWarnings, newPlanFinding(
			"market_regime_caution", "medium", "大盘（M）处于上升受压或尝试反弹期，建议压缩单笔风险或分批试错。", "direction",
			map[string]any{"market_regime": env.MarketRegime, "source_type": "market_regime"},
		))
	}
	if env.ATR14 != nil && *env.ATR14 > 0 && plan.EntryPrice != nil && plan.StopLoss != nil {
		stopDist := math.Abs(*plan.EntryPrice - *plan.StopLoss)
		atrMult := stopDist / *env.ATR14
		if atrMult < 0.5 {
			result.RiskWarnings = append(result.RiskWarnings, newPlanFinding(
				"stop_too_tight_atr", "high", "止损距离小于 0.5 倍 ATR(14)，极易被日内正常波动扫出。", "stop_loss",
				map[string]any{"atr_14": *env.ATR14, "stop_atr_multiple": math.Round(atrMult*100) / 100},
			))
		}
	}
	if env.MaxHeatPercent != nil && *env.MaxHeatPercent > 0 && env.ProjectedHeatPercent != nil && *env.ProjectedHeatPercent > *env.MaxHeatPercent {
		result.HardConflicts = append(result.HardConflicts, newPlanFinding(
			"portfolio_heat_exceeded", "high", "叠加本单后组合在途总风险（Portfolio Heat）超出设定上限。", "position_size",
			map[string]any{"projected_heat_percent": *env.ProjectedHeatPercent, "max_heat_percent": *env.MaxHeatPercent},
		))
	}
	return result
}

func BuildPersonalizedPreTradeReview(plan models.TradePlan, profile models.TradingProfile) DeterministicPreTradeReview {
	result := BuildDeterministicPreTradeReview(plan)
	if !profile.Configured || profile.MaxRiskPerTradePercent == nil || plan.MaxRiskPercent == nil || *plan.MaxRiskPercent <= *profile.MaxRiskPerTradePercent {
		return result
	}
	result.HardConflicts = append(result.HardConflicts, newPlanFinding(
		"personal_risk_limit_exceeded", "high", "计划单笔风险超过你在个人交易系统中设置的上限。", "max_risk_percent",
		map[string]any{"plan_percent": *plan.MaxRiskPercent, "profile_limit_percent": *profile.MaxRiskPerTradePercent, "source_type": "trading_profile", "source_id": "singleton"},
	))
	return result
}

type PreTradeAIReviewOutput struct {
	RiskWarnings         []models.PreTradeFinding `json:"risk_warnings"`
	BehaviorReminders    []models.PreTradeFinding `json:"behavior_reminders"`
	UnresolvedQuestions  []models.PreTradeFinding `json:"unresolved_questions"`
	ChangesSinceResearch []models.PreTradeFinding `json:"changes_since_research"`
	Freshness            map[string]any           `json:"freshness"`
	Sources              []models.PreTradeSource  `json:"sources"`
}

type PreTradeAIReviewer interface {
	Review(context.Context, PreTradeAIReviewInput) (PreTradeAIReviewOutput, error)
}

type DeterministicPreTradeReview struct {
	HardConflicts       []models.PreTradeFinding
	RiskWarnings        []models.PreTradeFinding
	UnresolvedQuestions []models.PreTradeFinding
}

func BuildDeterministicPreTradeReview(plan models.TradePlan) DeterministicPreTradeReview {
	result := DeterministicPreTradeReview{
		HardConflicts:       []models.PreTradeFinding{},
		RiskWarnings:        []models.PreTradeFinding{},
		UnresolvedQuestions: []models.PreTradeFinding{},
	}
	addHard := func(code, message, field string, evidence map[string]any) {
		result.HardConflicts = append(result.HardConflicts, newPlanFinding(code, "high", message, field, evidence))
	}
	addWarning := func(code, severity, message, field string, evidence map[string]any) {
		result.RiskWarnings = append(result.RiskWarnings, newPlanFinding(code, severity, message, field, evidence))
	}

	if plan.ValidUntil != nil && plan.ValidUntil.Before(plan.PlanDate) {
		addHard("valid_until_before_plan_date", "有效期早于计划日期。", "valid_until", map[string]any{
			"plan_date": plan.PlanDate.Format("2006-01-02"), "valid_until": plan.ValidUntil.Format("2006-01-02"),
		})
	}
	if plan.StopLoss == nil {
		addHard("missing_stop_loss", "计划没有定义止损价，无法计算单笔价格风险。", "stop_loss", map[string]any{"value": nil})
	}
	if plan.EntryPrice != nil && plan.StopLoss != nil {
		switch plan.Direction {
		case models.TradePlanDirectionBuy:
			if *plan.StopLoss >= *plan.EntryPrice {
				addHard("buy_stop_not_below_entry", "买入计划的止损价必须低于计划入场价。", "stop_loss", priceEvidence(plan))
			}
		case models.TradePlanDirectionSell:
			if *plan.StopLoss <= *plan.EntryPrice {
				addHard("sell_stop_not_above_entry", "卖出计划的止损价必须高于计划入场价。", "stop_loss", priceEvidence(plan))
			}
		}
	}
	if plan.EntryPrice != nil && plan.TakeProfit != nil {
		switch plan.Direction {
		case models.TradePlanDirectionBuy:
			if *plan.TakeProfit <= *plan.EntryPrice {
				addHard("buy_target_not_above_entry", "买入计划的止盈价必须高于计划入场价。", "take_profit", priceEvidence(plan))
			}
		case models.TradePlanDirectionSell:
			if *plan.TakeProfit >= *plan.EntryPrice {
				addHard("sell_target_not_below_entry", "卖出计划的止盈价必须低于计划入场价。", "take_profit", priceEvidence(plan))
			}
		}
	}
	if plan.EntryPrice != nil && plan.StopLoss != nil && plan.TakeProfit != nil {
		risk := math.Abs(*plan.EntryPrice - *plan.StopLoss)
		reward := math.Abs(*plan.TakeProfit - *plan.EntryPrice)
		if risk > 0 {
			ratio := reward / risk
			if ratio < 1.5 {
				addWarning("reward_risk_below_1_5", "medium", "按计划价格计算的潜在收益风险比低于 1.5。", "take_profit", map[string]any{"reward_risk_ratio": ratio})
			}
		}
	}
	if plan.EntryTrigger == nil || strings.TrimSpace(*plan.EntryTrigger) == "" {
		addWarning("missing_entry_trigger", "medium", "尚未定义可观察的入场触发条件。", "entry_trigger", map[string]any{"value": nil})
	}
	if plan.Invalidation == nil || strings.TrimSpace(*plan.Invalidation) == "" {
		addWarning("missing_invalidation", "high", "尚未写明除止损价外的论点失效条件。", "invalidation", map[string]any{"value": nil})
	}
	if plan.MaxRiskAmount == nil && plan.MaxRiskPercent == nil {
		addWarning("missing_risk_budget", "high", "尚未设置最大风险金额或最大风险比例。", "max_risk_percent", map[string]any{"max_risk_amount": nil, "max_risk_percent": nil})
	}
	if plan.UserThesisID == nil {
		field := "user_thesis_id"
		result.UnresolvedQuestions = append(result.UnresolvedQuestions, models.PreTradeFinding{
			Code: "missing_user_thesis", Severity: "medium", Category: "unknown", Message: "该计划未关联已保存的用户论点，无法核对研究到计划的变化。", Field: &field, Evidence: map[string]any{"value": nil},
		})
	}
	return result
}

func newPlanFinding(code, severity, message, field string, evidence map[string]any) models.PreTradeFinding {
	fieldCopy := field
	return models.PreTradeFinding{Code: code, Severity: severity, Category: "deterministic", Message: message, Field: &fieldCopy, Evidence: evidence}
}

func priceEvidence(plan models.TradePlan) map[string]any {
	return map[string]any{"direction": plan.Direction, "entry_price": plan.EntryPrice, "stop_loss": plan.StopLoss, "take_profit": plan.TakeProfit}
}
