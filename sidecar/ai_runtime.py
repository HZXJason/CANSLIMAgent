"""Vendored TradingAgents runtime plus an isolated Vibe-Trading adapter.

The TradingAgents graph and the CANSLIM/Vibe bridge live under ``sidecar/vendor``
in this repository.  Vibe-Trading remains an independent runtime because its
MCP server and backtest engine intentionally use a separate Python environment.
"""

from __future__ import annotations

import asyncio
import inspect
import json
import math
import os
import sys
import threading
import time
from collections.abc import Iterable
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import date, datetime, timedelta
from pathlib import Path
from typing import Any, Iterator


_TOOLS: dict[str, Any] | None = None
_TOOLS_LOCK = threading.Lock()
_PATCH_LOCK = threading.Lock()

_PREFETCH_SCHEMA_VERSION = "vibe.research_evidence.v1"
_PREFETCH_ROLE_ORDER = ("fundamentals", "market", "news", "social")
_PREFETCH_ROLE_ALIASES = {
    "fundamental": "fundamentals",
    "fundamentals": "fundamentals",
    "technical": "market",
    "market": "market",
    "news": "news",
    "sentiment": "social",
    "social": "social",
}
_PREFETCH_DEFAULT_TIMEOUT_SECONDS = 30.0
_PREFETCH_LIST_SAMPLE_SIZE = 16
_PREFETCH_MAX_STRING_CHARS = 2_000

_SIDECAR_ROOT = Path(__file__).resolve().parent
_VENDOR_ROOT = _SIDECAR_ROOT / "vendor"
_SIDECAR_PARENTS = list(_SIDECAR_ROOT.parents)
_LEGACY_BASE = _SIDECAR_PARENTS[1] if len(_SIDECAR_PARENTS) > 1 else _SIDECAR_PARENTS[0]
_LEGACY_VIBE_ROOT = _LEGACY_BASE / "TradingAgent2.0" / "Vibe-Trading"

if str(_VENDOR_ROOT) not in sys.path:
    sys.path.insert(0, str(_VENDOR_ROOT))


@dataclass(frozen=True)
class _PrefetchCall:
    role: str
    evidence_key: str
    tool_name: str
    arguments: dict[str, Any]


def _env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def local_vendor_root() -> Path:
    """Return the repository-owned TradingAgents and bridge source root."""
    return _VENDOR_ROOT


def vibe_trading_root() -> Path:
    configured = os.getenv("VIBE_TRADING_ROOT")
    if configured:
        return Path(configured).expanduser().resolve()
    bundled = _VENDOR_ROOT / "Vibe-Trading"
    if bundled.exists():
        return bundled
    return _LEGACY_VIBE_ROOT


def vibe_tools_enabled() -> bool:
    root = vibe_trading_root()
    legacy_default = _env_bool("ENABLE_TRADINGAGENT2_VIBE_TOOLS", root.exists())
    return _env_bool("ENABLE_VIBE_MCP_TOOLS", legacy_default)


def runtime_status() -> dict[str, Any]:
    vibe_root = vibe_trading_root()
    return {
        "mode": "vendored-tradingagents",
        "vendor_root": str(_VENDOR_ROOT),
        "vendor_available": (_VENDOR_ROOT / "tradingagents").exists() and (_VENDOR_ROOT / "ta_vibe_bridge").exists(),
        "vibe_root": str(vibe_root),
        "vibe_root_available": vibe_root.exists(),
        "vibe_tools_enabled": vibe_tools_enabled(),
        "vibe_tools_loaded": bool(_TOOLS),
        "vibe_tool_names": sorted(_TOOLS or {}),
    }


