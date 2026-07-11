"""Period-aware audit of Vibe evidence before it reaches analyst prompts.

An LLM can explain an evidence dispute but cannot establish which number is
correct.  This module resolves only the cases that can be computed from source
metadata: same metric, same period, known source and a declared report form.
Everything else is preserved as an explicit unresolved/blocked finding.
"""

from __future__ import annotations

from typing import Any


_REVENUE_KEY = "RevenueFromContractWithCustomerExcludingAssessedTax"
_EPS_KEY = "EarningsPerShareDiluted"


def _as_dict(value: Any) -> dict[str, Any]:
    return value if isinstance(value, dict) else {}


def _items(value: Any) -> list[dict[str, Any]]:
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    if isinstance(value, dict):
        sampled = value.get("items")
        if isinstance(sampled, list):
            return [item for item in sampled if isinstance(item, dict)]
    return []


def _trace(bundle: dict[str, Any], evidence_key: str) -> dict[str, Any]:
    for candidate in bundle.get("source_traces", []):
        if isinstance(candidate, dict) and candidate.get("evidence_key") == evidence_key:
            return candidate
    return {}


def _period_rows(trace: dict[str, Any]) -> list[dict[str, Any]]:
    payload = _as_dict(trace.get("payload"))
    data = _as_dict(payload.get("data"))
    for symbol_payload in data.values():
        periods = _as_dict(symbol_payload).get("periods")
        rows = _items(periods)
        if rows:
            return rows
    return []


def _latest_context_year(context: dict[str, Any]) -> dict[str, Any]:
    years = _as_dict(context.get("revenue")).get("years")
    candidates = [item for item in years or [] if isinstance(item, dict) and isinstance(item.get("year"), int)]
    return max(candidates, key=lambda item: item["year"], default={})


def _latest_context_quarter(context: dict[str, Any]) -> dict[str, Any]:
    quarters = _as_dict(context.get("eps")).get("quarters")
    candidates = [item for item in quarters or [] if isinstance(item, dict) and item.get("period")]
    return candidates[0] if candidates else {}


def _raw_quarter_row(rows: list[dict[str, Any]], period: str) -> dict[str, Any]:
    for row in rows:
        if row.get("FORM") == "10-Q" and f"{row.get('FISCAL_YEAR')}" + f"{row.get('FISCAL_PERIOD')}" == period:
            return row
    return {}


def _raw_annual_row(rows: list[dict[str, Any]], year: int) -> dict[str, Any]:
    # Vibe's compact SEC row does not include XBRL duration (start/end).  A
    # 10-K can therefore contain a prior comparative quarter tagged FY.  Return
    # the row for comparison, but never treat it as a standalone annual fact.
    for row in rows:
        if row.get("FORM") == "10-K" and row.get("REPORT_DATE") == f"{year}-12-31":
            return row
    return {}


def _number(value: Any) -> float | None:
    return float(value) if isinstance(value, (int, float)) else None


def _close(left: float, right: float, tolerance: float = 0.001) -> bool:
    scale = max(abs(left), abs(right), 1.0)
    return abs(left - right) / scale <= tolerance


def _candidate(source: str, value: float | None, *, period: str, detail: str, accession: str | None = None) -> dict[str, Any]:
    result: dict[str, Any] = {"source": source, "value": value, "period": period, "detail": detail}
    if accession:
        result["accession"] = accession
    return result


