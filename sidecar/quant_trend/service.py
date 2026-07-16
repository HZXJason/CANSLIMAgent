"""Application service for prediction, snapshots, and single-symbol evaluation."""

from __future__ import annotations

from datetime import date, timedelta
import math
from pathlib import Path
from typing import Any, Sequence

import pandas as pd

from .artifacts import QuantArtifactStore, frame_content_hash, stable_json_hash
from .config import DEFAULT_STRATEGY_CONFIG, TrendStrategyConfig
from .data import (
    CachedPriceSource,
    InsufficientDataError,
    PriceDataError,
    PriceDataSource,
    YFinancePriceSource,
    normalize_price_frame,
)
from .evaluation import evaluate_trend_frame
from .experiments import validate_snapshot
from .strategy import TrendStrategy


def _history_start(as_of: date, config: TrendStrategyConfig) -> date:
    # Calendar buffer covers weekends, holidays, and a conservative warm-up.
    return as_of - timedelta(days=max(550, math.ceil(config.required_history_bars * 2.1)))


def _frame_until(frame: pd.DataFrame, as_of: date) -> pd.DataFrame:
    return frame.loc[frame.index.date <= as_of].copy()


def _combined_data_hash(frames: dict[str, pd.DataFrame], as_of: date) -> str:
    return stable_json_hash(
        {
            symbol: frame_content_hash(_frame_until(frame, as_of))
            for symbol, frame in sorted(frames.items())
        }
    )


def _percentile(values: dict[str, float], target: str) -> float | None:
    if target not in values or len(values) < 2:
        return None
    series = pd.Series(values, dtype="float64")
    rank = float(series.rank(method="average", ascending=True)[target])
    return (rank - 1.0) / (len(series) - 1.0)