def _load_tools() -> dict[str, Any]:
    global _TOOLS
    if _TOOLS is not None:
        return _TOOLS

    with _TOOLS_LOCK:
        if _TOOLS is not None:
            return _TOOLS

        vibe_root = vibe_trading_root()
        if not (_VENDOR_ROOT / "tradingagents").exists() or not (_VENDOR_ROOT / "ta_vibe_bridge").exists():
            raise RuntimeError(
                f"Vendored TradingAgents runtime not found at {_VENDOR_ROOT}. Reinstall the CANSLIMAgent source tree."
            )
        if not vibe_root.exists():
            raise RuntimeError(f"Vibe-Trading not found at {vibe_root}. Set VIBE_TRADING_ROOT.")

        from ta_vibe_bridge.config import BridgeConfig
        from ta_vibe_bridge.mcp_data_tools import load_vibe_tools_sync, preflight

        cfg = BridgeConfig(
            vibe_root=vibe_root,
            ta_root=_VENDOR_ROOT,
        )
        problems = preflight(cfg)
        if problems:
            raise RuntimeError("Vibe runtime preflight failed: " + "; ".join(problems))

        _TOOLS = {tool.name: tool for tool in load_vibe_tools_sync(cfg)}
        if not _TOOLS:
            raise RuntimeError("Vibe-Trading exposed no MCP research tools")
        return _TOOLS


def _normalize_prefetch_roles(analysts: Iterable[str] | None) -> tuple[str, ...]:
    if analysts is None:
        return _PREFETCH_ROLE_ORDER
    if isinstance(analysts, str):
        analysts = (analysts,)

    selected: set[str] = set()
    for raw in analysts:
        role = _PREFETCH_ROLE_ALIASES.get(str(raw).strip().lower())
        if role:
            selected.add(role)
    return tuple(role for role in _PREFETCH_ROLE_ORDER if role in selected)


def _normalize_analysis_date(value: str | date | datetime | None) -> date:
    if value is None:
        return date.today()
    if isinstance(value, datetime):
        return value.date()
    if isinstance(value, date):
        return value
    try:
        return date.fromisoformat(str(value).strip()[:10])
    except ValueError as exc:
        raise ValueError("analysis_date must use YYYY-MM-DD format") from exc


def _normalize_vibe_symbols(ticker: str) -> tuple[str, str]:
    symbol = str(ticker or "").strip().upper()
    if not symbol:
        raise ValueError("ticker must be a non-empty symbol")

    if symbol.endswith(".US"):
        return symbol[:-3], symbol
    if "." in symbol or "/" in symbol or symbol.endswith("-USDT"):
        return symbol, symbol
    return symbol, f"{symbol}.US"


def _tool_schema_properties(tool: Any) -> set[str] | None:
    schema = getattr(tool, "args_schema", None)
    if schema is None:
        return None
    if not isinstance(schema, dict):
        try:
            schema = schema.model_json_schema()
        except (AttributeError, TypeError):
            try:
                schema = schema.schema()
            except (AttributeError, TypeError):
                return None
    properties = schema.get("properties") if isinstance(schema, dict) else None
    if not isinstance(properties, dict):
        return None
    return set(properties)


def _tool_supports_argument(tool: Any, argument: str) -> bool:
    properties = _tool_schema_properties(tool)
    return properties is None or argument in properties


def _build_prefetch_calls(
    bare_ticker: str,
    market_symbol: str,
    analysis_date: date,
    roles: tuple[str, ...],
    tools: dict[str, Any],
) -> list[_PrefetchCall]:
    calls: list[_PrefetchCall] = []

    if "fundamentals" in roles:
        calls.append(
            _PrefetchCall(
                "fundamentals",
                "stock_profile",
                "get_stock_profile",
                {
                    "ticker": bare_ticker,
                    "sections": [
                        "key_stats",
                        "financials",
                        "earnings_trend",
                        "institution_ownership",
                        "insider_holders",
                        "recommendation_trend",
                    ],
                },
            )
        )

        financial_tool = tools.get("get_financial_statements")
        base_financial_args = {
            "code": market_symbol,
            "statement": "indicators",
        }
        if financial_tool is None or _tool_supports_argument(financial_tool, "period"):
            for period, label in (("annual", "annual"), ("quarter", "quarterly")):
                calls.append(
                    _PrefetchCall(
                        "fundamentals",
                        f"financial_indicators_{label}",
                        "get_financial_statements",
                        {**base_financial_args, "period": period},
                    )
                )
        else:
            calls.append(
                _PrefetchCall(
                    "fundamentals",
                    "financial_indicators",
                    "get_financial_statements",
                    base_financial_args,
                )
            )

        calls.append(
            _PrefetchCall(
                "fundamentals",
                "sec_filings",
                "get_sec_filings",
                {"ticker": bare_ticker, "limit": 12},
            )
        )

    if "market" in roles:
        calls.append(
            _PrefetchCall(
                "market",
                "market_data",
                "get_market_data",
                {
                    "codes": [market_symbol],
                    "start_date": (analysis_date - timedelta(days=370)).isoformat(),
                    "end_date": analysis_date.isoformat(),
                    "source": "auto",
                    "interval": "1D",
                    "max_rows": 180,
                },
            )
        )

    if "news" in roles:
        calls.append(
            _PrefetchCall(
                "news",
                "stock_news",
                "get_stock_news",
                {"code": market_symbol, "scope": "stock", "limit": 20},
            )
        )

    if "social" in roles:
        calls.append(
            _PrefetchCall(
                "social",
                "options_chain",
                "get_options_chain",
                {"ticker": bare_ticker},
            )
        )

    return calls


