"""Surgical Operator 1: Evidence Card Compiler (deterministic + single-hop counter-thesis)."""

from __future__ import annotations

import json
import re
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage

from data_auditor import audit_data
from pretrade_review import PROHIBITED_DECISION_LANGUAGE


COUNTER_THESIS_PROMPT = """你是 AlphaLens 的买方研究审查助手（非投顾、非交易员）。
基于提供的结构化事实卡片与来源列表，提炼最多 3 条核心看多锚点（bull_anchors）、最多 3 条具体可证伪看空/反证条件（bear_falsifiers）以及最多 3 条信息盲点（blind_spots）。
严禁出现 BUY、SELL、HOLD、建议买入、建议卖出、批准或任何仓位指令。
只能引用输入 sources 中出现的 source_id；没有依据的内容不得编造。
输出严格 JSON，字段只能是 bull_anchors、bear_falsifiers、blind_spots。
每个列表项必须是对象：{"title": "...", "detail": "...", "source_ids": ["..."]}。"""


def _as_dict(value: Any) -> dict[str, Any]:
    return value if isinstance(value, dict) else {}


def _as_list(value: Any) -> list[Any]:
    return value if isinstance(value, list) else []


def compile_deterministic_cards(
    symbol: str,
    context: dict[str, Any],
    audit: dict[str, Any] | None = None,
) -> dict[str, Any]:
    ticker = str(symbol or "").strip().upper()
    summary = _as_dict(context.get("stock_summary"))
    metrics = _as_dict(summary.get("metrics"))
    eps_ctx = _as_dict(context.get("eps"))
    rev_ctx = _as_dict(context.get("revenue"))
    margin_ctx = _as_dict(context.get("gross_margin"))
    kline_stats = _as_dict(context.get("kline_stats"))
    events = _as_list(context.get("upcoming_events"))

    audit_result = audit if isinstance(audit, dict) else audit_data({}, context)

    # 1. Growth & margin inflection card
    growth_items: list[dict[str, Any]] = []
    c_eps = metrics.get("c_eps_growth_pct")
    if metrics.get("c_data_sufficient") and c_eps is not None:
        growth_items.append({"label": "当季 EPS 同比 (C)", "value": f"{float(c_eps):+.2f}%", "status": "ok"})
    else:
        growth_items.append({"label": "当季 EPS 同比 (C)", "value": "未知（数据不足）", "status": "unknown"})

    annual = _as_list(metrics.get("a_annual_eps_growth"))
    if metrics.get("a_data_sufficient") and annual:
        growth_items.append({"label": "近3年 EPS 增速 (A)", "value": " / ".join(f"{float(v):+.1f}%" for v in annual[:3]), "status": "ok"})
    else:
        growth_items.append({"label": "近3年 EPS 增速 (A)", "value": "未知（数据不足）", "status": "unknown"})

    quarters_margin = _as_list(margin_ctx.get("quarters"))
    if quarters_margin:
        latest_gm = _as_dict(quarters_margin[0])
        gm_val = latest_gm.get("gross_margin")
        if gm_val is not None:
            growth_items.append({"label": f"最新毛利率 ({latest_gm.get('period', '-')})", "value": f"{float(gm_val):.2f}%", "status": "ok"})
    if not any(item["label"].startswith("最新毛利率") for item in growth_items):
        growth_items.append({"label": "最新毛利率", "value": "未知", "status": "unknown"})

    # 2. Price, volume, RS & volatility card
    price_items: list[dict[str, Any]] = []
    if metrics.get("l_data_sufficient") and metrics.get("l_rs_rating") is not None:
        price_items.append({"label": "RS 相对强度 (L)", "value": f"{metrics.get('l_rs_rating')}", "status": "ok"})
    else:
        price_items.append({"label": "RS 相对强度 (L)", "value": "未知", "status": "unknown"})

    if metrics.get("s_data_sufficient") and metrics.get("s_volume_ratio") is not None:
        price_items.append({"label": "量比 (S)", "value": f"{float(metrics.get('s_volume_ratio')):.2f}x", "status": "ok"})
    else:
        price_items.append({"label": "量比 (S)", "value": "未知", "status": "unknown"})

    if kline_stats.get("latest_close") is not None:
        price_items.append({"label": "最新收盘价", "value": f"{float(kline_stats['latest_close']):.2f}", "status": "ok"})
    if kline_stats.get("atr14") is not None and kline_stats.get("atr14_percent") is not None:
        price_items.append(
            {
                "label": "ATR(14) 日均波幅",
                "value": f"{float(kline_stats['atr14']):.2f} ({float(kline_stats['atr14_percent']):.2f}%)",
                "status": "ok",
            }
        )
    if kline_stats.get("distance_to_52w_high_percent") is not None:
        price_items.append(
            {
                "label": "距 52 周高点",
                "value": f"{float(kline_stats['distance_to_52w_high_percent']):+.2f}%",
                "status": "ok",
            }
        )

    # 3. Catalysts & upcoming events card
    catalyst_items: list[dict[str, Any]] = []
    n_summary = str(summary.get("n_analysis_summary") or "").strip()
    if n_summary:
        catalyst_items.append({"label": "新催化记录 (N)", "value": n_summary, "status": "ok"})
    for event in events[:4]:
        ev = _as_dict(event)
        catalyst_items.append(
            {
                "label": f"{ev.get('date', '-')} · {ev.get('kind', '事件')}",
                "value": str(ev.get("title") or "未命名事件"),
                "status": "warning",
            }
        )
    if not catalyst_items:
        catalyst_items.append({"label": "事件窗口", "value": "暂无近期日历事件（仍需下单前人工核查财报日）", "status": "unknown"})

    # 4. Data audit & unknowns card
    audit_items: list[dict[str, Any]] = []
    findings = _as_list(audit_result.get("findings"))
    for finding in findings[:5]:
        f = _as_dict(finding)
        audit_items.append(
            {
                "label": f"{f.get('field', '字段')} ({f.get('period', '-')})",
                "value": f"[{f.get('status', 'unknown')}] {f.get('reason', '')}",
                "status": "warning" if f.get("status") in {"disputed", "unverified"} else "ok",
            }
        )
    if not audit_items:
        audit_items.append({"label": "来源审计", "value": "已按可用数据源日期快照对齐", "status": "ok"})

    sources = _collect_sources(summary, eps_ctx, rev_ctx, margin_ctx, kline_stats)

    return {
        "symbol": ticker,
        "data_as_of": summary.get("data_as_of") or kline_stats.get("data_as_of"),
        "cards": [
            {"id": "growth_inflection", "title": "C / A 增长与毛利拐点", "items": growth_items},
            {"id": "price_volume_rs", "title": "S / L 量价结构与波动率", "items": price_items},
            {"id": "catalysts_and_events", "title": "N 催化与事件风险窗口", "items": catalyst_items},
            {"id": "audit_and_unknowns", "title": "数据审计与未知边界", "items": audit_items},
        ],
        "sources": sources,
        "ai_status": "not_requested",
        "counter_thesis": None,
    }


