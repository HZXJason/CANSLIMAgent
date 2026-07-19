"""Expose Vibe-Trading's MCP data tools to TradingAgents' analyst agents (Layer 1).

Vibe-Trading ships an MCP server (``vibe-trading-mcp``) with 54 tools. This
module loads them as LangChain tools inside TradingAgents' process using
``langchain-mcp-adapters``, so an analyst can call Vibe's richer, multi-source
data layer instead of TradingAgents' 2-vendor default.

Why this respects the "never share a process" rule: the MCP *server* runs as a
subprocess in Vibe-Trading's own interpreter (``cfg.vibe_python -m mcp_server``);
only the thin MCP *client* lives in TradingAgents' process. The two dependency
stacks never import each other — they speak MCP over stdio. This is exactly the
boundary MCP exists for.

The heavy imports (``langchain_mcp_adapters``, ``mcp``) are done lazily so this
module imports fine in any environment; only :func:`load_vibe_tools` needs them.

The per-analyst assignment (TradingAgents keys; all zero-key for US per the
interface survey) is defined once in :data:`ta_patch.DEFAULT_ANALYST_TOOLS` and
imported here as ``TOOLS_BY_ANALYST`` so the two layers can never drift.
"""

from __future__ import annotations

import asyncio
from concurrent.futures import Future
from contextvars import copy_context
from functools import wraps
from pathlib import Path
from threading import Thread
from typing import TYPE_CHECKING, Any, Awaitable, Callable

from .config import BridgeConfig

if TYPE_CHECKING:  # only for type checkers; never imported at runtime here
    from langchain_core.tools import BaseTool

# US-relevant Vibe MCP tools, grouped by the TradingAgents analyst that benefits.
# Single source of truth lives in ``ta_patch`` (it uses TradingAgents' real
# analyst keys: market / social / news / fundamentals). Importing it here avoids
# two maps drifting apart. ``ta_patch`` imports only stdlib ``typing``, so there
# is no import cycle.
from .ta_patch import DEFAULT_ANALYST_TOOLS as TOOLS_BY_ANALYST

# The union of every tool we ever hand to an analyst — the allowlist we ask the
# MCP client to load. Keeping it tight speeds startup and avoids exposing the
# trading/connector tools to research analysts.
DEFAULT_US_TOOLS: tuple[str, ...] = tuple(
    sorted({name for names in TOOLS_BY_ANALYST.values() for name in names})
)


def _run_coroutine_sync(factory: Callable[[], Awaitable[Any]]) -> Any:
    """Run a freshly-created coroutine from synchronous TradingAgents code.

    TradingAgents executes its compiled graph synchronously. MCP tools, however,
    are exposed by ``langchain-mcp-adapters`` as coroutine-only
    ``StructuredTool`` objects. The normal graph path runs in an executor thread
    without an event loop, while tests and embedders can call it from a thread
    that already owns one. The latter needs a helper thread because blocking on
    the current loop would deadlock.

    ``factory`` deliberately creates the coroutine inside the loop that awaits
    it, avoiding cross-loop awaitables and un-awaited coroutine warnings.
    """
    try:
        asyncio.get_running_loop()
    except RuntimeError:
        return asyncio.run(factory())

    outcome: Future[Any] = Future()

    def run_in_helper_thread() -> None:
        try:
            outcome.set_result(asyncio.run(factory()))
        except BaseException as exc:  # preserve the tool's original failure
            outcome.set_exception(exc)

    context = copy_context()
    thread = Thread(
        target=context.run,
        args=(run_in_helper_thread,),
        name="vibe-mcp-sync-bridge",
        daemon=True,
    )
    thread.start()
    thread.join()
    return outcome.result()


def make_sync_capable(tool: "BaseTool") -> "BaseTool":
    """Add a sync entrypoint to an async-only MCP ``StructuredTool``.

    The copied tool retains the original schema, async coroutine, callbacks,
    metadata, error handlers, and ``content_and_artifact`` response contract.
    ``wraps`` is load-bearing: LangGraph inspects the selected sync function for
    injected arguments, so it must still see the MCP coroutine's annotated
    ``runtime`` parameter.

    Existing synchronous tools and non-``StructuredTool`` implementations are
    returned unchanged.
    """
    from langchain_core.tools import StructuredTool

    if not isinstance(tool, StructuredTool):
        return tool
    if callable(getattr(tool, "func", None)):
        return tool

    coroutine = getattr(tool, "coroutine", None)
    if not callable(coroutine):
        return tool

    @wraps(coroutine)
    def invoke_sync(*args: Any, **kwargs: Any) -> Any:
        return _run_coroutine_sync(lambda: coroutine(*args, **kwargs))

    return tool.model_copy(update={"func": invoke_sync})