class QuantTrendService:
    """LLM-free trend baseline with replaceable market data and artifact stores."""

    def __init__(
        self,
        *,
        data_source: PriceDataSource | None = None,
        artifact_store: QuantArtifactStore | None = None,
        config: TrendStrategyConfig = DEFAULT_STRATEGY_CONFIG,
        benchmark: str = "SPY",
    ) -> None:
        self.data_source = data_source or CachedPriceSource(YFinancePriceSource())
        self.artifact_store = artifact_store or QuantArtifactStore()
        self.config = config
        self.benchmark = benchmark.strip().upper()
        self.strategy = TrendStrategy(config)

    def predict(
        self,
        *,
        symbol: str,
        as_of: date,
        horizon_days: int = 20,
        universe: Sequence[str] = (),
        universe_version: str | None = None,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        if horizon_days < 5 or horizon_days > 252:
            raise ValueError("horizon_days must be between 5 and 252")
        safe_symbol = symbol.strip().upper()
        if not safe_symbol:
            raise ValueError("symbol is required")
        normalized_universe = tuple(
            dict.fromkeys(item.strip().upper() for item in universe if item.strip())
        )
        if len(normalized_universe) > 500:
            raise ValueError("universe is limited to 500 symbols per request")
        if normalized_universe and safe_symbol not in normalized_universe:
            normalized_universe = (safe_symbol, *normalized_universe)
        request_contract = {
            "schema_version": "quant.trend_request.v1",
            "strategy_version": self.config.strategy_version,
            "config_hash": self.config.config_hash(),
            "data_source": self.data_source.provider_id,
            "benchmark": self.benchmark,
            "symbol": safe_symbol,
            "requested_as_of_date": as_of.isoformat(),
            "horizon_trading_days": horizon_days,
            "universe": normalized_universe,
            "universe_version": universe_version or ("single_asset" if not normalized_universe else "unversioned"),
        }
        request_hash = stable_json_hash(request_contract)
        if not force_refresh:
            cached = self.artifact_store.load_snapshot(request_hash)
            if cached is not None:
                try:
                    validate_snapshot(
                        cached,
                        expected={
                            "strategy_version": request_contract["strategy_version"],
                            "config_hash": request_contract["config_hash"],
                            "data_source": request_contract["data_source"],
                            "benchmark": request_contract["benchmark"],
                            "symbol": request_contract["symbol"],
                            "requested_as_of_date": request_contract["requested_as_of_date"],
                            "horizon_trading_days": request_contract["horizon_trading_days"],
                            "universe_version": request_contract["universe_version"],
                            "universe_hash": stable_json_hash(normalized_universe),
                        },
                    )
                except ValueError:
                    cached = None
                else:
                    return cached

        start = _history_start(as_of, self.config)
        requested_symbols = tuple(
            dict.fromkeys((safe_symbol, self.benchmark, *normalized_universe))
        )
        refresh_many = getattr(self.data_source, "refresh_many", None)
        if force_refresh and callable(refresh_many):
            source_frames = refresh_many(requested_symbols, start, as_of)
        else:
            source_frames = self.data_source.history_many(requested_symbols, start, as_of)
        frames: dict[str, pd.DataFrame] = {}
        for returned_symbol, frame in source_frames.items():
            canonical_symbol = str(returned_symbol).strip().upper()
            if not canonical_symbol:
                continue
            try:
                frames[canonical_symbol] = normalize_price_frame(frame, symbol=canonical_symbol)
            except PriceDataError:
                if canonical_symbol == safe_symbol:
                    raise
                continue
        if safe_symbol not in frames:
            raise PriceDataError(f"{safe_symbol}: data source returned no usable frame")
        stock = frames[safe_symbol]
        stock_index = self.strategy.resolve_as_of_index(stock, as_of)
        actual_as_of = stock.index[stock_index].date()

        benchmark_momentum: float | None = None
        if self.benchmark in frames:
            benchmark_frame = frames[self.benchmark]
            try:
                benchmark_index = self.strategy.resolve_as_of_index(benchmark_frame, actual_as_of)
                benchmark_momentum = self.strategy.momentum_at(benchmark_frame, benchmark_index)
            except InsufficientDataError:
                benchmark_momentum = None

        relative_percentile: float | None = None
        eligible_momenta: dict[str, float] = {}
        stale_universe_symbols: list[str] = []
        if normalized_universe:
            for candidate in normalized_universe:
                frame = frames.get(candidate)
                if frame is None:
                    continue
                try:
                    candidate_index = self.strategy.resolve_as_of_index(frame, actual_as_of)
                    if frame.index[candidate_index].date() != actual_as_of:
                        stale_universe_symbols.append(candidate)
                        continue
                    eligible_momenta[candidate] = self.strategy.momentum_at(frame, candidate_index)
                except (InsufficientDataError, PriceDataError):
                    continue
            if len(eligible_momenta) >= self.config.minimum_cross_section:
                relative_percentile = _percentile(eligible_momenta, safe_symbol)

        try:
            prediction = self.strategy.predict_at(
                stock,
                stock_index,
                relative_momentum_percentile=relative_percentile,
                benchmark_momentum_12_2=benchmark_momentum,
            )
        except InsufficientDataError as exc:
            return {
                **request_contract,
                "schema_version": "quant.trend_snapshot.v1",
                "status": "insufficient_data",
                "as_of_date": actual_as_of.isoformat(),
                "action": None,
                "direction": None,
                "signal_strength": None,
                "insufficient_reason": str(exc),
                "probabilities": None,
            }

        relevant_frames = {
            candidate: frame
            for candidate, frame in frames.items()
            if candidate in {safe_symbol, self.benchmark, *eligible_momenta.keys()}
        }
        data_hash = _combined_data_hash(relevant_frames, actual_as_of)
        direction = {"BUY": "BULLISH", "SELL": "BEARISH", "HOLD": "NEUTRAL"}[prediction["action"]]
        warnings = [
            "Signal strength is rule agreement, not a calibrated probability.",
            "Yahoo Finance is suitable for exploratory research but is not a survivorship-safe institutional data set.",
        ]
        if normalized_universe and len(eligible_momenta) < self.config.minimum_cross_section:
            warnings.append(
                f"Cross-sectional confirmation is unavailable: {len(eligible_momenta)} valid symbols; "
                f"{self.config.minimum_cross_section} required."
            )
        if normalized_universe and (universe_version is None or universe_version == "unversioned"):
            warnings.append("The supplied universe has no point-in-time version and must not support causal research claims.")

        snapshot_core = {
            "schema_version": "quant.trend_snapshot.v1",
            "status": "ok",
            "strategy_version": self.config.strategy_version,
            "config_hash": self.config.config_hash(),
            "data_source": self.data_source.provider_id,
            "benchmark": self.benchmark,
            "adjustment_policy": "split-adjusted price OHLC plus total-return adjusted close for momentum factors",
            "data_hash": data_hash,
            "symbol": safe_symbol,
            "requested_as_of_date": as_of.isoformat(),
            "as_of_date": prediction["as_of_date"],
            "horizon_trading_days": horizon_days,
            "universe_version": request_contract["universe_version"],
            "universe_hash": stable_json_hash(normalized_universe),
            "universe_requested_count": len(normalized_universe),
            "universe_eligible_count": len(eligible_momenta),
            "universe_stale_symbols": stale_universe_symbols,
            "action": prediction["action"],
            "direction": direction,
            "trend_score": prediction["trend_score"],
            "signal_strength": prediction["signal_strength"],
            "decision_rule": prediction["decision_rule"],
            "component_actions": prediction["component_actions"],
            "factors": prediction["factors"],
            "reasons": prediction["reasons"],
            "probabilities": None,
            "calibration_status": "not_calibrated",
            "warnings": warnings,
        }
        snapshot = {**snapshot_core, "snapshot_id": stable_json_hash(snapshot_core)}
        self.artifact_store.save_snapshot(request_hash, snapshot)
        return snapshot

    def evaluate(
        self,
        *,
        symbol: str,
        start_date: date,
        end_date: date,
        horizon_days: int = 20,
        step_days: int | None = None,
        maximum_nodes: int | None = None,
        transaction_cost_bps: float = 10.0,
    ) -> dict[str, Any]:
        safe_symbol = symbol.strip().upper()
        if not safe_symbol:
            raise ValueError("symbol is required")
        future_calendar_days = math.ceil(horizon_days * 2.0) + 14
        load_start = _history_start(start_date, self.config)
        load_end = min(date.today(), end_date + timedelta(days=future_calendar_days))
        stock = normalize_price_frame(
            self.data_source.history(safe_symbol, load_start, load_end),
            symbol=safe_symbol,
        )
        benchmark: pd.DataFrame | None = None
        try:
            benchmark = stock if safe_symbol == self.benchmark else normalize_price_frame(
                self.data_source.history(self.benchmark, load_start, load_end),
                symbol=self.benchmark,
            )
        except PriceDataError:
            benchmark = None

        evaluation = evaluate_trend_frame(
            stock,
            symbol=safe_symbol,
            start_date=start_date,
            end_date=end_date,
            horizon_days=horizon_days,
            step_days=step_days,
            maximum_nodes=maximum_nodes,
            benchmark_frame=benchmark,
            config=self.config,
            transaction_cost_bps=transaction_cost_bps,
        )
        first_as_of = date.fromisoformat(evaluation["nodes"][0]["as_of_date"])
        last_as_of = date.fromisoformat(evaluation["nodes"][-1]["as_of_date"])
        last_outcome = date.fromisoformat(evaluation["nodes"][-1]["outcome_date"])
        stock_first_position = stock.index.get_indexer([pd.Timestamp(first_as_of)])[0]
        stock_last_position = stock.index.get_indexer([pd.Timestamp(last_outcome)])[0]
        if stock_first_position < 0 or stock_last_position < 0:
            raise PriceDataError("evaluation dates are missing from the canonical stock frame")
        stock_start_position = max(
            0,
            stock_first_position - self.config.required_history_bars + 1,
        )
        effective_stock = stock.iloc[stock_start_position : stock_last_position + 1].copy()
        effective_frames = {safe_symbol: effective_stock}
        if benchmark is not None and safe_symbol != self.benchmark:
            first_candidates = benchmark.index[benchmark.index.date <= first_as_of]
            last_candidates = benchmark.index[benchmark.index.date <= last_as_of]
            if len(first_candidates) and len(last_candidates):
                first_position = benchmark.index.get_loc(first_candidates[-1])
                last_position = benchmark.index.get_loc(last_candidates[-1])
                if isinstance(first_position, int) and isinstance(last_position, int):
                    benchmark_start = max(
                        0,
                        first_position - self.config.required_history_bars + 1,
                    )
                    effective_frames[self.benchmark] = benchmark.iloc[
                        benchmark_start : last_position + 1
                    ].copy()
        data_hash = stable_json_hash(
            {
                candidate: frame_content_hash(frame)
                for candidate, frame in sorted(effective_frames.items())
            }
        )
        protocol = {
            "schema_version": "quant.trend_evaluation_protocol.v1",
            "strategy_version": self.config.strategy_version,
            "config_hash": self.config.config_hash(),
            "symbol": safe_symbol,
            "start_date": start_date.isoformat(),
            "end_date": end_date.isoformat(),
            "horizon_trading_days": horizon_days,
            "step_trading_days": step_days or horizon_days,
            "maximum_nodes": maximum_nodes,
            "transaction_cost_bps": transaction_cost_bps,
            "data_source": self.data_source.provider_id,
            "benchmark": self.benchmark,
        }
        run_core = {
            **evaluation,
            "protocol": protocol,
            "protocol_hash": stable_json_hash(protocol),
            "data_hash": data_hash,
            "data_source": self.data_source.provider_id,
            "data_window": {
                "first_date": effective_stock.index[0].date().isoformat(),
                "last_date": effective_stock.index[-1].date().isoformat(),
                "rows": len(effective_stock),
            },
        }
        result = {**run_core, "run_id": stable_json_hash(run_core)}
        path = self.artifact_store.save_evaluation(result)
        return {**result, "artifact_path": str(Path(path).resolve())}
