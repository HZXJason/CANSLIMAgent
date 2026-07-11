"""Bridge configuration for CANSLIMAgent's vendored AI runtime.

TradingAgents and this bridge are repository-owned sources under
``sidecar/vendor``. Vibe-Trading is intentionally an isolated engine and is
configured with ``VIBE_TRADING_ROOT``; the prior sibling checkout remains a
backward-compatible local-development fallback.
"""

from __future__ import annotations

import os
import shutil
from dataclasses import dataclass, field
from pathlib import Path

# ``.../sidecar/vendor/ta_vibe_bridge/config.py``.
_VENDOR_ROOT = Path(__file__).resolve().parents[1]
_REPO_ROOT = Path(__file__).resolve().parents[3]
_LEGACY_VIBE_ROOT = _REPO_ROOT.parent / "TradingAgent2.0" / "Vibe-Trading"


def _env_path(name: str, default: Path) -> Path:
    raw = os.environ.get(name)
    return Path(raw).expanduser().resolve() if raw else default


def _first_existing(*candidates: Path) -> Path | None:
    for c in candidates:
        if c and c.exists():
            return c
    return None


@dataclass
class BridgeConfig:
    """Paths and interpreters for both projects.

    Attributes:
        vibe_root: Path to the Vibe-Trading checkout.
        ta_root: Path to the TradingAgents checkout.
        vibe_python: Interpreter that can ``import backtest`` (i.e. run the
            Vibe-Trading backtest runner). Defaults to a ``.venv`` inside the
            Vibe-Trading repo, then ``VIBE_PYTHON`` env, then ``python3``.
        ta_python: Interpreter that can ``import tradingagents``.
        runs_root: Where the bridge writes backtest run directories. Must be
            under one of Vibe-Trading's allowed run roots (defaults to
            ``Vibe-Trading/agent/runs``, which is always allowed).
        memory_log_path: TradingAgents decision log the closed loop writes back
            to. Mirrors TradingAgents' own default and its
            ``TRADINGAGENTS_MEMORY_LOG_PATH`` override.
    """

    vibe_root: Path = field(
        default_factory=lambda: _env_path(
            "VIBE_TRADING_ROOT",
            _VENDOR_ROOT / "Vibe-Trading" if (_VENDOR_ROOT / "Vibe-Trading").exists() else _LEGACY_VIBE_ROOT,
        )
    )
    ta_root: Path = field(
        default_factory=lambda: _env_path("TA_ROOT", _VENDOR_ROOT)
    )
    vibe_python: str = field(default_factory=lambda: os.environ.get("VIBE_PYTHON", ""))
    ta_python: str = field(default_factory=lambda: os.environ.get("TA_PYTHON", ""))
    runs_root: Path | None = None
    memory_log_path: Path = field(
        default_factory=lambda: _env_path(
            "TRADINGAGENTS_MEMORY_LOG_PATH",
            Path("~/.tradingagents/memory/trading_memory.md").expanduser(),
        )
    )

    def __post_init__(self) -> None:
        # Coerce str -> Path so callers can pass either (a very natural mistake
        # that would otherwise crash on the first ``/`` path join below).
        self.vibe_root = Path(self.vibe_root).expanduser()
        self.ta_root = Path(self.ta_root).expanduser()
        if self.runs_root is not None:
            self.runs_root = Path(self.runs_root).expanduser()
        self.memory_log_path = Path(self.memory_log_path).expanduser()
        # agent/ is the import root for Vibe-Trading's backtest package.
        self.vibe_agent_dir = self.vibe_root / "agent"
        if self.runs_root is None:
            # agent/runs is always on Vibe-Trading's safe_run_dir allowlist.
            self.runs_root = _env_path(
                "TA_VIBE_RUNS_ROOT", self.vibe_agent_dir / "runs"
            )
        if not self.vibe_python:
            self.vibe_python = self._resolve_interpreter(self.vibe_root)
        if not self.ta_python:
            self.ta_python = self._resolve_interpreter(self.ta_root)

    @classmethod
    def from_env(cls) -> "BridgeConfig":
        """Construct from environment variables.

        The dataclass fields already read their env vars in ``default_factory``,
        so a plain ``cls()`` is env-driven. This named constructor exists so call
        sites read intentionally and so future env-only wiring has one home.
        """
        return cls()

    def subprocess_env(self) -> dict[str, str]:
        """Environment for the Vibe-Trading backtest subprocess.

        Starts from the current environment (so market-data keys like
        ``TUSHARE_TOKEN`` / ``FMP_API_KEY`` flow through) and prepends the Vibe
        ``agent/`` dir to ``PYTHONPATH``. The runner is launched with
        ``cwd=agent/`` and does ``from src.tools... import`` and ``import
        backtest...``; cwd normally covers that, but pinning PYTHONPATH makes the
        spawn robust to interpreters that don't put cwd on ``sys.path``.
        """
        env = dict(os.environ)
        agent = str(self.vibe_agent_dir)
        existing = env.get("PYTHONPATH", "")
        env["PYTHONPATH"] = f"{agent}{os.pathsep}{existing}" if existing else agent
        return env

    @staticmethod
    def _resolve_interpreter(repo_root: Path) -> str:
        """Best-effort: a repo-local venv, else system python3/python."""
        venv_py = _first_existing(
            repo_root / ".venv" / "bin" / "python",
            repo_root / ".venv" / "Scripts" / "python.exe",
            repo_root / "venv" / "bin" / "python",
        )
        if venv_py:
            return str(venv_py)
        return shutil.which("python3") or shutil.which("python") or "python3"

    # --- validation helpers ---

    def check_vibe(self) -> list[str]:
        """Return a list of human-readable problems with the Vibe-Trading side."""
        problems = []
        if not self.vibe_root.exists():
            problems.append(f"Vibe-Trading repo not found at {self.vibe_root}")
        if not (self.vibe_agent_dir / "backtest" / "runner.py").exists():
            problems.append(
                f"backtest/runner.py not found under {self.vibe_agent_dir} "
                "(is VIBE_TRADING_ROOT pointing at the repo root?)"
            )
        return problems

    def check_ta(self) -> list[str]:
        """Return a list of human-readable problems with the TradingAgents side."""
        problems = []
        if not self.ta_root.exists():
            problems.append(f"TradingAgents repo not found at {self.ta_root}")
        if not (self.ta_root / "tradingagents").exists():
            problems.append(
                f"tradingagents/ package not found under {self.ta_root} "
                "(is TA_ROOT pointing at the repo root?)"
            )
        return problems

    def describe(self) -> str:
        """One-line-per-field summary for diagnostics."""
        return "\n".join(
            [
                f"vibe_root       = {self.vibe_root}",
                f"vibe_agent_dir  = {self.vibe_agent_dir}",
                f"vibe_python     = {self.vibe_python}",
                f"ta_root         = {self.ta_root}",
                f"ta_python       = {self.ta_python}",
                f"runs_root       = {self.runs_root}",
                f"memory_log_path = {self.memory_log_path}",
            ]
        )
