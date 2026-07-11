"""Turn a Vibe-Trading :class:`BacktestResult` into TradingAgents feedback.

TradingAgents' ``TradingMemoryLog`` resolves a pending decision with three
numeric slots — ``raw_return``, ``alpha_return``, ``holding_days`` — plus a
free-text ``reflection`` block. On the next run for the same ticker the
Portfolio Manager prompt is seeded with those resolved entries (see
``memory.get_past_context``), so whatever we pack into the reflection prose is
exactly what the PM reads as "what happened last time".

The semantic we adopt (matching the integration design): the numbers written
back are a **historical backtest validation** of the decision's signal, not a
realised forward return. The prose says so explicitly, so the PM never mistakes
a backtest for a booked trade. The three numeric slots map as:

* ``raw_return``   <- backtest ``total_return``
* ``alpha_return`` <- backtest ``excess_return`` (vs the benchmark), falling back
  to ``total_return - benchmark_return`` when the engine did not emit it
* ``holding_days`` <- the spec's intended holding horizon (the tag is a label,
  not a per-trade average)
"""

from __future__ import annotations

from dataclasses import dataclass

from .backtest_bridge import BacktestResult
from .signal_spec import SignalSpec

# The memory-log separator is an HTML comment an LLM can't emit; make sure our
# generated prose can never accidentally contain it either.
_FORBIDDEN = "<!-- ENTRY_END -->"


@dataclass
class ReflectionPayload:
    """The four values ``TradingMemoryLog.update_with_outcome`` needs.

    Attributes:
        raw_return / alpha_return: decimals (0.12 == +12%); the log formats
            them as percentages in the entry tag.
        holding_days: horizon label for the tag.
        reflection: plain-prose lesson injected into the next PM prompt.
    """

    raw_return: float
    alpha_return: float
    holding_days: int
    reflection: str

    def as_update(self, ticker: str, trade_date: str) -> dict:
        """Shape one element for ``batch_update_with_outcomes``."""
        return {
            "ticker": ticker,
            "trade_date": trade_date,
            "raw_return": self.raw_return,
            "alpha_return": self.alpha_return,
            "holding_days": self.holding_days,
            "reflection": self.reflection,
        }


def _fmt_pct(x: float | None) -> str:
    return "n/a" if x is None else f"{x:+.1%}"


def _fmt_num(x: float | None, nd: int = 2) -> str:
    return "n/a" if x is None else f"{x:.{nd}f}"


def _alpha_return(result: BacktestResult) -> float:
    """Excess return vs benchmark, with a fallback when not emitted."""
    excess = result.metrics.get("excess_return")
    if isinstance(excess, (int, float)):
        return float(excess)
    total = result.metrics.get("total_return")
    bench = result.metrics.get("benchmark_return")
    if isinstance(total, (int, float)) and isinstance(bench, (int, float)):
        return float(total) - float(bench)
    return 0.0


def _verdict(result: BacktestResult) -> str:
    """One-line judgement combining significance, consistency, and drawdown.

    The point is to tell the PM whether last time's *signal* held up under
    scrutiny, not just whether it made money once.
    """
    sharpe = result.sharpe
    mc_p = result.mc_p_value_sharpe
    wf = result.walk_forward_consistency

    # Statistical significance gate first: a good Sharpe that a permutation test
    # says is indistinguishable from random ordering is not worth trusting.
    if sharpe is not None and sharpe > 0.8 and (mc_p is None or mc_p < 0.05):
        strength = "held up"
    elif sharpe is not None and sharpe > 0:
        strength = "was weak but positive"
    else:
        strength = "did not hold up"

    bits = [f"The signal {strength} in backtest"]
    if mc_p is not None:
        sig = "significant" if mc_p < 0.05 else "NOT significant (looks random)"
        bits.append(f"Monte Carlo p={mc_p:.2f} ({sig})")
    if wf is not None:
        bits.append(f"walk-forward consistency {wf:.0%}")
    dd = result.max_drawdown
    if dd is not None:
        bits.append(f"max drawdown {dd:.1%}")
    return "; ".join(bits) + "."


def build_reflection(
    spec: SignalSpec,
    result: BacktestResult,
    *,
    start_date: str = "",
    end_date: str = "",
) -> ReflectionPayload:
    """Compose the numeric slots + prose lesson from a backtest result.

    Args:
        spec: the signal spec that was backtested.
        result: the backtest outcome.
        start_date / end_date: the validation window, named in the prose so the
            PM knows how much history backs the verdict (a 3-year window is
            stronger evidence than a 3-month one).

    Falls back to an explicit "could not be validated" reflection when the
    backtest failed, so a failed validation still teaches the PM something
    (and never silently reports a fake +0.0% success).
    """
    window = f" over {start_date}..{end_date}" if start_date and end_date else ""
    if not result.ok:
        reason = (result.error or "unknown error").strip().replace("\n", " ")
        reflection = (
            f"BACKTEST VALIDATION FAILED for the {spec.rating} call on "
            f"{spec.ticker}{window}: {reason}. Treat the prior decision as "
            "unverified — no historical evidence either way."
        )
        return ReflectionPayload(0.0, 0.0, spec.holding_days, _clean(reflection))

    raw = result.total_return if result.total_return is not None else 0.0
    alpha = _alpha_return(result)

    header = (
        f"HISTORICAL BACKTEST of this {spec.rating} signal on {spec.ticker}"
        f"{window} ({spec.summary_conditions()}): "
        f"total return {_fmt_pct(result.total_return)}, "
        f"excess vs benchmark {_fmt_pct(alpha)}, "
        f"Sharpe {_fmt_num(result.sharpe)}, "
        f"win rate {_fmt_pct(result.win_rate)} over "
        f"{result.trade_count if result.trade_count is not None else 'n/a'} trades."
    )
    verdict = _verdict(result)
    guidance = (
        "This is a historical simulation of the decision's signal, not a booked "
        "return — weight it as evidence about the signal's edge, and size or "
        "skip accordingly next time."
    )
    reflection = f"{header} {verdict} {guidance}"
    return ReflectionPayload(
        raw_return=round(float(raw), 4),
        alpha_return=round(float(alpha), 4),
        holding_days=spec.holding_days,
        reflection=_clean(reflection),
    )


def _clean(text: str) -> str:
    """Guard against the memory-log separator leaking into prose."""
    return text.replace(_FORBIDDEN, "[entry-end]").strip()