def _parse_json_text(value: str) -> Any:
    text = value.strip()
    if text.startswith("```") and text.endswith("```"):
        lines = text.splitlines()
        if len(lines) >= 3:
            text = "\n".join(lines[1:-1]).strip()
    parsed: Any = text
    for _ in range(2):
        if not isinstance(parsed, str):
            break
        candidate = parsed.strip()
        if not candidate or candidate[0] not in "[{\"":
            break
        try:
            parsed = json.loads(candidate)
        except json.JSONDecodeError:
            break
    return parsed


def _normalize_tool_output(value: Any) -> Any:
    if hasattr(value, "model_dump"):
        try:
            value = value.model_dump()
        except (TypeError, ValueError):
            pass
    elif hasattr(value, "content"):
        value = getattr(value, "content")

    if isinstance(value, tuple) and value:
        value = value[0]

    if isinstance(value, str):
        return _parse_json_text(value)

    if isinstance(value, list):
        text_parts: list[str] = []
        all_text = bool(value)
        for item in value:
            if isinstance(item, str):
                text_parts.append(item)
            elif isinstance(item, dict) and item.get("type") == "text":
                text_parts.append(str(item.get("text") or ""))
            else:
                all_text = False
                break
        if all_text:
            return _parse_json_text("\n".join(text_parts))
    return value


def _compact_payload(value: Any, *, depth: int = 0) -> Any:
    if depth >= 8:
        return "[omitted: maximum nesting depth]"
    if value is None or isinstance(value, (bool, int)):
        return value
    if isinstance(value, float):
        return value if math.isfinite(value) else None
    if isinstance(value, str):
        if len(value) <= _PREFETCH_MAX_STRING_CHARS:
            return value
        return value[:_PREFETCH_MAX_STRING_CHARS] + "…[truncated]"
    if isinstance(value, dict):
        return {
            str(key): _compact_payload(item, depth=depth + 1)
            for key, item in value.items()
        }
    if isinstance(value, (list, tuple)):
        items = list(value)
        if len(items) <= _PREFETCH_LIST_SAMPLE_SIZE:
            return [_compact_payload(item, depth=depth + 1) for item in items]
        half = _PREFETCH_LIST_SAMPLE_SIZE // 2
        sample = items[:half] + items[-half:]
        return {
            "type": "list_sample",
            "total_items": len(items),
            "returned_items": len(sample),
            "truncated": True,
            "policy": "first-and-last",
            "items": [_compact_payload(item, depth=depth + 1) for item in sample],
        }
    if hasattr(value, "isoformat"):
        try:
            return value.isoformat()
        except (TypeError, ValueError):
            pass
    return _compact_payload(str(value), depth=depth + 1)


def _payload_error(payload: Any) -> str | None:
    if isinstance(payload, dict):
        if payload.get("ok") is False:
            return str(payload.get("error") or "Vibe-Trading returned ok=false")
        status = str(payload.get("status") or "").strip().lower()
        if status in {"error", "failed", "failure"}:
            return str(payload.get("error") or payload.get("message") or status)
        unresolved = payload.get("_unresolved")
        if unresolved:
            return f"Vibe-Trading returned no market data for: {unresolved}"
    if payload in ({}, [], "", None):
        return "Vibe-Trading returned an empty payload"
    return None


def _tool_message_error(raw_result: Any) -> str | None:
    if str(getattr(raw_result, "status", "")).strip().lower() != "error":
        return None
    content = getattr(raw_result, "content", None)
    normalized = _normalize_tool_output(content)
    nested_error = _payload_error(normalized)
    if nested_error:
        return nested_error
    return str(normalized or "Vibe MCP tool returned an error")