def _collect_sources(
    summary: dict[str, Any],
    eps_ctx: dict[str, Any],
    rev_ctx: dict[str, Any],
    margin_ctx: dict[str, Any],
    kline_stats: dict[str, Any],
) -> list[dict[str, Any]]:
    sources: list[dict[str, Any]] = []
    if summary.get("data_as_of"):
        sources.append({"id": "summary-metrics", "label": "CANSLIM Screener Metrics", "data_as_of": summary.get("data_as_of"), "status": "available"})
    for key, label, payload in (
        ("eps-series", "EPS Quarterly", eps_ctx),
        ("revenue-series", "Revenue Annual", rev_ctx),
        ("gross-margin", "Gross Margin", margin_ctx),
        ("kline-daily", "Daily Price & ATR", kline_stats),
    ):
        src = str(payload.get("source") or "").strip()
        if src or payload.get("data_as_of"):
            sources.append(
                {
                    "id": key,
                    "label": f"{label} ({src or 'local'})",
                    "data_as_of": payload.get("data_as_of"),
                    "status": "available",
                }
            )
    return sources


def _extract_json(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    text = str(value or "").strip()
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text, flags=re.IGNORECASE)
        text = re.sub(r"\s*```$", "", text)
    parsed = json.loads(text)
    if not isinstance(parsed, dict):
        raise ValueError("counter thesis must be a JSON object")
    return parsed


def validate_counter_thesis_payload(payload: dict[str, Any], allowed_source_ids: set[str]) -> dict[str, Any]:
    expected = {"bull_anchors", "bear_falsifiers", "blind_spots"}
    if set(payload) != expected:
        raise ValueError("counter thesis has invalid top-level keys")
    normalized: dict[str, list[dict[str, Any]]] = {}
    for key in ("bull_anchors", "bear_falsifiers", "blind_spots"):
        raw_items = payload.get(key)
        if not isinstance(raw_items, list) or len(raw_items) > 3:
            raise ValueError(f"{key} must be a list of at most 3 items")
        clean_items: list[dict[str, Any]] = []
        for item in raw_items:
            if not isinstance(item, dict):
                raise ValueError(f"{key} item must be an object")
            title = str(item.get("title") or "").strip()
            detail = str(item.get("detail") or "").strip()
            source_ids = item.get("source_ids", [])
            if not title or not detail:
                raise ValueError(f"{key} item missing title or detail")
            if PROHIBITED_DECISION_LANGUAGE.search(title + " " + detail):
                raise ValueError("counter thesis contains prohibited decision language")
            if not isinstance(source_ids, list) or any(sid not in allowed_source_ids for sid in source_ids):
                raise ValueError(f"{key} references an unknown source_id")
            clean_items.append({"title": title, "detail": detail, "source_ids": list(source_ids)})
        normalized[key] = clean_items
    return normalized


def enrich_counter_thesis_with_llm(card_pack: dict[str, Any], llm: Any) -> dict[str, Any]:
    allowed_ids = {str(src.get("id")) for src in _as_list(card_pack.get("sources")) if _as_dict(src).get("id")}
    prompt_payload = {
        "symbol": card_pack.get("symbol"),
        "data_as_of": card_pack.get("data_as_of"),
        "cards": card_pack.get("cards"),
        "sources": card_pack.get("sources"),
    }
    response = llm.invoke(
        [
            SystemMessage(content=COUNTER_THESIS_PROMPT),
            HumanMessage(content="以下结构化证据是唯一可用输入：\n" + json.dumps(prompt_payload, ensure_ascii=False, default=str)),
        ]
    )
    content = getattr(response, "content", response)
    validated = validate_counter_thesis_payload(_extract_json(content), allowed_ids)
    updated = dict(card_pack)
    updated["ai_status"] = "completed"
    updated["counter_thesis"] = validated
    return updated
