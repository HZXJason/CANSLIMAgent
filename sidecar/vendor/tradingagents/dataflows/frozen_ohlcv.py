"""Run-scoped, protocol-frozen OHLCV for historical replay tools."""

from __future__ import annotations

from contextlib import contextmanager
from contextvars import ContextVar
from dataclasses import dataclass, field
from typing import Iterator

import pandas as pd

from .symbol_utils import normalize_symbol


def _symbol_tokens(value: str) -> frozenset[str]:
    raw = str(value or "").strip().upper()
    canonical = str(normalize_symbol(raw) or "").strip().upper()
    tokens = {token for token in (raw, canonical) if token}
    for token in tuple(tokens):
        if token.endswith(".US"):
            tokens.add(token[:-3])
    return frozenset(tokens)


@dataclass
class FrozenOHLCVState:
    symbol: str
    model_data_id: str
    frame: pd.DataFrame
    usage: list[dict] = field(default_factory=list)

    def select(
        self,
        requested_symbol: str,
        *,
        start_date: str | None,
        end_date: str,
        purpose: str,
    ) -> pd.DataFrame | None:
        if not (_symbol_tokens(self.symbol) & _symbol_tokens(requested_symbol)):
            return None
        end = pd.to_datetime(end_date, errors="raise").normalize()
        start = (
            pd.to_datetime(start_date, errors="raise").normalize()
            if start_date
            else self.frame["Date"].min()
        )
        subset = self.frame.loc[
            (self.frame["Date"] >= start) & (self.frame["Date"] <= end)
        ].copy()
        if subset.empty:
            return None
        self.usage.append(
            {
                "schema_version": "ta.frozen_ohlcv_usage.v1",
                "model_data_id": self.model_data_id,
                "purpose": purpose,
                "requested_symbol": str(requested_symbol).strip().upper(),
                "requested_start_date": start_date,
                "requested_end_date": end_date,
                "returned_first_date": subset["Date"].iloc[0].date().isoformat(),
                "returned_last_date": subset["Date"].iloc[-1].date().isoformat(),
                "returned_rows": len(subset),
            }
        )
        return subset


_CURRENT: ContextVar[FrozenOHLCVState | None] = ContextVar(
    "tradingagents_frozen_ohlcv", default=None
)


@contextmanager
def frozen_ohlcv_scope(
    symbol: str, model_data_id: str, frame: pd.DataFrame
) -> Iterator[FrozenOHLCVState]:
    required = {"Date", "Open", "High", "Low", "Close", "Volume"}
    missing = sorted(required - set(frame.columns))
    if missing:
        raise ValueError(f"frozen OHLCV is missing columns: {', '.join(missing)}")
    normalized = frame.copy()
    normalized["Date"] = pd.to_datetime(normalized["Date"], errors="raise")
    normalized = normalized.sort_values("Date").drop_duplicates("Date", keep="last")
    state = FrozenOHLCVState(
        symbol=str(symbol).strip().upper(),
        model_data_id=model_data_id,
        frame=normalized,
    )
    token = _CURRENT.set(state)
    try:
        yield state
    finally:
        _CURRENT.reset(token)


def frozen_ohlcv_slice(
    symbol: str,
    *,
    start_date: str | None,
    end_date: str,
    purpose: str,
) -> pd.DataFrame | None:
    state = _CURRENT.get()
    if state is None:
        return None
    return state.select(
        symbol,
        start_date=start_date,
        end_date=end_date,
        purpose=purpose,
    )


def frozen_model_data_id() -> str | None:
    state = _CURRENT.get()
    return state.model_data_id if state is not None else None