def _payload_source(tool_name: str, payload: Any) -> str:
    if isinstance(payload, dict):
        source = payload.get("source")
        if source:
            return str(source)
    if tool_name == "get_market_data":
        return "vibe-trading:auto"
    return "vibe-trading"


async def _invoke_vibe_tool(tool: Any, arguments: dict[str, Any]) -> Any:
    async_invoke = getattr(tool, "ainvoke", None)
    if callable(async_invoke):
        try:
            result = async_invoke(arguments)
            if inspect.isawaitable(result):
                return await result
            return result
        except NotImplementedError:
            pass

    sync_invoke = getattr(tool, "invoke", None)
    if callable(sync_invoke):
        return await asyncio.to_thread(sync_invoke, arguments)

    async_run = getattr(tool, "arun", None)
    if callable(async_run):
        result = async_run(arguments)
        if inspect.isawaitable(result):
            return await result
        return result

    sync_run = getattr(tool, "run", None)
    if callable(sync_run):
        return await asyncio.to_thread(sync_run, arguments)
    raise TypeError(f"Vibe tool {getattr(tool, 'name', '<unnamed>')} is not invokable")


def _configured_prefetch_timeout(timeout_seconds: float | None) -> float:
    if timeout_seconds is None:
        raw = os.getenv("TRADINGAGENT2_VIBE_TOOL_TIMEOUT_SECONDS", "")
        try:
            timeout_seconds = float(raw) if raw.strip() else _PREFETCH_DEFAULT_TIMEOUT_SECONDS
        except ValueError:
            timeout_seconds = _PREFETCH_DEFAULT_TIMEOUT_SECONDS
    if not math.isfinite(timeout_seconds) or timeout_seconds <= 0:
        return _PREFETCH_DEFAULT_TIMEOUT_SECONDS
    return timeout_seconds


def _trace_for_unavailable(call: _PrefetchCall, error: str) -> dict[str, Any]:
    return {
        "role": call.role,
        "evidence_key": call.evidence_key,
        "tool": call.tool_name,
        "status": "unavailable",
        "duration_ms": 0,
        "source": "vibe-trading",
        "error": error,
        "arguments": _compact_payload(call.arguments),
        "payload": None,
    }


def _add_trace_to_evidence(
    evidence: dict[str, dict[str, Any]], trace: dict[str, Any], trace_index: int
) -> None:
    evidence[trace["role"]][trace["evidence_key"]] = {
        "source_trace_index": trace_index,
        "payload_ref": f"source_traces[{trace_index}].payload",
        "tool": trace["tool"],
        "status": trace["status"],
        "source": trace["source"],
        "error": trace["error"],
    }


