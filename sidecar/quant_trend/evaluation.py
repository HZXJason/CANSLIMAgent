"""Causal historical evaluation for the deterministic trend baseline."""

from __future__ import annotations

from collections import Counter
from datetime import date
import math
import statistics
from typing import Any

import pandas as pd

from .config import DEFAULT_STRATEGY_CONFIG, TrendStrategyConfig
from .data import InsufficientDataError, PriceDataError, normalize_price_frame
from .strategy import TrendStrategy, VALID_ACTIONS


def _round(value: float | None, digits: int = 8) -> float | None:
    if value is None or not math.isfinite(float(value)):
        return None
    return round(float(value), digits)


def _barrier(config: TrendStrategyConfig, atr_percent: float, horizon_days: int) -> float:
    horizon_scale = math.sqrt(max(1.0, horizon_days / 20.0))
    return max(config.minimum_barrier, config.barrier_atr_multiple * atr_percent) * horizon_scale


def _score_future_path(
    frame: pd.DataFrame,
    *,
    index: int,
    horizon_days: int,
    atr_percent: float,
    config: TrendStrategyConfig,
) -> dict[str, Any]:
    """Label a node with symmetric first-hit and terminal-return definitions."""

    entry_index = index + 1
    end_index = index + horizon_days
    if entry_index >= len(frame) or end_index >= len(frame):
        raise PriceDataError("future price window is incomplete")
    window = frame.iloc[entry_index : end_index + 1]
    if len(window) != horizon_days:
        raise PriceDataError("future price window is incomplete")
    entry_open = float(window.iloc[0]["open"])
    entry_close = float(window.iloc[0]["close"])
    entry_total_return_close = float(window.iloc[0]["total_return_close"])
    if entry_open <= 0 or entry_close <= 0 or entry_total_return_close <= 0:
        raise PriceDataError("next-session entry open is invalid")
    # Yahoo's adjusted close is a total-return index. Apply the entry day's
    # adjustment ratio to its raw open so endpoint P&L includes splits and
    # distributions while path labels remain based on observable raw OHLC.
    total_return_entry_open = entry_open * entry_total_return_close / entry_close
    if total_return_entry_open <= 0:
        raise PriceDataError("next-session total-return entry open is invalid")

    barrier = _barrier(config, atr_percent, horizon_days)
    path_label = "HOLD"
    first_hit = "none"
    hit_day: int | None = None
    hit_date: str | None = None
    ambiguous = False
    for offset, (timestamp, row) in enumerate(window.iterrows(), start=1):
        up_hit = float(row["high"]) / entry_open - 1.0 >= barrier
        down_hit = 1.0 - float(row["low"]) / entry_open >= barrier
        if up_hit and down_hit:
            path_label = "AMBIGUOUS"
            first_hit = "both_same_day"
            hit_day = offset
            hit_date = timestamp.date().isoformat()
            ambiguous = True
            break
        if up_hit:
            path_label = "BUY"
            first_hit = "upper"
            hit_day = offset
            hit_date = timestamp.date().isoformat()
            break
        if down_hit:
            path_label = "SELL"
            first_hit = "lower"
            hit_day = offset
            hit_date = timestamp.date().isoformat()
            break

    endpoint_return = float(window.iloc[-1]["close"]) / entry_open - 1.0
    endpoint_total_return = (
        float(window.iloc[-1]["total_return_close"]) / total_return_entry_open - 1.0
    )
    neutral_band = config.neutral_return_band * math.sqrt(max(1.0, horizon_days / 20.0))
    trend_label = "BUY" if endpoint_return >= neutral_band else (
        "SELL" if endpoint_return <= -neutral_band else "HOLD"
    )
    endpoint_barrier_label = "BUY" if endpoint_return >= barrier else (
        "SELL" if endpoint_return <= -barrier else "HOLD"
    )
    return {
        "entry_date": window.index[0].date().isoformat(),
        "entry_open": _round(entry_open),
        "total_return_entry_open": _round(total_return_entry_open),
        "outcome_date": window.index[-1].date().isoformat(),
        "barrier": _round(barrier),
        "neutral_return_band": _round(neutral_band),
        "path_label": path_label,
        "trend_label": trend_label,
        "endpoint_barrier_label": endpoint_barrier_label,
        "first_hit": first_hit,
        "barrier_hit_day": hit_day,
        "barrier_hit_date": hit_date,
        "ambiguous": ambiguous,
        "endpoint_return": _round(endpoint_return),
        "endpoint_total_return": _round(endpoint_total_return),
        "maximum_favorable_move": _round(float(window["high"].max()) / entry_open - 1.0),
        "maximum_adverse_move": _round(1.0 - float(window["low"].min()) / entry_open),
    }


