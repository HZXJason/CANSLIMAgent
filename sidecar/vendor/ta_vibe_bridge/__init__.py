"""ta_vibe_bridge — expose Vibe-Trading evidence to TradingAgents.

This package keeps both projects independent — they have incompatible dependency
stacks (TradingAgents needs Python 3.12 + recent LangChain; Vibe-Trading needs
3.11+) and must never share a process. The bridge talks to each side through
subprocess boundaries and file artifacts instead of cross-imports.

``mcp_data_tools`` + ``ta_patch`` expose Vibe-Trading's data tools to
TradingAgents' analyst agents. Historical validation of a current decision via
a static generated rule is intentionally not part of this bridge.
"""

from __future__ import annotations

# Every module below is stdlib-only *at import time* — the calls that need a
# project's runtime (subprocess to Vibe's / TA's Python, or the optional
# langchain-mcp-adapters import) are deferred to call time. So the whole public
# API imports cleanly in a bare environment; only invoking a cross-stack call
# needs that stack present.
from .config import BridgeConfig
__all__ = [
    # config
    "BridgeConfig",
    "__version__",
]

__version__ = "0.1.0"
