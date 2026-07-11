"""Inject Vibe-Trading MCP tools into TradingAgents' analyst agents at runtime.

Layer 1 + Layer 2 of the integration: give each TradingAgents analyst extra data
tools (backed by Vibe-Trading's MCP server) without editing TradingAgents source.

Why runtime patching (not a source edit): TradingAgents wires tools in two places
that must stay in sync, established by reading its code:

1. **Bind side** — each ``create_<x>_analyst(llm)`` factory hardcodes a ``tools``
   list and calls ``llm.bind_tools(tools)``. This is what puts a tool in the
   model's schema so it can *call* it. The factories are imported into
   ``tradingagents.graph.setup`` as module globals
   (``from tradingagents.agents import create_fundamentals_analyst``), so patching
   the name in *that* namespace is what the graph build actually sees.
2. **Execution side** — ``TradingAgentsGraph._create_tool_nodes()`` builds a
   ``{analyst_key: ToolNode}`` dict with its own hardcoded tool lists. This is
   what *executes* a tool call. A tool bound but absent here raises at runtime.

``apply_mcp_tools`` patches both. The pure helpers (:func:`bind_extra_tools`,
:func:`tools_for_analyst`) are unit-tested; the monkeypatch wrapper needs the
TradingAgents + langchain runtime to exercise, so it is kept thin and explicit.

Analyst keys used by TradingAgents' graph: ``market``, ``social`` (sentiment),
``news``, ``fundamentals``. Note ``social``/sentiment does not bind tools (it
pre-fetches and uses structured output), so injected tools only reach it via the
ToolNode if a future version enables tool calls — we still map it for symmetry
but it is a no-op on the bind side today.
"""

from __future__ import annotations

from typing import Any, Callable, Iterable

# Default per-analyst assignment of Vibe MCP tools, matching the integration
# design: fundamentals gets SEC/financials, news gets news+filings, sentiment
# gets options flow (put/call, IV skew), market gets raw OHLCV + screening.
DEFAULT_ANALYST_TOOLS: dict[str, tuple[str, ...]] = {
    "fundamentals": (
        "get_sec_filings",
        "get_financial_statements",
        "get_stock_profile",
    ),
    "news": (
        "get_stock_news",
        "get_sec_filings",
    ),
    "social": (
        "get_options_chain",
        "get_stock_profile",
    ),
    "market": (
        "get_market_data",
        "screen_market",
        "search_symbol",
    ),
}


class _BoundToolsProxy:
    """Wrap an LLM so ``bind_tools`` appends ``extra_tools`` to the caller's list.

    The analyst factories call ``llm.bind_tools(their_tools)``; wrapping the llm
    they receive lets us add tools to that specific analyst without editing the
    factory. Every other attribute/method delegates to the real llm, so the
    proxy is transparent to LangChain.
    """

    def __init__(self, llm: Any, extra_tools: list[Any]):
        self._llm = llm
        self._extra_tools = list(extra_tools)

    def bind_tools(self, tools, *args, **kwargs):
        merged = list(tools) + self._extra_tools
        return self._llm.bind_tools(merged, *args, **kwargs)

    def __getattr__(self, name):
        # Only called for attributes not found on the proxy itself.
        return getattr(self._llm, name)


def bind_extra_tools(llm: Any, extra_tools: Iterable[Any]) -> Any:
    """Return ``llm`` unchanged when there are no extra tools, else a proxy.

    Kept pure and separate so it can be unit-tested with a fake llm.
    """
    extra = [t for t in extra_tools if t is not None]
    if not extra:
        return llm
    return _BoundToolsProxy(llm, extra)


