"""Point-in-time price-source boundary for deterministic trend research."""

from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import date, timedelta
import json
import os
from pathlib import Path
import tempfile
from threading import Lock
from typing import Protocol, Sequence

import pandas as pd


class PriceDataError(RuntimeError):
    """Raised when a price source cannot provide an auditable daily frame."""


class InsufficientDataError(PriceDataError):
    """Raised when valid data exists but cannot satisfy the frozen warm-up."""


class PriceDataSource(Protocol):
    """Minimal replaceable data-source contract used by the quant engine."""

    provider_id: str

    def history(self, symbol: str, start: date, end: date) -> pd.DataFrame:
        """Return adjusted daily OHLCV rows in the inclusive date range."""

    def history_many(self, symbols: Sequence[str], start: date, end: date) -> dict[str, pd.DataFrame]:
        """Return frames independently so one failed symbol cannot corrupt peers."""


def normalize_price_frame(frame: pd.DataFrame, *, symbol: str) -> pd.DataFrame:
    """Normalize a vendor frame and reject data that cannot support replay."""

    if frame is None or frame.empty:
        raise PriceDataError(f"{symbol}: price history is empty")
    renamed = {column: str(column).strip().lower().replace(" ", "_") for column in frame.columns}
    normalized = frame.rename(columns=renamed).copy()
    required = ("open", "high", "low", "close")
    missing = [column for column in required if column not in normalized.columns]
    if missing:
        raise PriceDataError(f"{symbol}: missing price columns: {', '.join(missing)}")
    if "total_return_close" not in normalized.columns:
        if "adj_close" not in normalized.columns:
            raise PriceDataError(
                f"{symbol}: missing total-return close (expected total_return_close or adj_close)"
            )
        normalized["total_return_close"] = normalized["adj_close"]
    keep = [
        *required,
        "total_return_close",
        *(column for column in ("volume", "dividends", "stock_splits") if column in normalized.columns),
    ]
    normalized = normalized[keep]
    normalized.index = pd.to_datetime(normalized.index, errors="coerce")
    if getattr(normalized.index, "tz", None) is not None:
        normalized.index = normalized.index.tz_localize(None)
    normalized = normalized[~normalized.index.isna()]
    normalized = normalized[~normalized.index.duplicated(keep="last")].sort_index()
    for column in keep:
        normalized[column] = pd.to_numeric(normalized[column], errors="coerce")
    normalized = normalized.replace([float("inf"), float("-inf")], float("nan"))
    normalized = normalized.dropna(subset=[*required, "total_return_close"])
    normalized = normalized[
        (normalized["open"] > 0)
        & (normalized["high"] > 0)
        & (normalized["low"] > 0)
        & (normalized["close"] > 0)
        & (normalized["total_return_close"] > 0)
    ]
    if normalized.empty:
        raise PriceDataError(f"{symbol}: no valid positive OHLC rows")
    duplicate_dates = pd.Index(normalized.index.date).duplicated(keep=False)
    if duplicate_dates.any():
        raise PriceDataError(f"{symbol}: daily price history contains more than one row per date")
    invalid_ohlc = (
        (normalized["high"] < normalized[["open", "close", "low"]].max(axis=1))
        | (normalized["low"] > normalized[["open", "close", "high"]].min(axis=1))
    )
    if invalid_ohlc.any():
        raise PriceDataError(f"{symbol}: daily price history contains invalid OHLC relationships")
    return normalized.astype({column: "float64" for column in keep})


class YFinancePriceSource:
    """Adjusted daily Yahoo Finance data via the repo's existing yfinance dependency.

    Yahoo is suitable for local research and the first ablation study because it
    is already deployed in the sidecar and exposes adjusted OHLC.  The protocol
    above keeps the strategy independent so a survivorship-safe institutional
    source can replace it without changing factor or experiment code.
    """

    provider_id = "yfinance.price_and_total_return.v1"

    def __init__(self, *, max_workers: int = 8) -> None:
        self.max_workers = max(1, min(16, int(max_workers)))

    def history(self, symbol: str, start: date, end: date) -> pd.DataFrame:
        import yfinance as yf

        safe_symbol = symbol.strip().upper()
        if not safe_symbol:
            raise PriceDataError("symbol is required")
        if end < start:
            raise PriceDataError("end date must not precede start date")
        try:
            # yfinance treats end as exclusive; add one day to keep this
            # provider's public contract inclusive.
            frame = yf.Ticker(safe_symbol).history(
                start=start.isoformat(),
                end=(end + timedelta(days=1)).isoformat(),
                auto_adjust=False,
                actions=True,
            )
        except Exception as exc:  # noqa: BLE001 - normalize vendor failures
            raise PriceDataError(f"{safe_symbol}: Yahoo Finance request failed") from exc
        return normalize_price_frame(frame, symbol=safe_symbol)

    def history_many(self, symbols: Sequence[str], start: date, end: date) -> dict[str, pd.DataFrame]:
        ordered = tuple(dict.fromkeys(symbol.strip().upper() for symbol in symbols if symbol.strip()))
        if not ordered:
            return {}
        frames: dict[str, pd.DataFrame] = {}
        with ThreadPoolExecutor(max_workers=min(self.max_workers, len(ordered))) as executor:
            futures = {executor.submit(self.history, symbol, start, end): symbol for symbol in ordered}
            for future in as_completed(futures):
                symbol = futures[future]
                try:
                    frames[symbol] = future.result()
                except PriceDataError:
                    continue
        return {symbol: frames[symbol] for symbol in ordered if symbol in frames}


