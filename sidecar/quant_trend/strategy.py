"""Causal trend features and a transparent consensus classifier."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import date
import math
from typing import Any

import pandas as pd

from .config import DEFAULT_STRATEGY_CONFIG, TrendStrategyConfig
from .data import InsufficientDataError, PriceDataError, normalize_price_frame


VALID_ACTIONS = ("BUY", "HOLD", "SELL")


@dataclass(frozen=True)
class FactorState:
    as_of_index: int
    as_of_date: date
    close: float
    tsm_1m: float
    tsm_3m: float
    tsm_12m: float
    absolute_momentum_12_2: float
    fast_ma: float
    slow_ma: float
    fast_ma_slope: float
    price_to_52w_high: float
    atr_percent: float
    relative_momentum_percentile: float | None = None
    benchmark_momentum_12_2: float | None = None


def _safe_float(value: float | None) -> float | None:
    if value is None or not math.isfinite(float(value)):
        return None
    return round(float(value), 8)


class TrendStrategy:
    """Canonical momentum direction plus separately reported confirmations."""

    def __init__(self, config: TrendStrategyConfig = DEFAULT_STRATEGY_CONFIG) -> None:
        self.config = config

    def resolve_as_of_index(self, frame: pd.DataFrame, as_of: date) -> int:
        normalized = normalize_price_frame(frame, symbol="frame")
        eligible = normalized.index[normalized.index.date <= as_of]
        if len(eligible) == 0:
            raise PriceDataError(f"no price row exists on or before {as_of.isoformat()}")
        position = normalized.index.get_loc(eligible[-1])
        if not isinstance(position, int):
            raise PriceDataError("price index is not unique")
        return position

    def factors_at(
        self,
        frame: pd.DataFrame,
        index: int,
        *,
        relative_momentum_percentile: float | None = None,
        benchmark_momentum_12_2: float | None = None,
    ) -> FactorState:
        frame = normalize_price_frame(frame, symbol="frame")
        cfg = self.config
        if index < cfg.required_history_bars - 1 or index >= len(frame):
            raise InsufficientDataError(
                f"trend baseline needs {cfg.required_history_bars} completed bars before the decision date"
            )
        close = frame["close"]
        total_return_close = frame["total_return_close"]
        tsm_1m = float(total_return_close.iloc[index]) / float(
            total_return_close.iloc[index - cfg.tsm_short_days]
        ) - 1.0
        tsm_3m = float(total_return_close.iloc[index]) / float(
            total_return_close.iloc[index - cfg.tsm_medium_days]
        ) - 1.0
        tsm_12m = float(total_return_close.iloc[index]) / float(
            total_return_close.iloc[index - cfg.tsm_long_days]
        ) - 1.0
        momentum_start = float(total_return_close.iloc[index - cfg.momentum_lookback_days])
        momentum_end = float(total_return_close.iloc[index - cfg.momentum_skip_days])
        absolute_momentum = momentum_end / momentum_start - 1.0

        fast_ma = float(close.iloc[index - cfg.fast_ma_days + 1 : index + 1].mean())
        slow_ma = float(close.iloc[index - cfg.slow_ma_days + 1 : index + 1].mean())
        prior_fast_end = index - cfg.ma_slope_days
        prior_fast = float(
            close.iloc[prior_fast_end - cfg.fast_ma_days + 1 : prior_fast_end + 1].mean()
        )
        fast_slope = fast_ma / prior_fast - 1.0
        rolling_high = float(close.iloc[index - cfg.high_lookback_days + 1 : index + 1].max())
        current_close = float(close.iloc[index])

        true_ranges: list[float] = []
        for offset in range(index - cfg.atr_days + 1, index + 1):
            previous_close = float(close.iloc[offset - 1])
            high = float(frame["high"].iloc[offset])
            low = float(frame["low"].iloc[offset])
            true_ranges.append(max(high - low, abs(high - previous_close), abs(low - previous_close)))
        atr_percent = sum(true_ranges) / len(true_ranges) / current_close

        return FactorState(
            as_of_index=index,
            as_of_date=frame.index[index].date(),
            close=current_close,
            tsm_1m=tsm_1m,
            tsm_3m=tsm_3m,
            tsm_12m=tsm_12m,
            absolute_momentum_12_2=absolute_momentum,
            fast_ma=fast_ma,
            slow_ma=slow_ma,
            fast_ma_slope=fast_slope,
            price_to_52w_high=current_close / rolling_high,
            atr_percent=atr_percent,
            relative_momentum_percentile=relative_momentum_percentile,
            benchmark_momentum_12_2=benchmark_momentum_12_2,
        )

    def classify(self, factors: FactorState) -> dict[str, Any]:
        cfg = self.config
        tsm_returns = (factors.tsm_1m, factors.tsm_3m, factors.tsm_12m)
        tsm_actions = tuple("BUY" if value > 0 else ("SELL" if value < 0 else "HOLD") for value in tsm_returns)
        tsm_votes = tuple(1 if value > 0 else (-1 if value < 0 else 0) for value in tsm_returns)
        vote_sum = sum(tsm_votes)
        action = "BUY" if vote_sum > 0 else ("SELL" if vote_sum < 0 else "HOLD")
        momentum_12_2_sign = "BUY" if factors.absolute_momentum_12_2 > 0 else (
            "SELL" if factors.absolute_momentum_12_2 < 0 else "HOLD"
        )
        ma_bull = (
            factors.close > factors.slow_ma
            and factors.fast_ma > factors.slow_ma
            and factors.fast_ma_slope > 0
        )
        ma_bear = (
            factors.close < factors.slow_ma
            and factors.fast_ma < factors.slow_ma
            and factors.fast_ma_slope < 0
        )
        ma_action = "BUY" if ma_bull else ("SELL" if ma_bear else "HOLD")
        # Hurst-style daily adaptation: equal-weight signs at 21/63/252
        # sessions. The original paper uses month-end excess returns, market
        # risk scaling and a multi-asset portfolio; this classifier does not
        # claim to reproduce that portfolio.

        high_confirmation = (
            "BUY"
            if factors.price_to_52w_high >= cfg.high_confirmation_ratio
            else ("SELL" if factors.price_to_52w_high <= cfg.low_confirmation_ratio else "HOLD")
        )
        relative_confirmation = "UNAVAILABLE"
        percentile = factors.relative_momentum_percentile
        if percentile is not None:
            relative_confirmation = (
                "BUY"
                if percentile >= cfg.relative_winner_percentile
                else ("SELL" if percentile <= cfg.relative_loser_percentile else "HOLD")
            )

        confirmations = [ma_action, high_confirmation]
        if relative_confirmation != "UNAVAILABLE":
            confirmations.append(relative_confirmation)
        confirming_count = sum(1 for item in confirmations if item == action)
        contradicting_count = sum(
            1 for item in confirmations if item in {"BUY", "SELL"} and action in {"BUY", "SELL"} and item != action
        )
        signal_strength = "LOW"
        if action in {"BUY", "SELL"}:
            unanimous_tsm = abs(vote_sum) == 3
            if unanimous_tsm and confirming_count >= 2 and contradicting_count == 0:
                signal_strength = "HIGH"
            elif confirming_count >= 1 and contradicting_count <= 1:
                signal_strength = "MEDIUM"

        reasons = [
            (
                f"1/3/12-month time-series momentum is "
                f"{factors.tsm_1m:+.2%}/{factors.tsm_3m:+.2%}/{factors.tsm_12m:+.2%} "
                f"({','.join(tsm_actions)})"
            ),
            f"12-2 own return is {factors.absolute_momentum_12_2:+.2%} ({momentum_12_2_sign})",
            (
                f"MA regime is {ma_action}: close={factors.close:.2f}, "
                f"MA{cfg.fast_ma_days}={factors.fast_ma:.2f}, MA{cfg.slow_ma_days}={factors.slow_ma:.2f}, "
                f"fast-MA slope={factors.fast_ma_slope:+.2%}"
            ),
            f"price is {factors.price_to_52w_high:.1%} of its 52-week high ({high_confirmation})",
        ]
        if percentile is not None:
            reasons.append(f"cross-sectional momentum percentile is {percentile:.1%} ({relative_confirmation})")

        return {
            "action": action,
            "trend_score": round(vote_sum / 3.0, 8),
            "signal_strength": signal_strength,
            "decision_rule": "majority_sign_of_21_63_252_session_total_returns",
            "component_actions": {
                "tsm_1m": tsm_actions[0],
                "tsm_3m": tsm_actions[1],
                "tsm_12m": tsm_actions[2],
                "momentum_12_2_sign": momentum_12_2_sign,
                "moving_average_regime": ma_action,
                "52_week_high_confirmation": high_confirmation,
                "cross_sectional_confirmation": relative_confirmation,
            },
            "reasons": reasons,
        }

    def predict_at(
        self,
        frame: pd.DataFrame,
        index: int,
        *,
        relative_momentum_percentile: float | None = None,
        benchmark_momentum_12_2: float | None = None,
    ) -> dict[str, Any]:
        factors = self.factors_at(
            frame,
            index,
            relative_momentum_percentile=relative_momentum_percentile,
            benchmark_momentum_12_2=benchmark_momentum_12_2,
        )
        decision = self.classify(factors)
        return {
            **decision,
            "as_of_date": factors.as_of_date.isoformat(),
            "factors": {
                "close": _safe_float(factors.close),
                "tsm_1m": _safe_float(factors.tsm_1m),
                "tsm_3m": _safe_float(factors.tsm_3m),
                "tsm_12m": _safe_float(factors.tsm_12m),
                "absolute_momentum_12_2": _safe_float(factors.absolute_momentum_12_2),
                "fast_ma": _safe_float(factors.fast_ma),
                "slow_ma": _safe_float(factors.slow_ma),
                "fast_ma_slope": _safe_float(factors.fast_ma_slope),
                "price_to_52w_high": _safe_float(factors.price_to_52w_high),
                "atr_percent": _safe_float(factors.atr_percent),
                "relative_momentum_percentile": _safe_float(factors.relative_momentum_percentile),
                "benchmark_momentum_12_2": _safe_float(factors.benchmark_momentum_12_2),
            },
        }

    def momentum_at(self, frame: pd.DataFrame, index: int) -> float:
        return self.factors_at(frame, index).absolute_momentum_12_2
