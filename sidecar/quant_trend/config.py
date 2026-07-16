"""Versioned configuration for the pre-registered trend baseline."""

from __future__ import annotations

from dataclasses import asdict, dataclass
import hashlib
import json
from typing import Any


@dataclass(frozen=True)
class TrendStrategyConfig:
    """Frozen parameters for a transparent, low-frequency trend baseline.

    The directional decision is the majority sign of 1-, 3-, and 12-month
    time-series momentum. The moving-average regime, 52-week high and optional
    12-2 cross-sectional momentum rank are reported as independent evidence,
    not silently fitted into the decision.
    """

    schema_version: str = "quant.trend_config.v1"
    strategy_version: str = "daily_tsm_sign_21_63_252.v1"
    tsm_short_days: int = 21
    tsm_medium_days: int = 63
    tsm_long_days: int = 252
    # Kenneth French daily momentum convention: t-251 through t-21.
    momentum_lookback_days: int = 251
    momentum_skip_days: int = 21
    fast_ma_days: int = 50
    slow_ma_days: int = 200
    ma_slope_days: int = 20
    high_lookback_days: int = 252
    high_confirmation_ratio: float = 0.90
    low_confirmation_ratio: float = 0.75
    relative_winner_percentile: float = 0.70
    relative_loser_percentile: float = 0.30
    minimum_cross_section: int = 20
    atr_days: int = 20
    barrier_atr_multiple: float = 2.0
    minimum_barrier: float = 0.05
    # Terminal direction ignores moves too small to cover a modest low-frequency
    # round trip; this is intentionally separate from the wider path barrier.
    neutral_return_band: float = 0.01

    @property
    def required_history_bars(self) -> int:
        return max(
            self.momentum_lookback_days + 1,
            self.tsm_long_days + 1,
            self.high_lookback_days,
            self.slow_ma_days + self.ma_slope_days,
            self.atr_days + 1,
        )

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def config_hash(self) -> str:
        encoded = json.dumps(self.to_dict(), sort_keys=True, separators=(",", ":")).encode("utf-8")
        return hashlib.sha256(encoded).hexdigest()


DEFAULT_STRATEGY_CONFIG = TrendStrategyConfig()
