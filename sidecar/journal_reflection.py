"""Trade-journal reflection for ``vibe.trade_record.v1`` fills.

The TradingAgent2 runtime and Vibe-Trading use separate Python environments.
This module validates records and builds Chinese reflection text locally, then
launches Vibe's own interpreter to execute its canonical journal primitives:

* tolerant validation and normalization of application-provided fill dicts;
* long-only FIFO lot matching with correct proportional fee allocation;
* Vibe's real profile and behavioural diagnostics through subprocess isolation;
* per-symbol observations and a concise Chinese reflection for prompt context.

The public result contains only JSON-safe primitives.  It intentionally does
not call an LLM or a market-data provider, which makes the same journal produce
the same reflection in tests, API requests, and background jobs.
"""

from __future__ import annotations

import json
import math
import os
import statistics
import subprocess
from collections import defaultdict, deque
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


INPUT_SCHEMA_VERSION = "vibe.trade_record.v1"
REFLECTION_SCHEMA_VERSION = "canslim.journal_reflection.v1"
MIN_SHADOW_PROFITABLE_ROUNDTRIPS = 5

_ALLOWED_MARKETS = {"china_a", "us", "hk", "crypto", "other"}
_EPSILON = 1e-9


@dataclass(frozen=True)
class _Trade:
    source_index: int
    dt: datetime
    symbol: str
    name: str
    side: str
    quantity: float
    price: float
    amount: float
    fee: float
    market: str


@dataclass
class _OpenLot:
    trade: _Trade
    remaining_quantity: float
    remaining_fee: float


@dataclass(frozen=True)
class _Roundtrip:
    sequence: int
    symbol: str
    name: str
    market: str
    buy_dt: datetime
    sell_dt: datetime
    quantity: float
    buy_price: float
    sell_price: float
    buy_fee: float
    sell_fee: float
    gross_pnl: float
    pnl: float

    @property
    def hold_days(self) -> float:
        return (self.sell_dt - self.buy_dt).total_seconds() / 86_400.0

    @property
    def pnl_pct(self) -> float:
        cost = self.buy_price * self.quantity
        return self.pnl / cost if cost else 0.0


