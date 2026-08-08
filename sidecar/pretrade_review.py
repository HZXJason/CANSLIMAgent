"""Structured, evidence-bounded pre-trade AI risk review."""

from __future__ import annotations

import json
import re
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage


PROHIBITED_DECISION_LANGUAGE = re.compile(
    r"(?:批准|计划通过|交易通过|可以买|可以卖|建议买入|建议卖出|替你决定|\bBUY\b|\bSELL\b|\bHOLD\b)",
    re.IGNORECASE,
)
FINDING_GROUPS = (
    "risk_warnings",
    "behavior_reminders",
    "unresolved_questions",
    "changes_since_research",
)
ALLOWED_SEVERITIES = {"low", "medium", "high"}
MAX_FINDINGS_PER_GROUP = 20


SYSTEM_PROMPT = """你是交易计划的风险审查员，不是交易决策者。
只补充用户可能遗漏的技术、新闻、情绪、事件、行为和数据风险。
禁止批准或否决交易，禁止输出 BUY、SELL、HOLD、建议买入、建议卖出或任何许可语义。
只能引用输入 JSON 中出现的来源 ID；没有证据时必须放入 unresolved_questions 并说明未知原因。
每条 high 风险必须在 evidence.source_ids 中引用至少一个输入来源。
输出严格 JSON，字段只能是 risk_warnings、behavior_reminders、unresolved_questions、changes_since_research、freshness、sources。"""


def _extract_json(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    text = str(value or "").strip()
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text, flags=re.IGNORECASE)
        text = re.sub(r"\s*```$", "", text)
    parsed = json.loads(text)
    if not isinstance(parsed, dict):
        raise ValueError("pre-trade review must be a JSON object")
    return parsed


def collect_allowed_sources(payload: dict[str, Any]) -> dict[str, dict[str, Any]]:
    allowed: dict[str, dict[str, Any]] = {}

    def visit(value: Any) -> None:
        if isinstance(value, dict):
            sources = value.get("sources")
            if isinstance(sources, list):
                for source in sources:
                    if not isinstance(source, dict):
                        continue
                    source_id = str(source.get("id") or "").strip()
                    if source_id:
                        allowed[source_id] = dict(source)
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    visit(payload.get("research_evidence", {}))
    visit(payload.get("current_evidence", {}))
    visit(payload.get("behavior_observations", []))
    return allowed


def validate_review_payload(payload: dict[str, Any], allowed_sources: dict[str, dict[str, Any]]) -> dict[str, Any]:
    expected_keys = {*FINDING_GROUPS, "freshness", "sources"}
    if set(payload) != expected_keys:
        raise ValueError("pre-trade review has invalid top-level fields")
    normalized: dict[str, Any] = {}
    for group in FINDING_GROUPS:
        findings = payload.get(group)
        if not isinstance(findings, list) or len(findings) > MAX_FINDINGS_PER_GROUP:
            raise ValueError(f"{group} must be a bounded list")
        normalized_findings: list[dict[str, Any]] = []
        for finding in findings:
            if not isinstance(finding, dict):
                raise ValueError(f"{group} contains an invalid finding")
            code = str(finding.get("code") or "").strip()
            severity = str(finding.get("severity") or "").strip().lower()
            category = str(finding.get("category") or "").strip()
            message = str(finding.get("message") or "").strip()
            evidence = finding.get("evidence")
            if not code or not category or not message or severity not in ALLOWED_SEVERITIES or not isinstance(evidence, dict):
                raise ValueError(f"{group} contains an incomplete finding")
            if PROHIBITED_DECISION_LANGUAGE.search(message):
                raise ValueError("pre-trade review contains prohibited decision language")
            source_ids = evidence.get("source_ids", [])
            if not isinstance(source_ids, list) or any(source_id not in allowed_sources for source_id in source_ids):
                raise ValueError("finding references an unknown source")
            if group == "risk_warnings" and severity == "high" and not source_ids:
                raise ValueError("high-risk finding requires a source citation")
            normalized_findings.append(
                {
                    "code": code,
                    "severity": severity,
                    "category": category,
                    "message": message,
                    "field": finding.get("field"),
                    "evidence": evidence,
                }
            )
        normalized[group] = normalized_findings

    freshness = payload.get("freshness")
    if not isinstance(freshness, dict):
        raise ValueError("freshness must be an object")
    normalized["freshness"] = freshness

    sources = payload.get("sources")
    if not isinstance(sources, list) or len(sources) > 50:
        raise ValueError("sources must be a bounded list")
    normalized_sources: list[dict[str, Any]] = []
    for source in sources:
        if not isinstance(source, dict):
            raise ValueError("source is invalid")
        source_id = str(source.get("id") or "").strip()
        allowed = allowed_sources.get(source_id)
        if allowed is None:
            raise ValueError("source was not supplied")
        supplied_url = allowed.get("url")
        if source.get("url") != supplied_url:
            raise ValueError("source URL does not match supplied evidence")
        normalized_sources.append(
            {
                "id": source_id,
                "label": str(source.get("label") or allowed.get("label") or source_id),
                "url": supplied_url,
                "data_as_of": source.get("data_as_of") or allowed.get("data_as_of"),
                "status": str(source.get("status") or allowed.get("status") or "available"),
            }
        )
    normalized["sources"] = normalized_sources
    return normalized


def review_with_llm(payload: dict[str, Any], llm: Any) -> dict[str, Any]:
    allowed_sources = collect_allowed_sources(payload)
    response = llm.invoke(
        [
            SystemMessage(content=SYSTEM_PROMPT),
            HumanMessage(content="以下 JSON 是唯一可用上下文：\n" + json.dumps(payload, ensure_ascii=False, default=str)),
        ]
    )
    content = getattr(response, "content", response)
    return validate_review_payload(_extract_json(content), allowed_sources)