def _wilson_interval(correct: int, total: int, z: float = 1.959963984540054) -> list[float] | None:
    if total <= 0:
        return None
    proportion = correct / total
    denominator = 1.0 + z * z / total
    centre = proportion + z * z / (2.0 * total)
    margin = z * math.sqrt(proportion * (1.0 - proportion) / total + z * z / (4.0 * total * total))
    return [round((centre - margin) / denominator, 6), round((centre + margin) / denominator, 6)]


def _classification_metrics(nodes: list[dict[str, Any]], label_key: str) -> dict[str, Any]:
    scored = [node for node in nodes if node[label_key] in VALID_ACTIONS]
    total = len(scored)
    correct = sum(node["prediction"] == node[label_key] for node in scored)
    directional = [node for node in scored if node["prediction"] in {"BUY", "SELL"}]
    directional_correct = sum(node["prediction"] == node[label_key] for node in directional)

    confusion = {
        predicted: {actual: 0 for actual in VALID_ACTIONS}
        for predicted in VALID_ACTIONS
    }
    for node in scored:
        confusion[node["prediction"]][node[label_key]] += 1

    per_class: dict[str, dict[str, Any]] = {}
    recalls: list[float] = []
    for action in VALID_ACTIONS:
        support = sum(node[label_key] == action for node in scored)
        predicted_count = sum(node["prediction"] == action for node in scored)
        true_positive = sum(
            node["prediction"] == action and node[label_key] == action for node in scored
        )
        recall = true_positive / support if support else None
        precision = true_positive / predicted_count if predicted_count else None
        if recall is not None:
            recalls.append(recall)
        per_class[action] = {
            "support": support,
            "predicted": predicted_count,
            "precision": _round(precision, 6),
            "recall": _round(recall, 6),
        }

    constants = {}
    for action in VALID_ACTIONS:
        baseline_correct = sum(node[label_key] == action for node in scored)
        constants[action] = {
            "correct": baseline_correct,
            "accuracy": _round(baseline_correct / total if total else None, 6),
        }

    return {
        "scored_nodes": total,
        "correct_nodes": correct,
        "accuracy": _round(correct / total if total else None, 6),
        "accuracy_95pct_wilson": _wilson_interval(correct, total),
        "balanced_accuracy": _round(statistics.fmean(recalls) if recalls else None, 6),
        "directional_nodes": len(directional),
        "directional_coverage": _round(len(directional) / total if total else None, 6),
        "directional_correct_nodes": directional_correct,
        "directional_accuracy": _round(
            directional_correct / len(directional) if directional else None, 6
        ),
        "actual_distribution": dict(Counter(node[label_key] for node in scored)),
        "prediction_distribution": dict(Counter(node["prediction"] for node in scored)),
        "per_class": per_class,
        "confusion_matrix": confusion,
        "constant_action_baselines": constants,
    }