def analyze_trade_records(records: Sequence[Mapping[str, Any]] | None) -> dict[str, Any]:
    """Analyze in-memory ``vibe.trade_record.v1`` records.

    Invalid required fields cause only that record to be skipped.  Recoverable
    problems (missing amount/market, invalid optional fee, an unmatched sell)
    are normalized and surfaced in ``validation_warnings``.  FIFO pairing is
    long-only, matching Vibe-Trading's journal analyzer; unmatched shorts are
    reported rather than silently treated as open short positions.
    """

    warnings: list[dict[str, Any]] = []
    skipped_indices: list[int] = []

    if records is None:
        source_records: list[Any] = []
    elif isinstance(records, Sequence) and not isinstance(records, (str, bytes, bytearray, Mapping)):
        source_records = list(records)
    else:
        source_records = []
        warnings.append(_warning("input_not_list", "交易日志必须是记录数组，当前输入已按空数组处理"))

    normalized: list[_Trade] = []
    for index, raw in enumerate(source_records):
        trade = _normalize_trade(raw, index, warnings)
        if trade is None:
            skipped_indices.append(index)
        else:
            normalized.append(trade)

    sorted_trades = sorted(normalized, key=lambda trade: (trade.dt, trade.source_index))
    if [trade.source_index for trade in sorted_trades] != [trade.source_index for trade in normalized]:
        warnings.append(_warning("records_reordered", "成交记录未按时间排列，分析时已按成交时间重新排序"))

    roundtrips, open_lots, unmatched_sells = _pair_fifo(sorted_trades, warnings)
    profile = _compute_profile(sorted_trades, roundtrips, open_lots, unmatched_sells)
    behavior = _compute_behavior(sorted_trades, roundtrips)
    per_symbol = _compute_per_symbol(sorted_trades, roundtrips, open_lots, unmatched_sells)

    traded_markets = sorted({trade.market for trade in sorted_trades})
    if len(traded_markets) > 1:
        warnings.append(
            _warning(
                "mixed_markets_without_currency_conversion",
                "日志包含多个市场；盈亏和成交额未做币种换算，合计值仅供行为复盘参考",
            )
        )

    overall_insights, symbol_insights = _build_insights(
        profile,
        behavior,
        per_symbol,
        open_lots,
        unmatched_sells,
        mixed_markets=len(traded_markets) > 1,
    )
    reflection_text = _build_reflection_text(overall_insights)

    profitable = sum(1 for trip in roundtrips if trip.pnl > _EPSILON)
    shadow_ready = profitable >= MIN_SHADOW_PROFITABLE_ROUNDTRIPS
    if not sorted_trades or not roundtrips:
        status = "insufficient_data"
    else:
        status = "ok"

    result: dict[str, Any] = {
        "schema_version": REFLECTION_SCHEMA_VERSION,
        "input_schema_version": INPUT_SCHEMA_VERSION,
        "status": status,
        "record_count": len(sorted_trades),
        "validation": {
            "input_records": len(source_records),
            "valid_records": len(sorted_trades),
            "invalid_records": len(skipped_indices),
            "skipped_record_indices": skipped_indices,
            "warning_count": len(warnings),
        },
        "validation_warnings": warnings,
        "date_range": {
            "start": _iso(sorted_trades[0].dt) if sorted_trades else None,
            "end": _iso(sorted_trades[-1].dt) if sorted_trades else None,
        },
        "roundtrips": [_roundtrip_to_dict(trip) for trip in roundtrips],
        "open_lots": open_lots,
        "unmatched_sells": unmatched_sells,
        "profile": profile,
        "behavior": behavior,
        "per_symbol": per_symbol,
        "insights": {
            "overall": overall_insights,
            "per_symbol": symbol_insights,
        },
        "strategy_extraction_readiness": {
            "ready": shadow_ready,
            "profitable_roundtrips": profitable,
            "required_profitable_roundtrips": MIN_SHADOW_PROFITABLE_ROUNDTRIPS,
            "reason": (
                "已达到 Vibe Shadow Account 的最低盈利回合样本要求"
                if shadow_ready
                else f"盈利闭合回合不足 {MIN_SHADOW_PROFITABLE_ROUNDTRIPS} 个，暂不宜提取稳定策略"
            ),
        },
        "shadow_strategy_ready": shadow_ready,
        "reflection_text": reflection_text,
    }

    vibe_result, vibe_trace = _run_vibe_journal(sorted_trades)
    result["engine"] = vibe_result.get("engine", "vibe-trading") if vibe_result else "canslim-compatible-fallback"
    result["source_trace"] = vibe_trace
    if vibe_result:
        result["vibe_analysis"] = vibe_result
        # Keep the corrected application profile as the public summary.  The
        # upstream Vibe FIFO currently over-allocates original buy fees after a
        # partial sell; its untouched canonical output remains auditable under
        # ``vibe_analysis`` while the user-facing totals use remaining-fee lots.
        result["compatibility_corrections"] = [
            "partial-fill buy fees are allocated from the remaining fee pool"
        ]
        vibe_behavior = vibe_result.get("behavior")
        if isinstance(vibe_behavior, dict):
            merged_behavior: dict[str, Any] = {}
            for key in set(behavior) | set(vibe_behavior):
                local_item = behavior.get(key)
                vibe_item = vibe_behavior.get(key)
                if isinstance(local_item, dict) and isinstance(vibe_item, dict):
                    merged_behavior[key] = {**local_item, **vibe_item}
                else:
                    merged_behavior[key] = vibe_item if vibe_item is not None else local_item
            result["behavior"] = merged_behavior

    # This assertion is deliberately part of the boundary.  It prevents a
    # future metric from leaking NaN/Infinity or datetime objects to FastAPI.
    json.dumps(result, ensure_ascii=False, allow_nan=False)
    return result


def build_journal_reflection(records: Sequence[Mapping[str, Any]] | None) -> dict[str, Any]:
    """Semantic alias used by callers that treat the output as prompt context."""

    return analyze_trade_records(records)


def _vibe_root() -> Path:
    configured = os.getenv("VIBE_TRADING_ROOT")
    if configured:
        return Path(configured).expanduser().resolve()
    bundled = Path(__file__).resolve().parent / "vendor" / "Vibe-Trading"
    if bundled.exists():
        return bundled
    return Path(__file__).resolve().parents[2] / "TradingAgent2.0" / "Vibe-Trading"


def _trade_to_vibe_record(trade: _Trade) -> dict[str, Any]:
    return {
        "datetime": _iso(trade.dt),
        "symbol": trade.symbol,
        "name": trade.name,
        "side": trade.side,
        "quantity": trade.quantity,
        "price": trade.price,
        "amount": trade.amount,
        "fee": trade.fee,
        "market": trade.market,
    }


