"""ta_vibe_bridge — connect TradingAgents (the brain) to Vibe-Trading (the hands).

This package keeps both projects independent — they have incompatible dependency
stacks (TradingAgents needs Python 3.12 + recent LangChain; Vibe-Trading needs
3.11+) and must never share a process. The bridge talks to each side through
subprocess boundaries and file artifacts instead of cross-imports.

Three layers, mapped to the integration design:

- Layer 1 (data):    ``mcp_data_tools`` + ``ta_patch`` — expose Vibe-Trading's
                     data tools to TradingAgents' analyst agents.
- Layer 3 (verify):  ``signal_spec`` -> ``codegen`` -> ``backtest_bridge`` ->
                     ``reflection`` -> ``closed_loop`` — turn a TradingAgents
                     decision into a Vibe-Trading backtest and feed the result
                     back into the TradingAgents decision log.

Layer 2 (enhancing individual agents) is a configuration concern that reuses
Layer 1's tools; see ``README.md``.
"""

from __future__ import annotations

# Every module below is stdlib-only *at import time* — the calls that need a
# project's runtime (subprocess to Vibe's / TA's Python, or the optional
# langchain-mcp-adapters import) are deferred to call time. So the whole public
# API imports cleanly in a bare environment; only invoking a cross-stack call
# needs that stack present.
from .config import BridgeConfig
from .signal_spec import (
    EntryCondition,
    SignalSpec,
    spec_from_decision,
)
from .codegen import assert_vibe_ast_safe, generate_signal_engine_source
from .backtest_bridge import BacktestResult, run_backtest
from .reflection import ReflectionPayload, build_reflection
from .memory_writeback import MemoryWriteback
from .closed_loop import LoopResult, propagate_and_verify, verify_decision

__all__ = [
    # config
    "BridgeConfig",
    # Layer 3: signal spec + codegen
    "SignalSpec",
    "EntryCondition",
    "spec_from_decision",
    "generate_signal_engine_source",
    "assert_vibe_ast_safe",
    # Layer 3: backtest + reflection + loop
    "BacktestResult",
    "run_backtest",
    "ReflectionPayload",
    "build_reflection",
    "MemoryWriteback",
    "LoopResult",
    "verify_decision",
    "propagate_and_verify",
    "__version__",
]

__version__ = "0.1.0"