def vibe_mcp_server_spec(cfg: BridgeConfig | None = None) -> dict[str, Any]:
    """Build the ``langchain-mcp-adapters`` server spec for Vibe-Trading.

    Returns a dict shaped for ``MultiServerMCPClient({"vibe-trading": <spec>})``.
    Launches ``<vibe_python> -m mcp_server --transport stdio`` with cwd at the
    Vibe ``agent/`` dir, so the server imports Vibe's package cleanly.
    """
    cfg = cfg or BridgeConfig.from_env()
    return {
        "command": cfg.vibe_python,
        "args": ["-m", "mcp_server", "--transport", "stdio"],
        "transport": "stdio",
        "cwd": str(cfg.vibe_agent_dir),
        "env": cfg.subprocess_env(),
    }


async def load_vibe_tools(
    cfg: BridgeConfig | None = None,
    *,
    only: tuple[str, ...] | None = DEFAULT_US_TOOLS,
) -> list["BaseTool"]:
    """Load Vibe-Trading's MCP tools as LangChain tools (async).

    Args:
        cfg: bridge configuration (defaults to env-derived).
        only: allowlist of MCP tool names to keep; ``None`` keeps all. Defaults
            to the US research tools in :data:`DEFAULT_US_TOOLS`.

    Returns:
        A list of LangChain ``BaseTool`` objects ready to ``bind_tools`` / drop
        into a ``ToolNode``.

    Raises:
        RuntimeError: if ``langchain-mcp-adapters`` is not installed, with the
            exact pip command to fix it.
    """
    try:
        from langchain_mcp_adapters.client import MultiServerMCPClient
    except ImportError as exc:  # pragma: no cover - environment-dependent
        raise RuntimeError(
            "langchain-mcp-adapters is required to load Vibe-Trading MCP tools. "
            "Install it in the TradingAgents environment:\n"
            "    pip install langchain-mcp-adapters\n"
            f"(original import error: {exc})"
        ) from exc

    cfg = cfg or BridgeConfig.from_env()
    client = MultiServerMCPClient({"vibe-trading": vibe_mcp_server_spec(cfg)})
    tools = await client.get_tools()
    if only is not None:
        allow = set(only)
        tools = [t for t in tools if t.name in allow]
    # TradingAgents uses CompiledStateGraph.invoke(), so every MCP tool placed
    # in its ToolNodes must support the synchronous BaseTool contract as well as
    # its native async contract. Apply this centrally for both A-share and US
    # assignments instead of special-casing either market.
    return [make_sync_capable(tool) for tool in tools]


def load_vibe_tools_sync(
    cfg: BridgeConfig | None = None,
    *,
    only: tuple[str, ...] | None = DEFAULT_US_TOOLS,
) -> list["BaseTool"]:
    """Synchronous wrapper over :func:`load_vibe_tools`.

    Convenience for TradingAgents' synchronous graph construction. Uses
    ``asyncio.run`` and therefore must not be called from within a running event
    loop; call :func:`load_vibe_tools` directly in async contexts.
    """
    return asyncio.run(load_vibe_tools(cfg, only=only))


def split_tools_by_analyst(
    tools: list["BaseTool"],
) -> dict[str, list["BaseTool"]]:
    """Partition a flat tool list into per-analyst lists per :data:`TOOLS_BY_ANALYST`.

    A tool that maps to several analysts (e.g. ``get_stock_profile``) appears in
    each. Tools with no mapping are ignored. Pure function — unit-testable with
    lightweight stand-ins that only need a ``.name`` attribute.
    """
    by_name = {t.name: t for t in tools}
    out: dict[str, list["BaseTool"]] = {}
    for analyst, names in TOOLS_BY_ANALYST.items():
        picked = [by_name[n] for n in names if n in by_name]
        if picked:
            out[analyst] = picked
    return out


def preflight(cfg: BridgeConfig | None = None) -> list[str]:
    """Return human-readable problems that would stop MCP tool loading.

    Checks the Vibe repo layout and adapter availability without spawning the
    server. Empty list == good to go.
    """
    cfg = cfg or BridgeConfig.from_env()
    problems = cfg.check_vibe()
    if not Path(cfg.vibe_agent_dir / "mcp_server.py").exists():
        problems.append(
            f"mcp_server.py not found under {cfg.vibe_agent_dir}; "
            "cannot start the Vibe MCP server."
        )
    try:
        import langchain_mcp_adapters  # noqa: F401
    except ImportError:
        problems.append(
            "langchain-mcp-adapters not installed in this (TradingAgents) env; "
            "run: pip install langchain-mcp-adapters"
        )
    return problems
