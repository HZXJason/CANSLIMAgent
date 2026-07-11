"""The verification closed loop: TradingAgents decision -> Vibe backtest -> writeback.

This is the highest-value integration point. TradingAgents produces a decision
and stops; this module runs that decision through Vibe-Trading's backtest engine
and writes the historical verdict back into TradingAgents' decision log, so the
Portfolio Manager sees "how did this kind of call actually perform" on the next
run for the same ticker.

Pipeline (:func:`verify_decision`):

    decision markdown ──parse──▶ SignalSpec
                                    │
                        store pending entry in trading_memory.md
                                    │
                          Vibe backtest (subprocess)
                                    │
                        BacktestResult ──▶ reflection prose + numeric slots
                                    │
                        resolve the pending entry (tag + REFLECTION)

The loop never imports either project. It reads the decision as text, shells out
to Vibe's Python for the backtest, and edits the markdown log in stdlib. To also
*produce* the decision from TradingAgents in one call, see
:func:`propagate_and_verify`, which shells out to TradingAgents' Python.

Ordering vs TradingAgents' own resolver: TradingAgents resolves pending entries
at the *start* of the next ``propagate()`` for a ticker. Running this loop right
after a decision (and before the next same-ticker ``propagate()``) makes the
backtest verdict win the single resolved slot. If you prefer to keep
TradingAgents' realized-return resolution and only *add* backtest context, point
``memory_log_path`` at a separate sidecar log (see README "two modes").
"""

from __future__ import annotations

import json
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from . import backtest_bridge
from .config import BridgeConfig
from .memory_writeback import MemoryWriteback
from .reflection import build_reflection
from .signal_spec import spec_from_decision


@dataclass
class LoopResult:
    """What the closed loop did, for logging and assertions.

    Attributes:
        ok: whether a backtest verdict was produced and written back.
        ticker: normalized Vibe symbol.
        spec_summary: one-line SignalSpec summary.
        backtest_ok: whether the backtest itself produced metrics.
        wrote_pending / resolved: memory-log side effects.
        reflection: the prose written into the log (empty on failure).
        run_dir: Vibe run directory for auditing.
        error: populated when ``ok`` is False.
    """

    ok: bool
    ticker: str
    spec_summary: str = ""
    backtest_ok: bool = False
    wrote_pending: bool = False
    resolved: bool = False
    reflection: str = ""
    run_dir: str = ""
    error: str = ""


def verify_decision(
    ticker: str,
    trade_date: str,
    final_trade_decision: str,
    start_date: str,
    end_date: str,
    *,
    trader_plan: str = "",
    cfg: BridgeConfig | None = None,
    write_back: bool = True,
    **backtest_kwargs: Any,
) -> LoopResult:
    """Run one decision through backtest verification and write the result back.

    Args:
        ticker: TradingAgents ticker (any market); normalized to Vibe format.
        trade_date: the decision date, used as the memory-log entry key.
        final_trade_decision: the PM decision markdown.
        start_date / end_date: the historical window to validate over. Use a
            multi-year window ending at ``trade_date`` so the signal is tested
            across regimes (this is the "past 3 years of similar conditions"
            idea from the integration design).
        trader_plan: optional trader markdown (entry/stop/sizing) to enrich the
            spec.
        cfg: bridge configuration (defaults to env-derived).
        write_back: when False, run the backtest but don't touch the log
            (dry run / sidecar analysis).
        **backtest_kwargs: forwarded to ``backtest_bridge.run_backtest``
            (e.g. ``source``, ``benchmark``, ``initial_cash``).

    Returns:
        A :class:`LoopResult`.
    """
    cfg = cfg or BridgeConfig.from_env()

    spec = spec_from_decision(ticker, final_trade_decision, trader_plan=trader_plan)
    result = LoopResult(ok=False, ticker=spec.ticker, spec_summary=spec.summary())

    if not spec.is_actionable:
        result.error = "decision is Hold / non-actionable — nothing to verify"
        return result

    log = MemoryWriteback(cfg.memory_log_path) if write_back else None
    if log is not None:
        result.wrote_pending = log.store_decision(
            spec.ticker, trade_date, final_trade_decision, spec.rating
        )

    bt = backtest_bridge.run_backtest(spec, start_date, end_date, cfg, **backtest_kwargs)
    result.backtest_ok = bt.ok
    result.run_dir = bt.run_dir

    payload = build_reflection(spec, bt, start_date=start_date, end_date=end_date)
    result.reflection = payload.reflection

    if not bt.ok:
        # Still surface the reason; the reflection already explains the failure.
        result.error = bt.error or "backtest did not produce metrics"

    if log is not None:
        result.resolved = log.update_with_outcome(
            spec.ticker,
            trade_date,
            raw_return=payload.raw_return,
            alpha_return=payload.alpha_return,
            holding_days=payload.holding_days,
            reflection=payload.reflection,
        )

    result.ok = bt.ok and (result.resolved or not write_back)
    return result


