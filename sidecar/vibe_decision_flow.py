"""Decision-stage Vibe support for the TradingAgents sidecar.

The MCP bridge supplies raw evidence and optional tools.  This module turns that
material into small, explicit decision artifacts: a role-scoped research pack
and an evidence-coverage reconciliation. It intentionally does not manufacture
a market conclusion when Vibe is unavailable.
"""

from __future__ import annotations

from typing import Any

_ROLE_LABELS = {
    "fundamentals": "基本面",
    "market": "技术面",
    "news": "新闻",
    "social": "情绪/期权",
}


def build_research_pack(bundle: dict[str, Any]) -> dict[str, Any]:
    """Create an auditable, compact prompt contract from raw Vibe traces."""
    traces = [item for item in bundle.get("source_traces", []) if isinstance(item, dict)]
    roles: dict[str, dict[str, Any]] = {}
    for trace in traces:
        role = str(trace.get("role") or "unknown")
        slot = roles.setdefault(role, {"requested": 0, "available": 0, "sources": [], "gaps": []})
        slot["requested"] += 1
        source = str(trace.get("source") or "vibe-trading")
        if source not in slot["sources"]:
            slot["sources"].append(source)
        if trace.get("status") == "ok":
            slot["available"] += 1
        else:
            slot["gaps"].append(
                {
                    "evidence": str(trace.get("evidence_key") or trace.get("tool") or "unknown"),
                    "reason": str(trace.get("error") or "unavailable"),
                }
            )

    for role, slot in roles.items():
        slot["label"] = _ROLE_LABELS.get(role, role)
        slot["coverage"] = "complete" if slot["requested"] and slot["available"] == slot["requested"] else (
            "partial" if slot["available"] else "unavailable"
        )

    return {
        "schema_version": "vibe.research_pack.v1",
        "symbol": bundle.get("market_symbol") or bundle.get("symbol"),
        "status": bundle.get("status", "unknown"),
        "roles": roles,
        "rules": [
            "Only treat successful Vibe evidence as factual support.",
            "Mark a claim unknown when its relevant evidence is unavailable or stale.",
            "Name the evidence source when it materially supports or contradicts a claim.",
            "Separate data facts from analyst inference, and state disconfirming evidence.",
        ],
    }


def render_evidence_reconciliation(bundle: dict[str, Any], state: dict[str, Any], selected_roles: tuple[str, ...]) -> str:
    """Report which analyst conclusions have usable evidence coverage.

    This is deliberately an availability reconciliation, not a claim of truth:
    the downstream debate still decides what the evidence means.
    """
    reports_by_role = {
        "fundamentals": bool(state.get("fundamentals_report")),
        "social": bool(state.get("sentiment_report")),
        "news": bool(state.get("news_report")),
        "market": bool(state.get("market_report")),
    }
    pack = build_research_pack(bundle)
    lines = [
        "Vibe 证据对账把已选分析师的报告与可用数据覆盖度对应起来；它不把数据可用误写成结论成立。",
        "",
        "| 分析维度 | 分析师报告 | Vibe 覆盖 | 可用/请求 | 数据来源 |",
        "|---|---|---|---:|---|",
    ]
    for role in selected_roles:
        slot = pack["roles"].get(role, {"coverage": "not_requested", "available": 0, "requested": 0, "sources": []})
        report = "已生成" if reports_by_role.get(role) else "未生成"
        coverage = {"complete": "完整", "partial": "部分", "unavailable": "不可用", "not_requested": "未请求"}.get(
            slot["coverage"], slot["coverage"]
        )
        sources = ", ".join(f"`{source}`" for source in slot.get("sources", [])) or "-"
        lines.append(
            f"| {_ROLE_LABELS.get(role, role)} | {report} | {coverage} | {slot.get('available', 0)}/{slot.get('requested', 0)} | {sources} |"
        )

    gaps = [gap for slot in pack["roles"].values() for gap in slot.get("gaps", [])]
    if gaps:
        lines.extend(["", "**必须保留为未知的证据缺口**："])
        lines.extend(f"- `{gap['evidence']}`：{gap['reason']}" for gap in gaps)
    return "\n".join(lines)