async def prefetch_vibe_evidence_async(
    ticker: str,
    *,
    analysis_date: str | date | datetime | None = None,
    analysts: Iterable[str] | None = None,
    timeout_seconds: float | None = None,
) -> dict[str, Any]:
    """Fetch role-specific Vibe evidence before TradingAgents starts.

    Every requested tool produces one source trace. Tool, transport, validation,
    and timeout failures are represented in that trace and never abort the
    other calls. The compact result is safe to place directly under a request's
    ``analysis_context["vibe_trading"]`` key.
    """
    roles = _normalize_prefetch_roles(analysts)
    as_of = _normalize_analysis_date(analysis_date)
    bare_ticker, market_symbol = _normalize_vibe_symbols(ticker)
    timeout = _configured_prefetch_timeout(timeout_seconds)

    tools: dict[str, Any] = {}
    load_error: str | None = None
    if not vibe_tools_enabled():
        load_error = "Vibe-Trading MCP tools are disabled"
    else:
        try:
            # _load_tools uses the bridge's synchronous loader (asyncio.run), so
            # keep it out of this event loop even when this API is called async.
            tools = await asyncio.to_thread(_load_tools)
        except Exception as exc:  # noqa: BLE001 - degrade the whole tool set
            load_error = f"Vibe-Trading MCP tool loading failed: {exc}"

    calls = _build_prefetch_calls(
        bare_ticker,
        market_symbol,
        as_of,
        roles,
        tools,
    )
    evidence: dict[str, dict[str, Any]] = {role: {} for role in roles}
    traces: list[dict[str, Any]] = []

    for call in calls:
        tool = tools.get(call.tool_name)
        if load_error or tool is None:
            error = load_error or f"Vibe-Trading did not expose {call.tool_name}"
            trace = _trace_for_unavailable(call, error)
            traces.append(trace)
            _add_trace_to_evidence(evidence, trace, len(traces) - 1)
            continue

        started = time.perf_counter()
        raw_result: Any = None
        try:
            raw_result = await asyncio.wait_for(
                _invoke_vibe_tool(tool, call.arguments),
                timeout=timeout,
            )
            payload = _normalize_tool_output(raw_result)
            error = _tool_message_error(raw_result) or _payload_error(payload)
            status = "error" if error else "ok"
        except TimeoutError:
            payload = None
            status = "error"
            error = f"timed out after {timeout:g}s"
        except Exception as exc:  # noqa: BLE001 - isolate every Vibe tool call
            payload = None
            status = "error"
            error = str(exc).strip() or exc.__class__.__name__

        trace = {
            "role": call.role,
            "evidence_key": call.evidence_key,
            "tool": call.tool_name,
            "status": status,
            "duration_ms": max(0, round((time.perf_counter() - started) * 1000)),
            "source": _payload_source(call.tool_name, payload),
            "error": _compact_payload(error) if error else None,
            "arguments": _compact_payload(call.arguments),
            "payload": _compact_payload(payload),
        }
        traces.append(trace)
        _add_trace_to_evidence(evidence, trace, len(traces) - 1)

    ok_count = sum(trace["status"] == "ok" for trace in traces)
    requested_count = len(traces)
    if requested_count == 0:
        overall_status = "not_requested"
    elif ok_count == requested_count:
        overall_status = "complete"
    elif ok_count:
        overall_status = "partial"
    else:
        overall_status = "unavailable"

    return {
        "schema_version": _PREFETCH_SCHEMA_VERSION,
        "provider": "vibe-trading",
        "symbol": bare_ticker,
        "market_symbol": market_symbol,
        "analysis_date": as_of.isoformat(),
        "status": overall_status,
        "requested_roles": list(roles),
        "summary": {
            "requested": requested_count,
            "ok": ok_count,
            "failed": requested_count - ok_count,
        },
        "evidence": evidence,
        "source_traces": traces,
    }


def prefetch_vibe_evidence(
    ticker: str,
    *,
    analysis_date: str | date | datetime | None = None,
    analysts: Iterable[str] | None = None,
    timeout_seconds: float | None = None,
) -> dict[str, Any]:
    """Synchronous wrapper for :func:`prefetch_vibe_evidence_async`.

    ``run_analysis`` normally calls this from its executor thread. If a caller
    already owns an event loop, the coroutine runs in a short helper thread;
    async callers should prefer ``prefetch_vibe_evidence_async`` to avoid that
    blocking join.
    """
    kwargs = {
        "analysis_date": analysis_date,
        "analysts": analysts,
        "timeout_seconds": timeout_seconds,
    }

    try:
        asyncio.get_running_loop()
    except RuntimeError:
        return asyncio.run(prefetch_vibe_evidence_async(ticker, **kwargs))

    result: dict[str, Any] = {}
    failure: list[BaseException] = []

    def run_in_helper_thread() -> None:
        try:
            result["value"] = asyncio.run(prefetch_vibe_evidence_async(ticker, **kwargs))
        except BaseException as exc:  # pragma: no cover - defensive handoff
            failure.append(exc)

    thread = threading.Thread(
        target=run_in_helper_thread,
        name="vibe-prefetch",
        daemon=True,
    )
    thread.start()
    thread.join()
    if failure:
        raise failure[0]
    return result["value"]


@contextmanager
def patched_tradingagents_graph() -> Iterator[None]:
    """Patch TradingAgents during graph construction, then restore globals."""
    if not vibe_tools_enabled():
        yield
        return

    tools = _load_tools()
    from ta_vibe_bridge.ta_patch import apply_mcp_tools

    with _PATCH_LOCK:
        undo = apply_mcp_tools(tools)
        try:
            yield
        finally:
            undo()