def audit_data(bundle: dict[str, Any], application_context: dict[str, Any]) -> dict[str, Any]:
    """Build a strict financial audit contract from current-run evidence.

    Trust order is intentionally narrow: the CANSLIM backend's SEC series is
    accepted for annual revenue because it keeps annual duration semantics;
    raw Vibe 10-Q rows are accepted for a matching quarter.  A raw 10-K row
    lacking duration metadata is never allowed to overwrite an annual value.
    """
    annual_trace = _trace(bundle, "financial_indicators_annual")
    quarterly_trace = _trace(bundle, "financial_indicators_quarterly")
    annual_rows = _period_rows(annual_trace)
    quarterly_rows = _period_rows(quarterly_trace)
    findings: list[dict[str, Any]] = []
    canonical: list[dict[str, Any]] = []
    blocked: list[dict[str, Any]] = []

    annual = _latest_context_year(application_context)
    if annual:
        year = int(annual["year"])
        app_value = _number(annual.get("revenue"))
        raw = _raw_annual_row(annual_rows, year)
        raw_value = _number(raw.get(_REVENUE_KEY))
        candidates = [
            _candidate(
                "CANSLIM SEC annual series",
                app_value,
                period=str(year),
                detail="period-aware annual series with data_sufficient flag",
            )
        ]
        if raw:
            candidates.append(
                _candidate(
                    "Vibe SEC compact 10-K row",
                    raw_value,
                    period=str(year),
                    detail="duration omitted; may be a comparative quarter in the latest 10-K",
                    accession=str(raw.get("ACCESSION") or "") or None,
                )
            )
        status = "verified" if raw_value is not None and app_value is not None and _close(app_value, raw_value) else "conflict"
        finding = {
            "id": f"annual_revenue_{year}",
            "field": "年度营收",
            "period": str(year),
            "status": status,
            "candidates": candidates,
            "selected": candidates[0],
            "reason": (
                "两条记录数值一致。"
                if status == "verified"
                else "Vibe 紧凑 10-K 行缺少 XBRL 起止日期，不能证明其为全年累计值；采用期间语义完整的 CANSLIM SEC 年度序列。"
            ),
        }
        findings.append(finding)
        canonical.append({"id": finding["id"], "field": finding["field"], "period": finding["period"], "value": app_value, "source": candidates[0]["source"]})

    quarter = _latest_context_quarter(application_context)
    if quarter:
        period = str(quarter["period"])
        app_value = _number(quarter.get("eps"))
        raw = _raw_quarter_row(quarterly_rows, period)
        raw_value = _number(raw.get(_EPS_KEY))
        candidates = [
            _candidate("CANSLIM SEC quarterly series", app_value, period=period, detail="period-aware quarterly series"),
        ]
        if raw:
            candidates.append(
                _candidate(
                    "Vibe SEC 10-Q row",
                    raw_value,
                    period=period,
                    detail="matching fiscal period and 10-Q form",
                    accession=str(raw.get("ACCESSION") or "") or None,
                )
            )
        if raw_value is None:
            status, reason = "unverified", "未找到同期间的 Vibe 10-Q 行，不能将该 EPS 作为跨源核验事实。"
        elif app_value is not None and _close(app_value, raw_value):
            status, reason = "verified", "CANSLIM SEC 季度序列与 Vibe SEC 10-Q 行数值一致。"
        else:
            status, reason = "conflict", "同一季度的 SEC 数值不一致；该字段必须保持未知，等待原始 filing 复核。"
        finding = {
            "id": f"diluted_eps_{period}",
            "field": "稀释每股收益",
            "period": period,
            "status": status,
            "candidates": candidates,
            "reason": reason,
        }
        findings.append(finding)
        if status == "verified":
            selected = candidates[0]
            finding["selected"] = selected
            canonical.append({"id": finding["id"], "field": finding["field"], "period": period, "value": app_value, "source": selected["source"]})
        elif status == "conflict":
            blocked.append({"id": finding["id"], "field": finding["field"], "period": period, "reason": reason})

    status = "clean" if findings and all(item["status"] == "verified" for item in findings) else (
        "issues_found" if findings else "not_available"
    )
    return {
        "schema_version": "vibe.data_audit.v1",
        "status": status,
        "scope": "financial periods with source and form metadata",
        "findings": findings,
        "canonical_facts": canonical,
        "blocked_claims": blocked,
        "rules": [
            "Use canonical_facts for audited numerical claims.",
            "Do not use a blocked_claim as a factual input.",
            "Treat fields outside this audit scope as unverified unless their own source metadata is shown.",
        ],
    }


def render_data_audit(audit: dict[str, Any]) -> str:
    lines = [
        "数据审计员已按字段、报告期、来源和原始表单比较可复算的财务数据；不能判定的数值不会被伪装成正确值。",
        "",
        "| 字段 | 期间 | 审计状态 | 采用值 | 依据 |",
        "|---|---|---|---:|---|",
    ]
    for finding in audit.get("findings", []):
        if not isinstance(finding, dict):
            continue
        selected = _as_dict(finding.get("selected"))
        value = selected.get("value") if selected else "未知"
        status = {"verified": "已核验", "conflict": "冲突", "unverified": "未核验"}.get(finding.get("status"), "未知")
        lines.append(f"| {finding.get('field', '-')} | {finding.get('period', '-')} | {status} | {value} | {finding.get('reason', '-')} |")
        if finding.get("status") == "conflict":
            for candidate in finding.get("candidates", []):
                if isinstance(candidate, dict):
                    lines.append(f"\n> 候选 `{candidate.get('source')}`：{candidate.get('value')}；{candidate.get('detail')}")
    if not audit.get("findings"):
        lines.extend(["| - | - | 未覆盖 | 未知 | 本次没有可审计的同期间财务来源。 |"])
    return "\n".join(lines)
