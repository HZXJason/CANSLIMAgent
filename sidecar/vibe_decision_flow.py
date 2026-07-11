"""Decision-stage Vibe support for the TradingAgents sidecar.

The MCP bridge supplies raw evidence and optional tools.  This module turns that
material into small, explicit decision artifacts: a role-scoped research pack,
an evidence-coverage reconciliation, and a conservative execution gate.  It
intentionally does not manufacture a market conclusion when Vibe is unavailable.
"""

from __future__ import annotations

import os
from datetime import date, timedelta
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


def execution_validation_enabled() -> bool:
    return os.getenv("ENABLE_VIBE_EXECUTION_VALIDATION", "true").strip().lower() not in {"0", "false", "no", "off"}


def validate_execution_plan(ticker: str, decision: Any, trader_plan: str, *, as_of: date | None = None) -> dict[str, Any]:
    """Backtest an actionable PM decision through Vibe without writing memory.

    The bridge uses Vibe's own interpreter.  A failure returns an explicit
    unverified result; it never stands in for a successful backtest.
    """
    if not execution_validation_enabled():
        return {"schema_version": "vibe.execution_validation.v1", "status": "disabled"}

    try:
        from ta_vibe_bridge.closed_loop import verify_decision

        end = as_of or date.today()
        start = end - timedelta(days=365 * 3)
        timeout = max(10, int(os.getenv("VIBE_EXECUTION_VALIDATION_TIMEOUT_SECONDS", "90")))
        result = verify_decision(
            ticker,
            end.isoformat(),
            str(decision or ""),
            start.isoformat(),
            end.isoformat(),
            trader_plan=str(trader_plan or ""),
            write_back=False,
            timeout=timeout,
        )
    except Exception as exc:  # noqa: BLE001 - a secondary validation must not erase a real analysis
        return {
            "schema_version": "vibe.execution_validation.v1",
            "status": "unavailable",
            "error": str(exc).strip() or exc.__class__.__name__,
        }

    if not result.backtest_ok:
        status = "not_actionable" if "non-actionable" in (result.error or "").lower() else "unavailable"
        return {
            "schema_version": "vibe.execution_validation.v1",
            "status": status,
            "symbol": result.ticker,
            "error": result.error or "Vibe did not produce a validated backtest",
        }

    backtest = result.reflection
    # ``verify_decision`` returns its reflection as the stable public artifact;
    # headline fields below are intentionally derived from that bridge result.
    return {
        "schema_version": "vibe.execution_validation.v1",
        "status": "complete",
        "symbol": result.ticker,
        "reflection": backtest,
        "run_dir": result.run_dir,
    }


def render_execution_validation(validation: dict[str, Any]) -> str:
    status = validation.get("status", "unknown")
    if status == "complete":
        return "\n".join(
            [
                "Vibe 已对 PM 的可执行信号完成历史回测验证；它是历史模拟，不是未来收益承诺。",
                "",
                str(validation.get("reflection") or "已完成验证，但未返回可展示的摘要。"),
            ]
        )
    if status == "not_actionable":
        return "Vibe 执行验证未运行：PM 建议为 HOLD 或无法量化的非交易信号，不应伪造回测结果。"
    if status == "disabled":
        return "Vibe 执行验证已关闭；最终决策没有经过历史执行校验。"
    return "Vibe 执行验证不可用；保留 TradingAgents 原始结论，但该结论应视为**未经过历史执行校验**。\n\n> 原因：" + str(validation.get("error") or "unknown")


def apply_execution_gate(action: str, validation: dict[str, Any]) -> tuple[str, str]:
    """Apply only a conservative, explainable gate to the final UI action.

    We do not invent a better trade from a failed or unavailable backtest.
    Only a completed Vibe verdict that explicitly says the signal did not hold
    up can block an actionable call; missing validation remains visible but
    leaves the original TradingAgents direction intact.
    """
    normalized = action if action in {"BUY", "SELL", "HOLD"} else "HOLD"
    status = validation.get("status")
    if status == "complete":
        reflection = str(validation.get("reflection") or "").lower()
        if normalized in {"BUY", "SELL"} and (
            "did not hold up" in reflection or "not significant (looks random)" in reflection
        ):
            return "HOLD", "Vibe 回测显示该信号未通过稳健性检验，已阻止直接执行，等待新的入场条件。"
        return normalized, "已通过 Vibe 历史执行验证，仍需人工确认入场条件和风险预算。"
    if status in {"unavailable", "disabled"}:
        return normalized, "执行校验不可用；保留原始方向，但该建议未经过历史执行验证。"
    return normalized, "该建议没有可执行的历史回测样本；维持原始方向，仅供人工研究。"