# Runner executed inside TradingAgents' own interpreter. It prints one JSON line
# with the decision so the parent (this module) can read it without importing TA.
_TA_PROPAGATE_RUNNER = '''
import json, sys
from tradingagents.graph.trading_graph import TradingAgentsGraph
from tradingagents.default_config import DEFAULT_CONFIG

ticker, trade_date = sys.argv[1], sys.argv[2]
config = DEFAULT_CONFIG.copy()
overrides = json.loads(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3] else {}
config.update(overrides)

ta = TradingAgentsGraph(config=config)
final_state, decision = ta.propagate(ticker, trade_date)
out = {
    "final_trade_decision": final_state.get("final_trade_decision", ""),
    "trader_investment_plan": final_state.get("trader_investment_plan", ""),
    "signal": decision,
}
print("TAVIBE_DECISION_JSON:" + json.dumps(out, ensure_ascii=False))
'''


def propagate_and_verify(
    ticker: str,
    trade_date: str,
    start_date: str,
    end_date: str,
    *,
    cfg: BridgeConfig | None = None,
    ta_config_overrides: dict[str, Any] | None = None,
    propagate_timeout: int = 1800,
    **verify_kwargs: Any,
) -> LoopResult:
    """Produce a decision with TradingAgents, then verify it with a backtest.

    Shells out to TradingAgents' Python (``cfg.ta_python``) to run
    ``TradingAgentsGraph.propagate(ticker, trade_date)``, captures the decision
    markdown, then feeds it into :func:`verify_decision`. Both projects stay in
    their own interpreters.

    Args:
        ticker / trade_date: passed to ``propagate``.
        start_date / end_date: the backtest validation window.
        cfg: bridge configuration.
        ta_config_overrides: merged into ``DEFAULT_CONFIG`` inside TradingAgents
            (e.g. ``{"llm_provider": "openai", "deep_think_llm": "gpt-5.5"}``).
        propagate_timeout: seconds to allow the multi-agent run.
        **verify_kwargs: forwarded to :func:`verify_decision`.

    Returns:
        A :class:`LoopResult`. On a propagate failure, ``ok`` is False and
        ``error`` explains what went wrong; no memory entry is written.
    """
    cfg = cfg or BridgeConfig.from_env()

    ta_pkg = cfg.ta_root / "tradingagents"
    if not ta_pkg.exists():
        return LoopResult(
            ok=False, ticker=ticker,
            error=f"TradingAgents package not found at {ta_pkg}. Set TA_ROOT.",
        )

    cmd = [
        str(cfg.ta_python), "-c", _TA_PROPAGATE_RUNNER,
        ticker, trade_date,
        json.dumps(ta_config_overrides or {}),
    ]
    try:
        proc = subprocess.run(
            cmd, cwd=str(cfg.ta_root), capture_output=True, text=True,
            timeout=propagate_timeout, env=cfg.subprocess_env(),
        )
    except subprocess.TimeoutExpired:
        return LoopResult(ok=False, ticker=ticker,
                          error=f"TradingAgents propagate timed out after {propagate_timeout}s")
    except (OSError, ValueError) as exc:
        return LoopResult(ok=False, ticker=ticker,
                          error=f"failed to spawn TradingAgents: {exc}")

    decision = _extract_decision_json(proc.stdout)
    if decision is None:
        tail = (proc.stderr or proc.stdout or "")[-1500:]
        return LoopResult(
            ok=False, ticker=ticker,
            error=f"could not parse decision from TradingAgents output. Tail:\n{tail}",
        )

    return verify_decision(
        ticker,
        trade_date,
        decision.get("final_trade_decision", ""),
        start_date,
        end_date,
        trader_plan=decision.get("trader_investment_plan", ""),
        cfg=cfg,
        **verify_kwargs,
    )


def _extract_decision_json(stdout: str) -> dict[str, Any] | None:
    """Pull the single ``TAVIBE_DECISION_JSON:`` line out of TA's stdout."""
    marker = "TAVIBE_DECISION_JSON:"
    for line in stdout.splitlines():
        if line.startswith(marker):
            try:
                return json.loads(line[len(marker):])
            except json.JSONDecodeError:
                return None
    return None