class CachedPriceSource:
    """Project-owned canonical CSV cache around any deterministic provider.

    The cache grows one file per symbol and records requested coverage plus a
    content hash. It never silently falls back to another vendor. A damaged or
    mismatched cache is ignored and rebuilt from the configured source.
    """

    def __init__(self, source: PriceDataSource, root: str | Path | None = None, *, max_workers: int = 8) -> None:
        configured = root or os.getenv("QUANT_TREND_PRICE_CACHE_DIR", "")
        self.root = Path(configured) if configured else Path(__file__).resolve().parents[1] / "data" / "quant-trend" / "prices"
        self.source = source
        self.provider_id = source.provider_id
        self.max_workers = max(1, min(16, int(max_workers)))
        self._locks_guard = Lock()
        self._symbol_locks: dict[str, Lock] = {}

    @staticmethod
    def _token(symbol: str) -> str:
        token = "".join(character for character in symbol.upper() if character.isalnum() or character in ".-_")
        if not token:
            raise PriceDataError("symbol is required")
        return token

    def _paths(self, symbol: str) -> tuple[Path, Path]:
        token = self._token(symbol)
        return self.root / self.provider_id / f"{token}.csv", self.root / self.provider_id / f"{token}.json"

    def _lock_for(self, symbol: str) -> Lock:
        token = self._token(symbol)
        with self._locks_guard:
            return self._symbol_locks.setdefault(token, Lock())

    def _load(self, symbol: str) -> tuple[pd.DataFrame, dict] | None:
        from .artifacts import frame_content_hash

        csv_path, metadata_path = self._paths(symbol)
        try:
            metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
            if (
                metadata.get("schema_version") != "quant.price_cache.v1"
                or metadata.get("provider") != self.provider_id
                or metadata.get("symbol") != symbol.upper()
            ):
                return None
            date.fromisoformat(str(metadata["covered_start"]))
            date.fromisoformat(str(metadata["covered_end"]))
            frame = pd.read_csv(csv_path, index_col="date", parse_dates=["date"])
            normalized = normalize_price_frame(frame, symbol=symbol)
            if (
                int(metadata.get("rows", -1)) != len(normalized)
                or metadata.get("data_hash") != frame_content_hash(normalized)
            ):
                return None
        except (FileNotFoundError, KeyError, OSError, ValueError, json.JSONDecodeError, PriceDataError):
            return None
        return normalized, metadata

    @staticmethod
    def _atomic_text(path: Path, content: str) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                handle.write(content)
                handle.flush()
                os.fsync(handle.fileno())
            Path(temporary_name).replace(path)
        except Exception:
            Path(temporary_name).unlink(missing_ok=True)
            raise

    def _save(self, symbol: str, frame: pd.DataFrame, covered_start: date, covered_end: date) -> None:
        from .artifacts import frame_content_hash

        csv_path, metadata_path = self._paths(symbol)
        serializable = frame.copy()
        serializable.index.name = "date"
        self._atomic_text(csv_path, serializable.to_csv(date_format="%Y-%m-%d"))
        # Hash the persisted representation, not the in-memory vendor floats,
        # so the first and every later read have the same content identity.
        persisted = pd.read_csv(csv_path, index_col="date", parse_dates=["date"])
        persisted = normalize_price_frame(persisted, symbol=symbol)
        metadata = {
            "schema_version": "quant.price_cache.v1",
            "provider": self.provider_id,
            "symbol": symbol.upper(),
            "covered_start": covered_start.isoformat(),
            "covered_end": covered_end.isoformat(),
            "first_data_date": persisted.index[0].date().isoformat(),
            "last_data_date": persisted.index[-1].date().isoformat(),
            "rows": len(persisted),
            "data_hash": frame_content_hash(persisted),
        }
        self._atomic_text(metadata_path, json.dumps(metadata, ensure_ascii=False, sort_keys=True, indent=2))

    def history(self, symbol: str, start: date, end: date) -> pd.DataFrame:
        safe_symbol = symbol.strip().upper()
        if end < start:
            raise PriceDataError("end date must not precede start date")
        with self._lock_for(safe_symbol):
            cached = self._load(safe_symbol)
            if cached is not None:
                frame, metadata = cached
                try:
                    covered_start = date.fromisoformat(str(metadata["covered_start"]))
                    covered_end = date.fromisoformat(str(metadata["covered_end"]))
                except (KeyError, ValueError):
                    cached = None
                else:
                    if covered_start <= start and covered_end >= end:
                        subset = frame.loc[(frame.index.date >= start) & (frame.index.date <= end)].copy()
                        if not subset.empty:
                            return subset
            if cached is None:
                frame = None
                fetch_start, fetch_end = start, end
            else:
                frame, metadata = cached
                covered_start = date.fromisoformat(str(metadata["covered_start"]))
                covered_end = date.fromisoformat(str(metadata["covered_end"]))
                fetch_start, fetch_end = min(start, covered_start), max(end, covered_end)

            fetched = self.source.history(safe_symbol, fetch_start, fetch_end)
            combined = fetched if frame is None else pd.concat([frame, fetched])
            combined = normalize_price_frame(combined, symbol=safe_symbol)
            actual_first = combined.index[0].date()
            actual_last = combined.index[-1].date()
            # Only actual rows become coverage. This intentionally retries a
            # missing prefix so a transient vendor truncation cannot turn into
            # a permanent "insufficient history" result.
            saved_start = actual_first
            saved_end = actual_last
            self._save(safe_symbol, combined, saved_start, saved_end)
            canonical = self._load(safe_symbol)
            if canonical is None:
                raise PriceDataError(f"{safe_symbol}: canonical cache could not be verified after write")
            canonical_frame, _ = canonical
            subset = canonical_frame.loc[
                (canonical_frame.index.date >= start) & (canonical_frame.index.date <= end)
            ].copy()
            if subset.empty:
                raise PriceDataError(f"{safe_symbol}: cached source has no rows in requested range")
            return subset

    def refresh(self, symbol: str, start: date, end: date) -> pd.DataFrame:
        """Refresh the requested range from the upstream source and merge it."""

        safe_symbol = symbol.strip().upper()
        if end < start:
            raise PriceDataError("end date must not precede start date")
        with self._lock_for(safe_symbol):
            cached = self._load(safe_symbol)
            fetched = normalize_price_frame(
                self.source.history(safe_symbol, start, end),
                symbol=safe_symbol,
            )
            combined = fetched if cached is None else pd.concat([cached[0], fetched])
            combined = normalize_price_frame(combined, symbol=safe_symbol)
            self._save(
                safe_symbol,
                combined,
                combined.index[0].date(),
                combined.index[-1].date(),
            )
            canonical = self._load(safe_symbol)
            if canonical is None:
                raise PriceDataError(f"{safe_symbol}: canonical cache could not be verified after refresh")
            frame, _ = canonical
            subset = frame.loc[(frame.index.date >= start) & (frame.index.date <= end)].copy()
            if subset.empty:
                raise PriceDataError(f"{safe_symbol}: refreshed source has no rows in requested range")
            return subset

    def history_many(self, symbols: Sequence[str], start: date, end: date) -> dict[str, pd.DataFrame]:
        return self._many(self.history, symbols, start, end)

    def refresh_many(self, symbols: Sequence[str], start: date, end: date) -> dict[str, pd.DataFrame]:
        """Refresh several symbols concurrently for explicit force-refresh calls."""

        return self._many(self.refresh, symbols, start, end)

    def _many(self, loader, symbols: Sequence[str], start: date, end: date) -> dict[str, pd.DataFrame]:
        ordered = tuple(dict.fromkeys(symbol.strip().upper() for symbol in symbols if symbol.strip()))
        if not ordered:
            return {}
        frames: dict[str, pd.DataFrame] = {}
        with ThreadPoolExecutor(max_workers=min(self.max_workers, len(ordered))) as executor:
            futures = {executor.submit(loader, symbol, start, end): symbol for symbol in ordered}
            for future in as_completed(futures):
                symbol = futures[future]
                try:
                    frames[symbol] = future.result()
                except PriceDataError:
                    continue
        return {symbol: frames[symbol] for symbol in ordered if symbol in frames}
