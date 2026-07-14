"""Configuration for the isolated Vibe-Trading MCP runtime."""

from __future__ import annotations

import os
import shutil
from dataclasses import dataclass, field
from pathlib import Path

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
    """Path and interpreter for the Vibe-Trading MCP server.

    Attributes:
        vibe_root: Path to the Vibe-Trading checkout.
        vibe_python: Interpreter that launches the Vibe MCP server. Defaults
            to a ``.venv`` inside the Vibe-Trading repo, then ``VIBE_PYTHON``,
            then ``python3``.
    """

    vibe_root: Path = field(
        default_factory=lambda: _env_path(
            "VIBE_TRADING_ROOT",
            _LEGACY_VIBE_ROOT,
        )
    )
    vibe_python: str = field(default_factory=lambda: os.environ.get("VIBE_PYTHON", ""))

    def __post_init__(self) -> None:
        self.vibe_root = Path(self.vibe_root).expanduser()
        self.vibe_agent_dir = self.vibe_root / "agent"
        if not self.vibe_python:
            self.vibe_python = self._resolve_interpreter(self.vibe_root)

    @classmethod
    def from_env(cls) -> "BridgeConfig":
        """Construct from environment variables.

        The dataclass fields already read their env vars in ``default_factory``,
        so a plain ``cls()`` is env-driven. This named constructor exists so call
        sites read intentionally and so future env-only wiring has one home.
        """
        return cls()

    def subprocess_env(self) -> dict[str, str]:
        """Environment for the Vibe-Trading MCP subprocess.

        Starts from the current environment and prepends the Vibe ``agent/``
        directory to ``PYTHONPATH`` so its MCP server imports reliably.
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
        """Return a list of human-readable Vibe MCP configuration problems."""
        problems = []
        if not self.vibe_root.exists():
            problems.append(f"Vibe-Trading repo not found at {self.vibe_root}")
        if not self.vibe_agent_dir.exists():
            problems.append(
                f"Vibe agent directory not found at {self.vibe_agent_dir} "
                "(is VIBE_TRADING_ROOT pointing at the repo root?)"
            )
        return problems

    def describe(self) -> str:
        """One-line-per-field summary for diagnostics."""
        return "\n".join(
            [
                f"vibe_root       = {self.vibe_root}",
                f"vibe_agent_dir  = {self.vibe_agent_dir}",
                f"vibe_python     = {self.vibe_python}",
            ]
        )