def _run_vibe_journal(trades: list[_Trade]) -> tuple[dict[str, Any] | None, dict[str, Any]]:
    """Invoke Vibe's real journal functions in the Vibe virtual environment."""
    root = _vibe_root()
    interpreter = root / ".venv" / "bin" / "python"
    runner = Path(__file__).resolve().with_name("vibe_journal_runner.py")
    trace: dict[str, Any] = {
        "provider": "vibe-trading",
        "transport": "isolated-subprocess",
        "interpreter": str(interpreter),
        "runner": str(runner),
        "status": "error",
        "error": None,
    }
    if not trades:
        trace.update({"status": "skipped", "error": "no valid trade records"})
        return None, trace
    if not interpreter.exists():
        trace["error"] = f"Vibe-Trading interpreter not found: {interpreter}"
        return None, trace

    timeout_raw = os.getenv("VIBE_JOURNAL_TIMEOUT_SECONDS", "30")
    try:
        timeout = max(1.0, float(timeout_raw))
    except ValueError:
        timeout = 30.0
    env = os.environ.copy()
    env["VIBE_TRADING_ROOT"] = str(root)
    payload = json.dumps(
        {"schema_version": INPUT_SCHEMA_VERSION, "records": [_trade_to_vibe_record(trade) for trade in trades]},
        ensure_ascii=False,
        allow_nan=False,
    )
    try:
        completed = subprocess.run(
            [str(interpreter), str(runner)],
            input=payload,
            text=True,
            capture_output=True,
            cwd=str(root / "agent"),
            env=env,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        trace["error"] = str(exc)
        return None, trace
    if completed.returncode != 0:
        trace["error"] = (completed.stderr or completed.stdout or f"exit {completed.returncode}").strip()[:2000]
        return None, trace
    try:
        result = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        trace["error"] = f"invalid Vibe JSON output: {exc}"
        return None, trace
    if not isinstance(result, dict) or result.get("engine") != "vibe-trading":
        trace["error"] = "Vibe runner returned an unexpected payload"
        return None, trace
    trace.update(
        {
            "status": "ok",
            "error": None,
            "implementation": result.get("implementation"),
            "record_count": result.get("record_count"),
        }
    )
    return result, trace


def _normalize_trade(raw: Any, index: int, warnings: list[dict[str, Any]]) -> _Trade | None:
    if not isinstance(raw, Mapping):
        warnings.append(_warning("record_not_object", "成交记录不是对象，已跳过", index))
        return None

    symbol = str(raw.get("symbol") or "").strip().upper()
    if not symbol:
        warnings.append(_warning("missing_symbol", "缺少 symbol，成交记录已跳过", index, "symbol"))
        return None

    dt = _parse_datetime(raw.get("datetime"))
    if dt is None:
        warnings.append(_warning("invalid_datetime", "datetime 不是有效的 ISO8601 时间，成交记录已跳过", index, "datetime"))
        return None

    side = str(raw.get("side") or "").strip().lower()
    if side not in {"buy", "sell"}:
        warnings.append(_warning("invalid_side", "side 必须为 buy 或 sell，成交记录已跳过", index, "side"))
        return None

    quantity = _finite_number(raw.get("quantity"))
    if quantity is None or quantity <= 0:
        warnings.append(_warning("invalid_quantity", "quantity 必须为正数，成交记录已跳过", index, "quantity"))
        return None

    price = _finite_number(raw.get("price"))
    if price is None or price <= 0:
        warnings.append(_warning("invalid_price", "price 必须为正数，成交记录已跳过", index, "price"))
        return None

    expected_amount = quantity * price
    amount = _finite_number(raw.get("amount"))
    if amount is None or amount <= 0:
        amount = expected_amount
        warnings.append(_warning("amount_recomputed", "amount 缺失或无效，已按 quantity × price 重算", index, "amount"))
    elif abs(amount - expected_amount) > max(0.01, expected_amount * 0.001):
        amount = expected_amount
        warnings.append(_warning("amount_mismatch", "amount 与 quantity × price 不一致，分析时已使用重算值", index, "amount"))

    fee = _finite_number(raw.get("fee"))
    if fee is None or fee < 0:
        fee = 0.0
        warnings.append(_warning("fee_defaulted", "fee 缺失、非数字或为负数，分析时已按 0 处理", index, "fee"))
    elif fee > amount:
        warnings.append(_warning("fee_exceeds_amount", "fee 大于成交额，请核对费用字段", index, "fee"))

    raw_market = str(raw.get("market") or "").strip().lower()
    if raw_market in _ALLOWED_MARKETS:
        market = raw_market
    else:
        market = _infer_market(symbol)
        code = "market_inferred" if not raw_market else "invalid_market_defaulted"
        warnings.append(_warning(code, f"market 无效或缺失，已推断为 {market}", index, "market"))

    name_value = raw.get("name")
    name = "" if name_value is None else str(name_value).strip()
    return _Trade(
        source_index=index,
        dt=dt,
        symbol=symbol,
        name=name,
        side=side,
        quantity=quantity,
        price=price,
        amount=amount,
        fee=fee,
        market=market,
    )


def _pair_fifo(
    trades: list[_Trade],
    warnings: list[dict[str, Any]],
) -> tuple[list[_Roundtrip], list[dict[str, Any]], list[dict[str, Any]]]:
    queues: dict[str, deque[_OpenLot]] = defaultdict(deque)
    roundtrips: list[_Roundtrip] = []
    unmatched_sells: list[dict[str, Any]] = []

    for trade in trades:
        if trade.side == "buy":
            queues[trade.symbol].append(
                _OpenLot(
                    trade=trade,
                    remaining_quantity=trade.quantity,
                    remaining_fee=trade.fee,
                )
            )
            continue

        remaining_sell = trade.quantity
        queue = queues[trade.symbol]
        while remaining_sell > _EPSILON and queue:
            lot = queue[0]
            take = min(lot.remaining_quantity, remaining_sell)
            # Allocate from the *remaining* fee pool.  This avoids Vibe's old
            # partial-fill over-allocation bug while preserving FIFO semantics.
            buy_fee = (
                lot.remaining_fee * (take / lot.remaining_quantity)
                if lot.remaining_quantity > _EPSILON
                else 0.0
            )
            sell_fee = trade.fee * (take / trade.quantity)
            gross_pnl = (trade.price - lot.trade.price) * take
            pnl = gross_pnl - buy_fee - sell_fee
            if lot.trade.market != trade.market:
                warnings.append(
                    _warning(
                        "matched_market_mismatch",
                        f"{trade.symbol} 的买卖记录 market 不一致，仍按 symbol 执行 FIFO 配对",
                        trade.source_index,
                        "market",
                    )
                )

            roundtrips.append(
                _Roundtrip(
                    sequence=len(roundtrips) + 1,
                    symbol=trade.symbol,
                    name=lot.trade.name or trade.name,
                    market=lot.trade.market,
                    buy_dt=lot.trade.dt,
                    sell_dt=trade.dt,
                    quantity=take,
                    buy_price=lot.trade.price,
                    sell_price=trade.price,
                    buy_fee=buy_fee,
                    sell_fee=sell_fee,
                    gross_pnl=gross_pnl,
                    pnl=pnl,
                )
            )

            lot.remaining_quantity -= take
            lot.remaining_fee -= buy_fee
            remaining_sell -= take
            if lot.remaining_quantity <= _EPSILON:
                queue.popleft()

        if remaining_sell > _EPSILON:
            unmatched_fee = trade.fee * (remaining_sell / trade.quantity)
            unmatched_sells.append(
                {
                    "symbol": trade.symbol,
                    "datetime": _iso(trade.dt),
                    "quantity": _rounded(remaining_sell, 8),
                    "price": _rounded(trade.price, 8),
                    "fee": _rounded(unmatched_fee, 8),
                    "source_index": trade.source_index,
                }
            )
            warnings.append(
                _warning(
                    "unmatched_sell_quantity",
                    f"{trade.symbol} 有 {_format_number(remaining_sell)} 股卖出找不到更早的买入记录，未计入已实现盈亏",
                    trade.source_index,
                    "quantity",
                )
            )

    open_lots: list[dict[str, Any]] = []
    for symbol in sorted(queues):
        for lot in queues[symbol]:
            if lot.remaining_quantity <= _EPSILON:
                continue
            open_lots.append(
                {
                    "symbol": symbol,
                    "name": lot.trade.name,
                    "market": lot.trade.market,
                    "buy_dt": _iso(lot.trade.dt),
                    "quantity": _rounded(lot.remaining_quantity, 8),
                    "buy_price": _rounded(lot.trade.price, 8),
                    "remaining_buy_fee": _rounded(max(lot.remaining_fee, 0.0), 8),
                    "cost_basis": _rounded(lot.remaining_quantity * lot.trade.price + max(lot.remaining_fee, 0.0), 2),
                }
            )
    return roundtrips, open_lots, unmatched_sells


def _compute_profile(
    trades: list[_Trade],
    roundtrips: list[_Roundtrip],
    open_lots: list[dict[str, Any]],
    unmatched_sells: list[dict[str, Any]],
) -> dict[str, Any]:
    if trades:
        span_days = max(1, int((trades[-1].dt - trades[0].dt).total_seconds() // 86_400))
        trade_frequency = len(trades) / span_days * 7
    else:
        trade_frequency = 0.0

    wins = [trip for trip in roundtrips if trip.pnl > _EPSILON]
    losses = [trip for trip in roundtrips if trip.pnl < -_EPSILON]
    breakeven = len(roundtrips) - len(wins) - len(losses)
    avg_win = _mean([trip.pnl for trip in wins])
    avg_loss = _mean([trip.pnl for trip in losses])
    if losses:
        profit_loss_ratio: float | None = avg_win / abs(avg_loss) if wins else 0.0
    else:
        profit_loss_ratio = None

    hold_days = [trip.hold_days for trip in roundtrips]
    total_pnl = sum(trip.pnl for trip in roundtrips)
    realized_fees = sum(trip.buy_fee + trip.sell_fee for trip in roundtrips)
    open_buy_fees = sum(float(lot["remaining_buy_fee"]) for lot in open_lots)
    unmatched_sell_fees = sum(float(sell["fee"]) for sell in unmatched_sells)

    cumulative = 0.0
    peak = 0.0
    max_drawdown = 0.0
    for trip in sorted(roundtrips, key=lambda item: (item.sell_dt, item.sequence)):
        cumulative += trip.pnl
        peak = max(peak, cumulative)
        max_drawdown = min(max_drawdown, cumulative - peak)

    by_symbol: dict[str, dict[str, float | int]] = defaultdict(lambda: {"trades": 0, "total_amount": 0.0})
    market_counts: dict[str, int] = defaultdict(int)
    hour_counts: dict[int, int] = defaultdict(int)
    for trade in trades:
        by_symbol[trade.symbol]["trades"] = int(by_symbol[trade.symbol]["trades"]) + 1
        by_symbol[trade.symbol]["total_amount"] = float(by_symbol[trade.symbol]["total_amount"]) + trade.amount
        market_counts[trade.market] += 1
        hour_counts[trade.dt.hour] += 1

    top_symbols = [
        {
            "symbol": symbol,
            "trades": int(values["trades"]),
            "total_amount": _rounded(float(values["total_amount"]), 2),
        }
        for symbol, values in sorted(
            by_symbol.items(),
            key=lambda item: (-float(item[1]["total_amount"]), item[0]),
        )[:10]
    ]
    preferred_markets = [
        market for market, _ in sorted(market_counts.items(), key=lambda item: (-item[1], item[0]))
    ]

    return {
        "total_trades": len(trades),
        "total_roundtrips": len(roundtrips),
        "winning_roundtrips": len(wins),
        "losing_roundtrips": len(losses),
        "breakeven_roundtrips": breakeven,
        "closed_quantity": _rounded(sum(trip.quantity for trip in roundtrips), 8),
        "open_quantity": _rounded(sum(float(lot["quantity"]) for lot in open_lots), 8),
        "unmatched_sell_quantity": _rounded(sum(float(sell["quantity"]) for sell in unmatched_sells), 8),
        "avg_holding_days": _rounded(_mean(hold_days), 2),
        "median_holding_days": _rounded(statistics.median(hold_days), 2) if hold_days else 0.0,
        "p75_holding_days": _rounded(_quantile(hold_days, 0.75), 2) if hold_days else 0.0,
        "trade_frequency_per_week": _rounded(trade_frequency, 2),
        "win_rate": _rounded(len(wins) / len(roundtrips), 4) if roundtrips else None,
        "profit_loss_ratio": _rounded(profit_loss_ratio, 2) if profit_loss_ratio is not None else None,
        "gross_profit": _rounded(sum(trip.pnl for trip in wins), 2),
        "gross_loss": _rounded(sum(trip.pnl for trip in losses), 2),
        "total_pnl": _rounded(total_pnl, 2),
        "max_drawdown": _rounded(max_drawdown, 2),
        "total_input_fees": _rounded(sum(trade.fee for trade in trades), 2),
        "realized_fees": _rounded(realized_fees, 2),
        "open_lot_fees": _rounded(open_buy_fees, 2),
        "unmatched_sell_fees": _rounded(unmatched_sell_fees, 2),
        "top_symbols": top_symbols,
        "market_distribution": {key: market_counts[key] for key in sorted(market_counts)},
        "preferred_markets": preferred_markets,
        "hourly_distribution": {str(key): hour_counts[key] for key in sorted(hour_counts)},
    }


def _compute_behavior(trades: list[_Trade], roundtrips: list[_Roundtrip]) -> dict[str, Any]:
    return {
        "disposition_effect": _disposition_effect(roundtrips),
        "overtrading": _overtrading(trades, roundtrips),
        "chasing_momentum": _chasing_momentum(trades),
        "anchoring": _anchoring(trades),
    }


def _disposition_effect(roundtrips: list[_Roundtrip]) -> dict[str, Any]:
    wins = [trip for trip in roundtrips if trip.pnl > _EPSILON]
    losses = [trip for trip in roundtrips if trip.pnl < -_EPSILON]
    if not wins or not losses:
        return _insufficient("至少需要一个盈利回合和一个亏损回合才能比较持仓时间")

    win_hold = _mean([trip.hold_days for trip in wins])
    loss_hold = _mean([trip.hold_days for trip in losses])
    if win_hold <= _EPSILON:
        ratio: float | None = None
        severity = "high" if loss_hold > _EPSILON else "low"
    else:
        ratio = loss_hold / win_hold
        severity = _severity(ratio, 1.2, 1.5)
    ratio_text = "不可比" if ratio is None else f"{ratio:.2f} 倍"
    return {
        "status": "ok",
        "severity": severity,
        "ratio_loss_to_win_hold": _rounded(ratio, 2) if ratio is not None else None,
        "avg_winner_hold_days": _rounded(win_hold, 2),
        "avg_loser_hold_days": _rounded(loss_hold, 2),
        "evidence": f"亏损回合平均持有 {loss_hold:.1f} 天，盈利回合 {win_hold:.1f} 天，比例 {ratio_text}",
    }


def _overtrading(trades: list[_Trade], roundtrips: list[_Roundtrip]) -> dict[str, Any]:
    if not trades or not roundtrips:
        return _insufficient("缺少闭合回合，无法比较高频与低频交易日")

    daily_counts: dict[str, int] = defaultdict(int)
    for trade in trades:
        daily_counts[trade.dt.date().isoformat()] += 1
    if len(daily_counts) < 4:
        return _insufficient("少于 4 个交易日，无法稳定判断过度交易")

    counts = [float(count) for count in daily_counts.values()]
    busy_cut = _quantile(counts, 0.75)
    quiet_cut = _quantile(counts, 0.25)
    busy_days = {day for day, count in daily_counts.items() if count >= busy_cut}
    quiet_days = {day for day, count in daily_counts.items() if count <= quiet_cut} - busy_days
    if not quiet_days:
        return _insufficient("各交易日活跃度过于接近，无法划分高频日和低频日")

    busy_pnl = [trip.pnl for trip in roundtrips if trip.sell_dt.date().isoformat() in busy_days]
    quiet_pnl = [trip.pnl for trip in roundtrips if trip.sell_dt.date().isoformat() in quiet_days]
    if not busy_pnl or not quiet_pnl:
        return _insufficient("闭合回合未同时覆盖高频日与低频日")

    busy_avg = _mean(busy_pnl)
    quiet_avg = _mean(quiet_pnl)
    gap = quiet_avg - busy_avg
    base = abs(quiet_avg) if abs(quiet_avg) > _EPSILON else 1.0
    severity = _severity(gap / base, 0.3, 1.0) if busy_avg < quiet_avg else "low"
    return {
        "status": "ok",
        "severity": severity,
        "busy_day_avg_pnl": _rounded(busy_avg, 2),
        "quiet_day_avg_pnl": _rounded(quiet_avg, 2),
        "busy_day_trade_threshold": _rounded(busy_cut, 1),
        "quiet_day_trade_threshold": _rounded(quiet_cut, 1),
        "evidence": f"高频日平均盈亏 {_signed_number(busy_avg)}，低频日 {_signed_number(quiet_avg)}",
    }


def _chasing_momentum(trades: list[_Trade]) -> dict[str, Any]:
    buys_by_symbol: dict[str, list[_Trade]] = defaultdict(list)
    for trade in trades:
        if trade.side == "buy":
            buys_by_symbol[trade.symbol].append(trade)

    evaluated = 0
    chased = 0
    for symbol in sorted(buys_by_symbol):
        buys = sorted(buys_by_symbol[symbol], key=lambda trade: (trade.dt, trade.source_index))
        for index in range(3, len(buys)):
            evaluated += 1
            if buys[index].price > buys[index - 3].price * 1.03:
                chased += 1
    if evaluated == 0:
        return _insufficient("同一标的少于 4 次买入，无法判断追涨倾向")

    ratio = chased / evaluated
    return {
        "status": "ok",
        "severity": _severity(ratio, 0.4, 0.6),
        "chase_ratio": _rounded(ratio, 3),
        "buys_evaluated": evaluated,
        "chasing_buys": chased,
        "evidence": f"{chased}/{evaluated} 次可评估买入发生在同标的价格较前三次买入上涨超过 3% 后",
    }


def _anchoring(trades: list[_Trade]) -> dict[str, Any]:
    by_symbol: dict[str, list[float]] = defaultdict(list)
    for trade in trades:
        by_symbol[trade.symbol].append(trade.price)

    evaluated: list[dict[str, Any]] = []
    for symbol in sorted(by_symbol):
        prices = by_symbol[symbol]
        if len(prices) < 5:
            continue
        mean_price = _mean(prices)
        if mean_price <= _EPSILON:
            continue
        cv = statistics.stdev(prices) / mean_price
        evaluated.append(
            {
                "symbol": symbol,
                "trades": len(prices),
                "mean_price": _rounded(mean_price, 2),
                "cv": _rounded(cv, 4),
            }
        )
    if not evaluated:
        return _insufficient("没有标的达到 5 笔成交，无法判断价格锚定")

    anchored = [row for row in evaluated if float(row["cv"]) < 0.05]
    ratio = len(anchored) / len(evaluated)
    return {
        "status": "ok",
        "severity": _severity(ratio, 0.33, 0.66),
        "anchored_symbol_ratio": _rounded(ratio, 3),
        "symbols_evaluated": len(evaluated),
        "anchored_symbols": anchored[:5],
        "evidence": f"{len(anchored)}/{len(evaluated)} 个高频交易标的的成交价格变异系数低于 5%",
    }


def _compute_per_symbol(
    trades: list[_Trade],
    roundtrips: list[_Roundtrip],
    open_lots: list[dict[str, Any]],
    unmatched_sells: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    trade_groups: dict[str, list[_Trade]] = defaultdict(list)
    trip_groups: dict[str, list[_Roundtrip]] = defaultdict(list)
    open_quantity: dict[str, float] = defaultdict(float)
    unmatched_quantity: dict[str, float] = defaultdict(float)
    for trade in trades:
        trade_groups[trade.symbol].append(trade)
    for trip in roundtrips:
        trip_groups[trip.symbol].append(trip)
    for lot in open_lots:
        open_quantity[str(lot["symbol"])] += float(lot["quantity"])
    for sell in unmatched_sells:
        unmatched_quantity[str(sell["symbol"])] += float(sell["quantity"])

    rows: list[dict[str, Any]] = []
    symbols = sorted(set(trade_groups) | set(trip_groups) | set(open_quantity) | set(unmatched_quantity))
    for symbol in symbols:
        symbol_trades = trade_groups[symbol]
        trips = trip_groups[symbol]
        wins = [trip for trip in trips if trip.pnl > _EPSILON]
        losses = [trip for trip in trips if trip.pnl < -_EPSILON]
        rows.append(
            {
                "symbol": symbol,
                "name": next((trade.name for trade in symbol_trades if trade.name), ""),
                "market": next((trade.market for trade in symbol_trades), "other"),
                "trades": len(symbol_trades),
                "buy_trades": sum(1 for trade in symbol_trades if trade.side == "buy"),
                "sell_trades": sum(1 for trade in symbol_trades if trade.side == "sell"),
                "roundtrips": len(trips),
                "closed_quantity": _rounded(sum(trip.quantity for trip in trips), 8),
                "open_quantity": _rounded(open_quantity[symbol], 8),
                "unmatched_sell_quantity": _rounded(unmatched_quantity[symbol], 8),
                "win_rate": _rounded(len(wins) / len(trips), 4) if trips else None,
                "realized_pnl": _rounded(sum(trip.pnl for trip in trips), 2),
                "avg_holding_days": _rounded(_mean([trip.hold_days for trip in trips]), 2),
                "total_amount": _rounded(sum(trade.amount for trade in symbol_trades), 2),
                "total_input_fees": _rounded(sum(trade.fee for trade in symbol_trades), 2),
                "winning_roundtrips": len(wins),
                "losing_roundtrips": len(losses),
            }
        )
    return rows


def _build_insights(
    profile: dict[str, Any],
    behavior: dict[str, Any],
    per_symbol: list[dict[str, Any]],
    open_lots: list[dict[str, Any]],
    unmatched_sells: list[dict[str, Any]],
    *,
    mixed_markets: bool,
) -> tuple[list[str], list[dict[str, Any]]]:
    overall: list[str] = []
    total_trades = int(profile["total_trades"])
    total_roundtrips = int(profile["total_roundtrips"])

    if total_trades == 0:
        overall.extend(["没有可用于复盘的有效成交记录", "请先补齐成交时间、标的、方向、数量和价格"])
    elif total_roundtrips == 0:
        overall.append(f"已读取 {total_trades} 笔有效成交，但没有形成完整的 FIFO 买卖回合")
    else:
        win_rate = float(profile["win_rate"] or 0.0)
        pnl = float(profile["total_pnl"])
        ratio = profile["profit_loss_ratio"]
        ratio_text = "暂无可比亏损样本" if ratio is None else f"盈亏比 {float(ratio):.2f}"
        overall.append(
            f"{total_roundtrips} 个闭合回合已实现盈亏 {_signed_number(pnl)}，胜率 {win_rate:.0%}，{ratio_text}"
        )
        if total_roundtrips < 5:
            overall.append("闭合样本少于 5 个，行为结论仅作提示，不宜据此固化交易规则")

    open_qty = float(profile["open_quantity"])
    if open_qty > _EPSILON:
        overall.append(f"仍有 {_format_number(open_qty)} 股/份未平仓，未实现盈亏不在本次统计内")
    unmatched_qty = float(profile["unmatched_sell_quantity"])
    if unmatched_qty > _EPSILON:
        overall.append(f"有 {_format_number(unmatched_qty)} 股/份卖出缺少更早买入，需补齐历史成交")

    behavior_labels = {
        "disposition_effect": "处置效应：亏损持仓时间偏长",
        "overtrading": "过度交易：高频交易日表现更差",
        "chasing_momentum": "追涨倾向：上涨后重复买入较多",
        "anchoring": "价格锚定：重复在狭窄价位交易",
    }
    for key in ("disposition_effect", "overtrading", "chasing_momentum", "anchoring"):
        diagnostic = behavior[key]
        if diagnostic.get("severity") == "high":
            overall.append("高风险行为信号——" + behavior_labels[key])
        elif diagnostic.get("severity") == "medium":
            overall.append("需关注的行为信号——" + behavior_labels[key])

    total_amount = sum(float(row["total_amount"]) for row in per_symbol)
    if len(per_symbol) > 1 and total_amount > _EPSILON:
        top = max(per_symbol, key=lambda row: (float(row["total_amount"]), str(row["symbol"])))
        share = float(top["total_amount"]) / total_amount
        if share >= 0.6:
            overall.append(f"成交额较集中：{top['symbol']} 占全部成交额 {share:.0%}")

    if mixed_markets:
        overall.append("跨市场合计未换算币种，应分别查看各市场盈亏后再比较")

    symbol_insights: list[dict[str, Any]] = []
    for row in per_symbol:
        items: list[str] = []
        roundtrip_count = int(row["roundtrips"])
        if roundtrip_count:
            pnl = float(row["realized_pnl"])
            win_rate = float(row["win_rate"] or 0.0)
            items.append(f"{roundtrip_count} 个闭合回合，已实现盈亏 {_signed_number(pnl)}，胜率 {win_rate:.0%}")
        else:
            items.append("尚无完整买卖回合，不能评估该标的的已实现表现")
        if float(row["open_quantity"]) > _EPSILON:
            items.append(f"仍有 {_format_number(float(row['open_quantity']))} 股/份未平仓")
        if float(row["unmatched_sell_quantity"]) > _EPSILON:
            items.append(f"有 {_format_number(float(row['unmatched_sell_quantity']))} 股/份卖出未配对")
        symbol_insights.append({"symbol": row["symbol"], "items": items})

    return overall, symbol_insights


def _build_reflection_text(insights: list[str]) -> str:
    if not insights:
        return "暂无足够信息形成交易复盘。"
    selected = insights[:4]
    return "。".join(item.rstrip("。") for item in selected) + "。"


def _roundtrip_to_dict(trip: _Roundtrip) -> dict[str, Any]:
    return {
        "sequence": trip.sequence,
        "symbol": trip.symbol,
        "name": trip.name,
        "market": trip.market,
        "buy_dt": _iso(trip.buy_dt),
        "sell_dt": _iso(trip.sell_dt),
        "qty": _rounded(trip.quantity, 8),
        "buy_price": _rounded(trip.buy_price, 8),
        "sell_price": _rounded(trip.sell_price, 8),
        "hold_days": _rounded(trip.hold_days, 2),
        "gross_pnl": _rounded(trip.gross_pnl, 2),
        "buy_fee": _rounded(trip.buy_fee, 8),
        "sell_fee": _rounded(trip.sell_fee, 8),
        "total_fee": _rounded(trip.buy_fee + trip.sell_fee, 8),
        "pnl": _rounded(trip.pnl, 2),
        "pnl_pct": _rounded(trip.pnl_pct, 4),
    }


def _parse_datetime(value: Any) -> datetime | None:
    if isinstance(value, datetime):
        parsed = value
    else:
        text = str(value or "").strip()
        if not text:
            return None
        if text.endswith(("Z", "z")):
            text = text[:-1] + "+00:00"
        try:
            parsed = datetime.fromisoformat(text)
        except ValueError:
            return None
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def _finite_number(value: Any) -> float | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        number = float(str(value).replace(",", "").strip())
    except (TypeError, ValueError):
        return None
    return number if math.isfinite(number) else None


def _infer_market(symbol: str) -> str:
    upper = symbol.upper()
    if upper.endswith(".HK"):
        return "hk"
    if upper.endswith((".SH", ".SZ", ".BJ")):
        return "china_a"
    if "USDT" in upper or "USDC" in upper or "-USD" in upper:
        return "crypto"
    if upper.isascii() and upper.isalpha():
        return "us"
    return "other"


def _warning(
    code: str,
    message: str,
    record_index: int | None = None,
    field: str | None = None,
) -> dict[str, Any]:
    warning: dict[str, Any] = {"code": code, "message": message}
    if record_index is not None:
        warning["record_index"] = record_index
    if field is not None:
        warning["field"] = field
    return warning


def _insufficient(evidence: str) -> dict[str, Any]:
    return {"status": "insufficient_data", "severity": "unknown", "evidence": evidence}


def _severity(score: float, medium_cutoff: float, high_cutoff: float) -> str:
    if score >= high_cutoff:
        return "high"
    if score >= medium_cutoff:
        return "medium"
    return "low"


def _mean(values: Sequence[float]) -> float:
    return sum(values) / len(values) if values else 0.0


def _quantile(values: Sequence[float], quantile: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(float(value) for value in values)
    position = (len(ordered) - 1) * quantile
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return ordered[lower]
    fraction = position - lower
    return ordered[lower] * (1 - fraction) + ordered[upper] * fraction


def _rounded(value: float, digits: int) -> float:
    rounded = round(float(value), digits)
    return 0.0 if abs(rounded) < 10 ** (-digits) / 2 else rounded


def _iso(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def _format_number(value: float) -> str:
    return f"{value:,.4f}".rstrip("0").rstrip(".")


def _signed_number(value: float) -> str:
    return f"{value:+,.2f}"


__all__ = [
    "INPUT_SCHEMA_VERSION",
    "REFLECTION_SCHEMA_VERSION",
    "analyze_trade_records",
    "build_journal_reflection",
]
