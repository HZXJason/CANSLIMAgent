"""Run a :class:`SignalSpec` through Vibe-Trading's backtest engine.

The bridge never imports Vibe-Trading. It writes a run directory Vibe understands
(``config.json`` + ``code/signal_engine.py``), shells out to Vibe's own Python
(``python -m backtest.runner <run_dir>`` with ``cwd=<vibe>/agent``), then reads
the artifacts back. Process isolation is deliberate: the two projects have
incompatible dependency stacks and must not share an interpreter.

Result shape (:class:`BacktestResult`): the parsed scalar metrics from
``artifacts/metrics.csv`` plus the nested ``artifacts/validation.json`` (Monte
Carlo / bootstrap / walk-forward), or a structured error when the run could not
complete.
"""

from __future__ import annotations

import csv
import json
import subprocess
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .codegen import generate_signal_engine_source
from .config import BridgeConfig
from .signal_spec import SignalSpec

# Metrics we surface as first-class fields; everything else stays in ``metrics``.
_HEADLINE_METRICS = (
    "total_return",
    "annual_return",
    "max_drawdown",
    "sharpe",
    "calmar",
    "sortino",
    "win_rate",
    "trade_count",
    "benchmark_return",
    "excess_return",
    "information_ratio",
)


@dataclass
class BacktestResult:
    """Outcome of one backtest run over a :class:`SignalSpec`.

    Attributes:
        ok: whether the engine produced artifacts.
        run_dir: the directory holding config/code/artifacts (for auditing);
            empty string when the run never got far enough to create one.
        metrics: every scalar from ``metrics.csv`` (keys are Vibe's own names).
        validation: parsed ``validation.json`` (may be empty if not requested
            or if too few trades were produced to run the statistics).
        exit_code: the runner subprocess exit code (None if never spawned).
        error / stderr_tail: populated when ``ok`` is False.
    """

    ok: bool
    run_dir: str = ""
    metrics: dict[str, float] = field(default_factory=dict)
    validation: dict[str, Any] = field(default_factory=dict)
    exit_code: int | None = None
    error: str = ""
    stderr_tail: str = ""

    # --- convenience accessors on the headline numbers -----------------------

    def _num(self, key: str) -> float | None:
        v = self.metrics.get(key)
        return v if isinstance(v, (int, float)) else None

    @property
    def sharpe(self) -> float | None:
        return self._num("sharpe")

    @property
    def total_return(self) -> float | None:
        return self._num("total_return")

    @property
    def max_drawdown(self) -> float | None:
        return self._num("max_drawdown")

    @property
    def win_rate(self) -> float | None:
        return self._num("win_rate")

    @property
    def trade_count(self) -> int | None:
        v = self._num("trade_count")
        return int(v) if v is not None else None

    @property
    def mc_p_value_sharpe(self) -> float | None:
        mc = self.validation.get("monte_carlo") or {}
        v = mc.get("p_value_sharpe")
        return v if isinstance(v, (int, float)) else None

    @property
    def walk_forward_consistency(self) -> float | None:
        wf = self.validation.get("walk_forward") or {}
        v = wf.get("consistency_rate")
        return v if isinstance(v, (int, float)) else None


def build_config(
    spec: SignalSpec,
    start_date: str,
    end_date: str,
    *,
    source: str = "auto",
    initial_cash: int = 1_000_000,
    benchmark: str = "SPY",
    walk_forward_windows: int = 5,
) -> dict[str, Any]:
    """Build the Vibe ``config.json`` dict for a spec over a date window.

    Notes:
        * ``source="auto"`` routes ``AAPL.US`` to the us_equity fallback chain
          (yahoo → stooq → sina → ...), all zero-key for US. Do **not** pass a
          bare source like ``yahoo`` — Vibe's engine router would misroute a US
          symbol to the crypto engine (see runner._create_market_engine).
        * The field is ``initial_cash`` (not ``initial_capital``).
        * ``validation`` presence is what makes the runner write
          ``artifacts/validation.json``.
    """
    return {
        "codes": [spec.ticker],
        "start_date": start_date,
        "end_date": end_date,
        "source": source,
        "interval": "1D",
        "engine": "daily",
        "initial_cash": initial_cash,
        "benchmark": benchmark,
        "validation": {
            "monte_carlo": {},
            "bootstrap": {},
            "walk_forward": {"n_windows": walk_forward_windows},
        },
        # Provenance so a run dir is self-describing when audited later.
        "_ta_vibe_bridge": {
            "source_ticker": spec.source_ticker,
            "rating": spec.rating,
            "target_weight": spec.target_weight,
            "holding_days": spec.holding_days,
            "stop_pct": spec.stop_pct,
            "entry": [c.describe() for c in spec.entry_conditions] or ["directional"],
        },
    }