def _strategy_performance(
    nodes: list[dict[str, Any]],
    transaction_cost_bps: float,
    *,
    forced_action: str | None = None,
    non_overlapping: bool = True,
) -> dict[str, Any]:
    cost = max(0.0, transaction_cost_bps) / 10_000.0
    returns: list[float] = []
    active_returns: list[float] = []
    equity = 1.0
    peak = 1.0
    maximum_drawdown = 0.0
    liquidated = False
    liquidation_as_of_date: str | None = None
    for node in nodes:
        if liquidated:
            break
        if forced_action is None:
            position = max(-1.0, min(1.0, float(node["trend_score"])))
        else:
            position = {"BUY": 1.0, "HOLD": 0.0, "SELL": -1.0}[forced_action]
        total_return = float(node["endpoint_total_return"])
        net_return = position * total_return - cost * abs(position)
        # A cash account cannot compound through negative equity. This mainly
        # protects the full-size constant-SELL baseline when a stock more than
        # doubles inside one holding window.
        effective_return = max(-1.0, net_return)
        returns.append(effective_return)
        if position != 0.0:
            active_returns.append(effective_return)
        equity *= 1.0 + effective_return
        peak = max(peak, equity)
        maximum_drawdown = min(maximum_drawdown, equity / peak - 1.0)
        if net_return <= -1.0:
            liquidated = True
            liquidation_as_of_date = str(node.get("as_of_date") or "") or None
    return {
        "non_overlapping_windows": non_overlapping,
        "return_basis": "total_return_close_from_next_session_open",
        "position_sizing": (
            "full_notional_constant_action"
            if forced_action is not None
            else "signed_tsm_vote_fraction"
        ),
        "transaction_cost_bps_per_full_notional_signal": transaction_cost_bps,
        "evaluated_nodes": len(nodes),
        "executed_nodes": len(returns),
        "liquidated": liquidated,
        "liquidation_as_of_date": liquidation_as_of_date,
        "total_compounded_return": _round(equity - 1.0, 6) if returns else None,
        "maximum_drawdown": _round(maximum_drawdown, 6) if returns else None,
        "mean_node_return": _round(statistics.fmean(returns), 6) if returns else None,
        "active_signals": len(active_returns),
        "active_hit_rate": (
            _round(sum(value > 0 for value in active_returns) / len(active_returns), 6)
            if active_returns
            else None
        ),
        "mean_active_return": (
            _round(statistics.fmean(active_returns), 6) if active_returns else None
        ),
    }


def _evenly_limit(indices: list[int], maximum: int | None) -> list[int]:
    if maximum is None or maximum <= 0 or len(indices) <= maximum:
        return indices
    if maximum == 1:
        return [indices[-1]]
    positions = {round(offset * (len(indices) - 1) / (maximum - 1)) for offset in range(maximum)}
    return [indices[position] for position in sorted(positions)]


