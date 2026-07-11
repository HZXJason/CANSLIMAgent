"""Parse a TradingAgents decision into a quantifiable SignalSpec.

TradingAgents' ``propagate()`` returns a markdown ``final_trade_decision`` plus
a plain 5-tier rating string (Buy / Overweight / Hold / Underweight / Sell).
That prose is the *only* durable output — the Pydantic objects behind it
(price target, entry price, sizing) are rendered to markdown and thrown away.

To validate a decision against history we need a *number*, so this module turns
the decision back into a structured ``SignalSpec``:

* ``rating`` -> signed target weight (the always-available signal)
* ``**Position Sizing**`` / ``**Price Target**`` / ``**Entry Price**`` /
  ``**Stop Loss**`` / ``**Time Horizon**`` -> parsed scalars when present
* optional entry conditions (RSI / MACD crossover / MA) extracted from the
  prose, so a decision like "buy AAPL when RSI < 30 and MACD golden-crosses"
  becomes a conditional backtest instead of a blind directional one.

Pure standard library — no pandas, no imports from either repo — so it is fully
unit-testable on its own.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Iterable

# --- rating -> directional weight -------------------------------------------

# Signed default magnitude for each 5-tier rating. Overweight/Underweight are
# half-weight tilts; Buy/Sell are full-weight; Hold is flat.
RATING_WEIGHT: dict[str, float] = {
    "Buy": 1.0,
    "Overweight": 0.5,
    "Hold": 0.0,
    "Underweight": -0.5,
    "Sell": -1.0,
}

_RATING_SET = {r.lower(): r for r in RATING_WEIGHT}

# "Rating: **Buy**", "rating - Sell", tolerant of markdown bold.
_RATING_LABEL_RE = re.compile(r"rating.*?[:\-][\s*]*([A-Za-z]+)", re.IGNORECASE)

# "**Price Target**: 215.0", "**Entry Price**: 189.5", "**Stop Loss**: 175"
_LABELLED_FLOAT = r"\*{{0,2}}{label}\*{{0,2}}\s*[:\-]\s*\*{{0,2}}\$?\s*([0-9]+(?:\.[0-9]+)?)"


def _find_labelled_float(text: str, label: str) -> float | None:
    m = re.search(_LABELLED_FLOAT.format(label=re.escape(label)), text, re.IGNORECASE)
    if not m:
        return None
    try:
        return float(m.group(1))
    except ValueError:
        return None


def parse_rating(text: str, default: str = "Hold") -> str:
    """Extract a 5-tier rating from decision prose.

    Mirrors TradingAgents' own ``rating.parse_rating`` two-pass heuristic so the
    bridge and the framework always agree on the tier: first an explicit
    ``Rating: X`` label, then the first rating word anywhere, else ``default``.
    """
    for line in text.splitlines():
        m = _RATING_LABEL_RE.search(line)
        if m and m.group(1).lower() in _RATING_SET:
            return _RATING_SET[m.group(1).lower()]
    for line in text.splitlines():
        for word in line.lower().split():
            clean = word.strip("*:.,")
            if clean in _RATING_SET:
                return _RATING_SET[clean]
    return default


def _parse_position_fraction(text: str) -> float | None:
    """Pull a position size fraction from sizing prose, e.g. '15% of portfolio'.

    Returns a fraction in (0, 1], or None when no percentage is stated. Values
    above 100% are clamped to 1.0 (the engine normalizes gross exposure anyway).
    """
    m = re.search(
        r"\*{0,2}position\s+sizing\*{0,2}\s*[:\-]?\s*.*?([0-9]+(?:\.[0-9]+)?)\s*%",
        text,
        re.IGNORECASE,
    )
    if not m:
        # Fall back to any "NN%" near the word "position"/"portfolio".
        m = re.search(
            r"([0-9]+(?:\.[0-9]+)?)\s*%\s*(?:of\s+)?(?:the\s+)?portfolio",
            text,
            re.IGNORECASE,
        )
    if not m:
        return None
    frac = float(m.group(1)) / 100.0
    if frac <= 0:
        return None
    return min(frac, 1.0)


# --- horizon -> holding days ------------------------------------------------

_MONTH_RE = re.compile(r"([0-9]+)\s*(?:-\s*[0-9]+\s*)?month", re.IGNORECASE)
_WEEK_RE = re.compile(r"([0-9]+)\s*(?:-\s*[0-9]+\s*)?week", re.IGNORECASE)
_DAY_RE = re.compile(r"([0-9]+)\s*(?:-\s*[0-9]+\s*)?(?:trading\s+)?day", re.IGNORECASE)
_YEAR_RE = re.compile(r"([0-9]+)\s*(?:-\s*[0-9]+\s*)?year", re.IGNORECASE)


def parse_holding_days(time_horizon: str | None, default: int = 21) -> int:
    """Convert a free-text horizon ('3-6 months', '2 weeks') to trading days.

    Uses the *lower* bound of any range (conservative holding period) and
    approximates 21 trading days per month, 5 per week, 252 per year.
    """
    if not time_horizon:
        return default
    t = time_horizon.strip()
    if m := _YEAR_RE.search(t):
        return int(m.group(1)) * 252
    if m := _MONTH_RE.search(t):
        return int(m.group(1)) * 21
    if m := _WEEK_RE.search(t):
        return int(m.group(1)) * 5
    if m := _DAY_RE.search(t):
        return int(m.group(1))
    return default


# --- entry conditions -------------------------------------------------------


@dataclass(frozen=True)
class EntryCondition:
    """A single gate that must hold for a bar to be an entry.

    ``kind`` is one of: ``rsi_below``, ``rsi_above``, ``macd_golden_cross``,
    ``macd_death_cross``, ``price_above_sma``, ``price_below_sma``. ``value`` is
    the threshold (RSI level or SMA window); crossover kinds ignore it.
    """

    kind: str
    value: float | None = None

    def describe(self) -> str:
        if self.kind == "rsi_below":
            return f"RSI(14) < {self.value:g}"
        if self.kind == "rsi_above":
            return f"RSI(14) > {self.value:g}"
        if self.kind == "macd_golden_cross":
            return "MACD golden cross"
        if self.kind == "macd_death_cross":
            return "MACD death cross"
        if self.kind == "price_above_sma":
            return f"close > SMA({int(self.value)})"
        if self.kind == "price_below_sma":
            return f"close < SMA({int(self.value)})"
        return self.kind


def extract_entry_conditions(text: str) -> list[EntryCondition]:
    """Best-effort extraction of technical entry gates from decision prose.

    Recognizes the common phrasings an analyst or trader writes: RSI thresholds,
    MACD golden/death crosses (English or 金叉/死叉), and moving-average
    relationships. Returns an empty list when none are found — in that case the
    backtest is a pure directional test of the rating.
    """
    conditions: list[EntryCondition] = []
    lowered = text.lower()

    # RSI thresholds: "RSI < 30", "RSI(14) below 30", "rsi under 35", and prose
    # phrasings with a few filler words in between ("RSI reading below 30", "RSI
    # is currently under 35", "RSI dips below 30"). The filler run is lazy and
    # alphabetic-only ({0,3}) so it can't swallow the threshold number or match
    # across unrelated prose where the comparator is many words away.
    for m in re.finditer(
        r"rsi(?:\s*\(?\d+\)?)?(?:\s+[a-z]+){0,3}?\s*(<|below|under|<=|>|above|over|>=)\s*([0-9]+(?:\.[0-9]+)?)",
        lowered,
    ):
        op, val = m.group(1), float(m.group(2))
        if op in ("<", "below", "under", "<="):
            conditions.append(EntryCondition("rsi_below", val))
        else:
            conditions.append(EntryCondition("rsi_above", val))

    # MACD crosses — English and Chinese. Use a bounded DOTALL gap so a
    # cross phrase wrapped across a newline ("a MACD\ngolden cross" — common in
    # real LLM prose) is still detected, while the {0,40} bound keeps a distant,
    # unrelated "cross" from false-matching a far-away "macd".
    if (
        re.search(r"macd.{0,40}(golden\s*cross|bullish\s*cross|cross(?:es)?\s*above)", lowered, re.DOTALL)
        or "金叉" in text
    ):
        conditions.append(EntryCondition("macd_golden_cross"))
    if (
        re.search(r"macd.{0,40}(death\s*cross|bearish\s*cross|cross(?:es)?\s*below)", lowered, re.DOTALL)
        or "死叉" in text
    ):
        conditions.append(EntryCondition("macd_death_cross"))

    # Price vs SMA: "above the 50-day moving average", "below 200-day MA"
    for m in re.finditer(
        r"(above|below)\s*(?:the\s*)?([0-9]+)[\s\-]*day\s*(?:moving\s*average|ma|sma)",
        lowered,
    ):
        side, window = m.group(1), float(m.group(2))
        kind = "price_above_sma" if side == "above" else "price_below_sma"
        conditions.append(EntryCondition(kind, window))

    # De-duplicate while preserving order.
    seen: set[tuple[str, float | None]] = set()
    unique: list[EntryCondition] = []
    for c in conditions:
        key = (c.kind, c.value)
        if key not in seen:
            seen.add(key)
            unique.append(c)
    return unique


# --- symbol normalization: TradingAgents -> Vibe-Trading --------------------


def to_vibe_symbol(ticker: str) -> str:
    """Map a TradingAgents ticker to the Vibe-Trading symbol convention.

    TradingAgents uses Yahoo suffixes (``AAPL``, ``0700.HK``, ``600519.SS``);
    Vibe-Trading uses ``AAPL.US`` for US equities, 5-digit ``00700.HK`` for HK,
    and ``.SH`` (not ``.SS``) for Shanghai. Crypto ``BTC-USD`` maps to the OKX
    ``BTC-USDT`` pair. Unknown formats pass through unchanged.
    """
    t = ticker.strip().upper()
    if not t:
        return t
    # Crypto: BTC-USD / ETH-USD -> BTC-USDT
    if t.endswith("-USD"):
        return t[:-4] + "-USDT"
    if "." not in t and "-" not in t:
        # Bare alphabetic ticker: treat as US equity.
        return f"{t}.US"
    if t.endswith(".SS"):
        return t[:-3] + ".SH"
    if t.endswith(".HK"):
        base = t[:-3]
        return base.zfill(5) + ".HK"
    return t


# --- the spec ---------------------------------------------------------------


@dataclass
class SignalSpec:
    """A backtest-ready, quantified view of one TradingAgents decision.

    Attributes:
        ticker: Vibe-Trading symbol (e.g. ``AAPL.US``).
        rating: 5-tier rating string.
        target_weight: Signed portfolio weight, magnitude from position sizing
            when stated, else the rating default. Positive = long.
        entry_price / stop_loss / price_target: parsed scalars or None.
        time_horizon: free-text horizon string or None.
        holding_days: horizon converted to trading days.
        entry_conditions: technical gates that must all hold to enter; empty
            means enter directionally on the first available bar.
        stop_pct: stop distance as a fraction of entry price (e.g. 0.10 = a 10%
            adverse move), derived from ``entry_price`` and ``stop_loss``. This
            is what the backtest applies — a *relative* stop tracks each trade's
            actual fill, whereas the decision's absolute ``stop_loss`` price is
            only meaningful at the decision date, not across a multi-year window.
        source_ticker: the original TradingAgents ticker, before normalization.
        raw_decision: the verbatim decision markdown.
    """

    ticker: str
    rating: str
    target_weight: float
    entry_price: float | None = None
    stop_loss: float | None = None
    price_target: float | None = None
    time_horizon: str | None = None
    holding_days: int = 21
    entry_conditions: list[EntryCondition] = field(default_factory=list)
    confirm_window: int = 5
    stop_pct: float | None = None
    source_ticker: str = ""
    raw_decision: str = ""

    @property
    def direction(self) -> int:
        """+1 long, -1 short, 0 flat."""
        if self.target_weight > 0:
            return 1
        if self.target_weight < 0:
            return -1
        return 0

    @property
    def is_actionable(self) -> bool:
        """A Hold with no directional weight is not worth backtesting."""
        return self.direction != 0

    def summary_conditions(self) -> str:
        """Human-readable entry description, e.g. ``RSI(14) < 30; MACD golden cross``."""
        if not self.entry_conditions:
            return "directional (enter first bar)"
        return "; ".join(c.describe() for c in self.entry_conditions)

    def summary(self) -> str:
        return (
            f"{self.ticker} {self.rating} weight={self.target_weight:+.2f} "
            f"hold={self.holding_days}d entry=[{self.summary_conditions()}]"
        )


def spec_from_decision(
    ticker: str,
    final_trade_decision: str,
    *,
    trader_plan: str = "",
    extra_conditions: Iterable[EntryCondition] | None = None,
    default_holding_days: int = 21,
) -> SignalSpec:
    """Build a :class:`SignalSpec` from a TradingAgents decision.

    Args:
        ticker: TradingAgents ticker (any market); normalized to Vibe format.
        final_trade_decision: the PM's decision markdown (``**Rating**`` etc.).
        trader_plan: optional ``trader_investment_plan`` markdown; searched for
            ``**Entry Price**`` / ``**Stop Loss**`` / ``**Position Sizing**``
            that the PM decision usually omits.
        extra_conditions: caller-supplied entry gates, merged with any parsed
            from the prose (e.g. produced by a PM-prompt extension).
        default_holding_days: fallback when no time horizon is stated.

    Returns:
        A fully populated :class:`SignalSpec`.
    """
    combined = f"{final_trade_decision}\n\n{trader_plan}".strip()
    rating = parse_rating(final_trade_decision)

    # Magnitude: explicit sizing wins, else the rating's default weight.
    default_w = RATING_WEIGHT.get(rating, 0.0)
    sizing_frac = _parse_position_fraction(combined)
    if sizing_frac is not None and default_w != 0:
        target_weight = sizing_frac if default_w > 0 else -sizing_frac
    else:
        target_weight = default_w

    time_horizon = _find_time_horizon(final_trade_decision)
    holding_days = parse_holding_days(time_horizon, default=default_holding_days)

    conditions = extract_entry_conditions(combined)
    if extra_conditions:
        for c in extra_conditions:
            if (c.kind, c.value) not in {(x.kind, x.value) for x in conditions}:
                conditions.append(c)

    entry_price = _find_labelled_float(combined, "Entry Price")
    stop_loss = _find_labelled_float(combined, "Stop Loss")
    price_target = _find_labelled_float(combined, "Price Target")

    # The decision's stop is an absolute price *as of the decision date*. Applied
    # verbatim across a multi-year backtest it is meaningless (3y ago the price
    # may have been half of today's). Convert it to a fraction of the entry
    # anchor so it can be applied against each trade's *actual* fill price.
    stop_pct = _stop_pct_from_prices(
        entry_price=entry_price,
        stop_loss=stop_loss,
        price_target=price_target,
        is_long=target_weight > 0,
    )

    return SignalSpec(
        ticker=to_vibe_symbol(ticker),
        rating=rating,
        target_weight=round(target_weight, 4),
        entry_price=entry_price,
        stop_loss=stop_loss,
        price_target=price_target,
        stop_pct=stop_pct,
        time_horizon=time_horizon,
        holding_days=holding_days,
        entry_conditions=conditions,
        source_ticker=ticker,
        raw_decision=final_trade_decision,
    )


def _stop_pct_from_prices(
    *,
    entry_price: float | None,
    stop_loss: float | None,
    price_target: float | None,
    is_long: bool,
) -> float | None:
    """Return a positive adverse-move fraction (e.g. 0.10 = 10%) or ``None``.

    Anchor preference: an explicit entry price, else the price target as a proxy
    for "roughly where we think it trades now". Without any price anchor the stop
    cannot be made regime-independent, so we return ``None`` (no stop) rather than
    apply a nonsensical absolute level.
    """
    if stop_loss is None or stop_loss <= 0:
        return None
    anchor = entry_price if (entry_price and entry_price > 0) else price_target
    if not anchor or anchor <= 0:
        return None
    if is_long:
        # A long stop sits below entry; ignore a stop placed above it.
        if stop_loss >= anchor:
            return None
        return round((anchor - stop_loss) / anchor, 4)
    # A short stop sits above entry.
    if stop_loss <= anchor:
        return None
    return round((stop_loss - anchor) / anchor, 4)


_TIME_HORIZON_RE = re.compile(
    r"\*{0,2}Time\s+Horizon\*{0,2}\s*[:\-]\s*\*{0,2}([^\n*]+)", re.IGNORECASE
)


def _find_time_horizon(text: str) -> str | None:
    m = _TIME_HORIZON_RE.search(text)
    return m.group(1).strip() if m else None