def tools_for_analyst(
    analyst_key: str,
    mcp_tools_by_name: dict[str, Any],
    assignment: dict[str, tuple[str, ...]] | None = None,
) -> list[Any]:
    """Resolve the MCP tool objects assigned to one analyst.

    Args:
        analyst_key: one of ``market`` / ``social`` / ``news`` / ``fundamentals``.
        mcp_tools_by_name: ``{tool_name: langchain_tool}`` loaded from Vibe's MCP
            server (see ``mcp_data_tools.load_vibe_tools``).
        assignment: override of :data:`DEFAULT_ANALYST_TOOLS`.

    Returns:
        The tool objects that exist in ``mcp_tools_by_name``; names not present
        (e.g. a key-gated tool the server didn't expose) are skipped silently.
    """
    wanted = (assignment or DEFAULT_ANALYST_TOOLS).get(analyst_key, ())
    return [mcp_tools_by_name[name] for name in wanted if name in mcp_tools_by_name]


# --- runtime monkeypatch (needs the TradingAgents + langchain runtime) --------

# Maps the analyst *key* used everywhere here to the factory name in
# ``tradingagents.graph.setup``'s namespace and the ToolNode key.
_FACTORY_NAMES = {
    "market": "create_market_analyst",
    "social": "create_sentiment_analyst",
    "news": "create_news_analyst",
    "fundamentals": "create_fundamentals_analyst",
}


def apply_mcp_tools(
    mcp_tools_by_name: dict[str, Any],
    *,
    assignment: dict[str, tuple[str, ...]] | None = None,
) -> Callable[[], None]:
    """Monkeypatch TradingAgents so each analyst gains its assigned MCP tools.

    Call this **before** constructing ``TradingAgentsGraph`` (and before the
    graph is compiled). Patches two places, matching how TradingAgents wires
    tools (verified against its source):

    * ``tradingagents.graph.setup`` factory globals — wraps the llm each analyst
      factory receives so its ``bind_tools`` call also binds the MCP tools.
    * ``TradingAgentsGraph._create_tool_nodes`` — appends the same MCP tools to
      each analyst's ``ToolNode`` so bound calls can execute.

    Args:
        mcp_tools_by_name: ``{tool_name: langchain_tool}`` from Vibe's MCP server.
        assignment: override of :data:`DEFAULT_ANALYST_TOOLS`.

    Returns:
        A zero-arg ``undo()`` that restores the original functions. Import errors
        (TradingAgents/langgraph absent) propagate — this half needs both stacks.
    """
    from tradingagents.graph import setup as ta_setup
    from tradingagents.graph import trading_graph as ta_tg
    from langgraph.prebuilt import ToolNode

    originals: dict[str, Any] = {}

    # 1) Bind side: wrap each factory global in setup's namespace.
    for key, factory_name in _FACTORY_NAMES.items():
        original = getattr(ta_setup, factory_name, None)
        if original is None:
            continue
        originals[factory_name] = original
        extra = tools_for_analyst(key, mcp_tools_by_name, assignment)

        def make_wrapper(orig, extra_tools):
            def wrapped(llm, *args, **kwargs):
                return orig(bind_extra_tools(llm, extra_tools), *args, **kwargs)
            return wrapped

        setattr(ta_setup, factory_name, make_wrapper(original, extra))

    # 2) Execution side: append the tools to each analyst's ToolNode.
    original_ctn = ta_tg.TradingAgentsGraph._create_tool_nodes
    originals["_create_tool_nodes"] = original_ctn

    def patched_create_tool_nodes(self):
        nodes = original_ctn(self)
        for key, node in list(nodes.items()):
            extra = tools_for_analyst(key, mcp_tools_by_name, assignment)
            if not extra:
                continue
            existing = list(getattr(node, "tools_by_name", {}).values())
            nodes[key] = ToolNode(existing + extra)
        return nodes

    ta_tg.TradingAgentsGraph._create_tool_nodes = patched_create_tool_nodes

    def undo() -> None:
        for factory_name in _FACTORY_NAMES.values():
            if factory_name in originals:
                setattr(ta_setup, factory_name, originals[factory_name])
        ta_tg.TradingAgentsGraph._create_tool_nodes = originals["_create_tool_nodes"]

    return undo