def evaluate_trend_frame(
    frame: pd.DataFrame,
    *,
    symbol: str,
    start_date: date,
    end_date: date,
    horizon_days: int = 20,
    step_days: int | None = None,
    maximum_nodes: int | None = None,
    benchmark_frame: pd.DataFrame | None = None,
    config: TrendStrategyConfig = DEFAULT_STRATEGY_CONFIG,
    transaction_cost_bps: float = 10.0,
) -> dict[str, Any]:
    """Evaluate fixed rules on non-overlapping point-in-time nodes by default."""

    if end_date < start_date:
        raise ValueError("end_date must not precede start_date")
    if horizon_days < 5 or horizon_days > 252:
        raise ValueError("horizon_days must be between 5 and 252")
    step = horizon_days if step_days is None else step_days
    if step < 1:
        raise ValueError("step_days must be positive")
    if step < horizon_days:
        raise ValueError("step_days must be at least horizon_days so evaluation windows do not overlap")

    normalized = normalize_price_frame(frame, symbol=symbol)
    benchmark = normalize_price_frame(benchmark_frame, symbol="benchmark") if benchmark_frame is not None else None
    strategy = TrendStrategy(config)
    eligible = [
        index
        for index in range(config.required_history_bars - 1, len(normalized) - horizon_days)
        if start_date <= normalized.index[index].date() <= end_date
    ]
    selected = _evenly_limit(eligible[::step], maximum_nodes)
    if not selected:
        raise InsufficientDataError("no evaluation nodes have both warm-up and future data")

    nodes: list[dict[str, Any]] = []
    for index in selected:
        as_of = normalized.index[index].date()
        benchmark_momentum: float | None = None
        if benchmark is not None:
            benchmark_eligible = benchmark.index[benchmark.index.date <= as_of]
            if len(benchmark_eligible):
                benchmark_index = benchmark.index.get_loc(benchmark_eligible[-1])
                if isinstance(benchmark_index, int) and benchmark_index >= config.required_history_bars - 1:
                    benchmark_momentum = strategy.momentum_at(benchmark, benchmark_index)
        prediction = strategy.predict_at(
            normalized,
            index,
            benchmark_momentum_12_2=benchmark_momentum,
        )
        outcome = _score_future_path(
            normalized,
            index=index,
            horizon_days=horizon_days,
            atr_percent=float(prediction["factors"]["atr_percent"]),
            config=config,
        )
        nodes.append(
            {
                "as_of_date": as_of.isoformat(),
                "prediction": prediction["action"],
                "trend_score": prediction["trend_score"],
                "signal_strength": prediction["signal_strength"],
                "factors": prediction["factors"],
                "component_actions": prediction["component_actions"],
                **outcome,
                "path_correct": prediction["action"] == outcome["path_label"],
                "trend_correct": prediction["action"] == outcome["trend_label"],
                "endpoint_barrier_correct": prediction["action"] == outcome["endpoint_barrier_label"],
            }
        )

    return {
        "schema_version": "quant.trend_evaluation.v1",
        "strategy_version": config.strategy_version,
        "config_hash": config.config_hash(),
        "symbol": symbol.strip().upper(),
        "start_date": start_date.isoformat(),
        "end_date": end_date.isoformat(),
        "horizon_trading_days": horizon_days,
        "step_trading_days": step,
        "total_nodes": len(nodes),
        "ambiguous_nodes": sum(node["ambiguous"] for node in nodes),
        "trend_metrics": _classification_metrics(nodes, "trend_label"),
        "path_metrics": _classification_metrics(nodes, "path_label"),
        "endpoint_barrier_metrics": _classification_metrics(nodes, "endpoint_barrier_label"),
        # Compatibility alias for early v1 consumers. New code should use the
        # explicitly named endpoint_barrier_metrics.
        "endpoint_metrics": _classification_metrics(nodes, "endpoint_barrier_label"),
        "strategy_performance": _strategy_performance(
            nodes,
            transaction_cost_bps,
            non_overlapping=step >= horizon_days,
        ),
        "strategy_performance_baselines": {
            action: _strategy_performance(
                # Endpoint returns remain observable even when daily OHLC
                # cannot identify which path barrier was touched first.
                nodes,
                transaction_cost_bps,
                forced_action=action,
                non_overlapping=step >= horizon_days,
            )
            for action in VALID_ACTIONS
        },
        "nodes": nodes,
        "warnings": [
            "This is a single-symbol historical evaluation, not proof of cross-sectional factor profitability.",
            "Yahoo price history does not provide a survivorship-safe historical universe.",
            "The direction rule is a daily single-stock adaptation, not a replication of the original multi-market risk-scaled TSM portfolio.",
            "Classification labels use raw price OHLC, while performance uses total-return adjusted close from the next-session open.",
            "Wilson intervals are descriptive because market nodes remain dependent across regimes even when holding windows do not overlap.",
            "Performance drawdown is sampled only at holding-window endpoints, so intraperiod drawdown can be worse.",
            "No probability is emitted until a separate out-of-sample calibration is fitted.",
        ],
    }
