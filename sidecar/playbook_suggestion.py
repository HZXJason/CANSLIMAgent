"""Evidence-bounded strategy playbook suggestions that always require user confirmation."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage


SYSTEM_PROMPT = """你是交易复盘助手，不是交易决策者。
根据输入中已完成的交易复盘和行为观察，提出最多 3 条可由用户编辑确认的策略剧本建议。
不得评价用户性格，不得给出 BUY、SELL、HOLD 或批准交易的结论。
每条建议只能引用输入中存在的 review id；证据不足时返回空 suggestions。
输出严格 JSON：{"suggestions":[{"name":"","markets":[],"horizons":[],"market_context":"","entry_rules":"","invalidation_rules":"","stop_rules":"","target_rules":"","position_rules":"","no_trade_rules":"","notes":"","evidence_review_ids":[]}]}。"""


FIELDS = (
    "market_context", "entry_rules", "invalidation_rules", "stop_rules",
    "target_rules", "position_rules", "no_trade_rules", "notes",
)


def _extract_json(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    text = str(value or "").strip()
    if text.startswith("```"):
        text = text.split("\n", 1)[-1].rsplit("```", 1)[0].strip()
    parsed = json.loads(text)
    if not isinstance(parsed, dict):
        raise ValueError("playbook suggestions must be an object")
    return parsed


def validate_playbook_suggestions(payload: dict[str, Any], allowed_review_ids: set[str]) -> dict[str, Any]:
    suggestions = payload.get("suggestions")
    if not isinstance(suggestions, list) or len(suggestions) > 3:
        raise ValueError("suggestions must be a bounded list")
    normalized: list[dict[str, Any]] = []
    for suggestion in suggestions:
        if not isinstance(suggestion, dict):
            raise ValueError("suggestion must be an object")
        evidence = suggestion.get("evidence_review_ids")
        if not isinstance(evidence, list) or any(str(item) not in allowed_review_ids for item in evidence):
            raise ValueError("suggestion references unknown review evidence")
        name = str(suggestion.get("name") or "").strip()
        if not name or not evidence:
            raise ValueError("suggestion requires a name and review evidence")
        item: dict[str, Any] = {
            "name": name,
            "markets": [str(value).upper() for value in suggestion.get("markets", []) if str(value).strip()],
            "horizons": [str(value).lower() for value in suggestion.get("horizons", []) if str(value).strip()],
        }
        for field in FIELDS:
            item[field] = str(suggestion.get(field) or "").strip()
        item["evidence_review_ids"] = [str(value) for value in evidence]
        item["evidence_count"] = len(evidence)
        item["generated_at"] = datetime.now(timezone.utc).isoformat()
        normalized.append(item)
    return {"suggestions": normalized}


def suggest_playbooks(payload: dict[str, Any], llm: Any) -> dict[str, Any]:
    reviews = payload.get("reviews") or []
    allowed_ids = {str(review.get("id")) for review in reviews if isinstance(review, dict) and review.get("id")}
    response = llm.invoke([
        SystemMessage(content=SYSTEM_PROMPT),
        HumanMessage(content="以下 JSON 是唯一可用证据：\n" + json.dumps(payload, ensure_ascii=False, default=str)),
    ])
    return validate_playbook_suggestions(_extract_json(getattr(response, "content", response)), allowed_ids)
