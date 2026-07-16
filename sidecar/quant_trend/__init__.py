"""Deterministic low-frequency trend baseline for CANSLIMAgent.

The package deliberately has no LLM dependency.  It can be called directly,
through the sidecar HTTP API, or by a future ablation runner that compares the
same point-in-time nodes across quant-only and TradingAgents arms.
"""

from .config import DEFAULT_STRATEGY_CONFIG, TrendStrategyConfig
from .data import CachedPriceSource, InsufficientDataError, PriceDataError, PriceDataSource, YFinancePriceSource
from .evaluation import evaluate_trend_frame
from .experiments import (
    ExperimentArm,
    build_experiment_manifest,
    build_outcome_record,
    build_experiment_context,
    build_prediction_record,
    compare_predictions,
    decision_node_id,
    validate_snapshot,
)
from .service import QuantTrendService
from .strategy import TrendStrategy

__all__ = [
    "DEFAULT_STRATEGY_CONFIG",
    "CachedPriceSource",
    "ExperimentArm",
    "InsufficientDataError",
    "PriceDataError",
    "PriceDataSource",
    "QuantTrendService",
    "TrendStrategy",
    "TrendStrategyConfig",
    "YFinancePriceSource",
    "build_experiment_context",
    "build_experiment_manifest",
    "build_outcome_record",
    "build_prediction_record",
    "compare_predictions",
    "decision_node_id",
    "validate_snapshot",
    "evaluate_trend_frame",
]