def write_run_dir(
    spec: SignalSpec,
    start_date: str,
    end_date: str,
    cfg: BridgeConfig,
    *,
    run_id: str | None = None,
    **config_kwargs: Any,
) -> Path:
    """Materialize a Vibe-compatible run directory and return its path.

    Layout:
        <runs_root>/<run_id>/config.json
        <runs_root>/<run_id>/code/signal_engine.py

    The signal engine is generated (and AST-self-checked) here so a bad spec
    fails before we spawn a subprocess.
    """
    run_id = run_id or f"tavibe_{spec.source_ticker or spec.ticker}_{start_date}_{int(time.time())}"
    run_id = run_id.replace("/", "_").replace(" ", "_")
    run_dir = cfg.runs_root / run_id
    (run_dir / "code").mkdir(parents=True, exist_ok=True)

    config = build_config(spec, start_date, end_date, **config_kwargs)
    (run_dir / "config.json").write_text(
        json.dumps(config, indent=2, ensure_ascii=False), encoding="utf-8"
    )

    source = generate_signal_engine_source(spec)  # raises on an unsafe spec
    (run_dir / "code" / "signal_engine.py").write_text(source, encoding="utf-8")
    return run_dir


def _read_metrics_csv(path: Path) -> dict[str, float]:
    """Read Vibe's metrics.csv into a flat {name: float} dict.

    Vibe writes it as two columns (metric,value) OR as a single header row +
    value row; handle both. Non-numeric cells are skipped.
    """
    if not path.exists():
        return {}
    rows = list(csv.reader(path.read_text(encoding="utf-8").splitlines()))
    rows = [r for r in rows if r]
    if not rows:
        return {}
    out: dict[str, float] = {}

    def _num(x: str) -> float | None:
        try:
            return float(x)
        except (TypeError, ValueError):
            return None

    header = [c.strip().lower() for c in rows[0]]
    if len(rows[0]) == 2 and header not in (["metric", "value"],):
        # Could be a genuine 2-col metric,value table with no header, or with
        # one. Try key/value parse across all rows.
        for r in rows:
            if len(r) >= 2:
                v = _num(r[1])
                if v is not None:
                    out[r[0].strip()] = v
        if out:
            return out
    if header == ["metric", "value"]:
        for r in rows[1:]:
            if len(r) >= 2:
                v = _num(r[1])
                if v is not None:
                    out[r[0].strip()] = v
        return out
    # Header row + single value row (wide format).
    if len(rows) >= 2:
        for name, val in zip(rows[0], rows[1]):
            v = _num(val)
            if v is not None:
                out[name.strip()] = v
    return out


def _collect_result(
    run_dir: Path, *, ok: bool, exit_code: int | None, error: str = "", stderr: str = ""
) -> BacktestResult:
    art = run_dir / "artifacts"
    metrics = _read_metrics_csv(art / "metrics.csv")
    validation: dict[str, Any] = {}
    vjson = art / "validation.json"
    if vjson.exists():
        try:
            validation = json.loads(vjson.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            validation = {}
    produced = ok and bool(metrics)
    # If the engine ran but wrote nothing parseable, surface why.
    if not produced and not error:
        error = "backtest produced no parseable metrics.csv"
    return BacktestResult(
        ok=produced,
        run_dir=str(run_dir),
        metrics=metrics,
        validation=validation,
        exit_code=exit_code,
        error=error,
        stderr_tail=stderr[-4000:] if stderr else "",
    )


def run_backtest(
    spec: SignalSpec,
    start_date: str,
    end_date: str,
    cfg: BridgeConfig | None = None,
    *,
    timeout: int = 300,
    **config_kwargs: Any,
) -> BacktestResult:
    """Run one backtest for a spec by shelling out to Vibe-Trading's runner.

    Isolation: this spawns Vibe's own Python (``cfg.vibe_python``) with cwd at
    the Vibe ``agent/`` dir and runs ``python -m backtest.runner <run_dir>`` —
    exactly the invocation Vibe's own backtest tool uses — so the two
    dependency stacks never share a process.
    """
    cfg = cfg or BridgeConfig.from_env()
    if not spec.is_actionable:
        return BacktestResult(
            ok=False,
            run_dir="",
            error="spec is not actionable (Hold / zero weight) — nothing to backtest",
        )

    run_dir = write_run_dir(spec, start_date, end_date, cfg, **config_kwargs)

    agent_dir = cfg.vibe_agent_dir
    if not agent_dir.exists():
        return _collect_result(
            run_dir, ok=False, exit_code=None,
            error=f"Vibe agent dir not found: {agent_dir}. Set VIBE_TRADING_ROOT.",
        )

    cmd = [str(cfg.vibe_python), "-m", "backtest.runner", str(run_dir)]
    try:
        proc = subprocess.run(
            cmd, cwd=str(agent_dir), capture_output=True, text=True,
            timeout=timeout, env=cfg.subprocess_env(),
        )
    except subprocess.TimeoutExpired:
        return _collect_result(run_dir, ok=False, exit_code=None,
                               error=f"backtest timed out after {timeout}s")
    except (OSError, ValueError) as exc:
        return _collect_result(run_dir, ok=False, exit_code=None,
                               error=f"failed to spawn Vibe runner: {exc}")

    return _collect_result(
        run_dir, ok=(proc.returncode == 0), exit_code=proc.returncode,
        stderr=proc.stderr or "",
    )
