import asyncio
import ast
import copy
import hmac
import ipaddress
import json
import os
import re
import socket
import time
import traceback
import uuid
from queue import Empty, Queue
from datetime import date, datetime
from pathlib import Path
from typing import Any, AsyncIterator, Iterable
from urllib.parse import urlparse

import uvicorn
import requests
from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse
from langchain_core.callbacks import BaseCallbackHandler
from langchain_core.messages import HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, field_validator

from analysis_runs import (
    RunCapacityError,
    RunConflictError,
    analysis_run_registry,
    request_fingerprint,
    validate_run_id,
)
from data_auditor import audit_data, render_data_audit
from journal_reflection import build_journal_reflection
from broker_sync import discover_broker_account, sync_broker
from ai_runtime import (
    patched_tradingagents_graph,
    prefetch_vibe_evidence_async,
    runtime_status,
)
from vibe_decision_flow import (
    apply_execution_gate,
    build_research_pack,
    render_evidence_reconciliation,
    render_execution_validation,
    validate_execution_plan,
)

load_dotenv()

app = FastAPI(title="AlphaLens TradingAgents Sidecar")

# This is a protocol header name, not a password.
SIDECAR_TOKEN_HEADER = "X-Sidecar-Token"  # nosec B105
MAX_REQUEST_BODY_BYTES = int(os.getenv("SIDECAR_MAX_REQUEST_BODY_BYTES", str(4 * 1024 * 1024)))
MAX_MODEL_DISCOVERY_RESPONSE_BYTES = 2 * 1024 * 1024
MAX_CONCURRENT_REQUESTS = max(1, int(os.getenv("SIDECAR_MAX_CONCURRENT_REQUESTS", "16")))
_active_requests = 0
_active_requests_lock = asyncio.Lock()


def _env_enabled(name: str) -> bool:
    return os.getenv(name, "").strip().lower() in {"1", "true", "yes", "on"}


def validate_backend_url(value: str | None) -> str | None:
    raw = (value or "").strip()
    if not raw:
        return None
    parsed = urlparse(raw)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password:
        raise ValueError("backend_url must be an HTTPS URL without embedded credentials")
    if parsed.query or parsed.fragment:
        raise ValueError("backend_url must not include query parameters or fragments")

    host = parsed.hostname.rstrip(".").lower()
    allowed_hosts = {
        item.strip().rstrip(".").lower()
        for item in os.getenv("AI_ALLOWED_BACKEND_HOSTS", "ai-gateway.vercel.sh").split(",")
        if item.strip()
    }
    require_allowlist = (
        os.getenv("APP_ENV", "development").strip().lower() == "production"
        or _env_enabled("AI_REQUIRE_BACKEND_ALLOWLIST")
    )
    if require_allowlist and host not in allowed_hosts:
        raise ValueError("backend_url host is not in AI_ALLOWED_BACKEND_HOSTS")
    if not _env_enabled("AI_ALLOW_PRIVATE_BACKENDS"):
        try:
            literal_address = ipaddress.ip_address(host)
            addresses = [literal_address]
        except ValueError:
            try:
                addresses = {
                    ipaddress.ip_address(item[4][0])
                    for item in socket.getaddrinfo(host, None, type=socket.SOCK_STREAM)
                }
            except (socket.gaierror, ValueError) as exc:
                raise ValueError("backend_url host could not be resolved") from exc
        if any(
            address.is_private
            or address.is_loopback
            or address.is_link_local
            or address.is_reserved
            or address.is_unspecified
            or address.is_multicast
            for address in addresses
        ):
            raise ValueError("private backend_url addresses are disabled")
    return raw.rstrip("/")


@app.middleware("http")
async def protect_sidecar(request: Request, call_next):
    global _active_requests
    if request.url.path != "/health":
        expected = os.getenv("SIDECAR_SHARED_SECRET", "").strip()
        if os.getenv("APP_ENV", "development").strip().lower() == "production" and len(expected) < 32:
            return JSONResponse(status_code=503, content={"detail": "sidecar security is not configured"})
        if expected and not hmac.compare_digest(request.headers.get(SIDECAR_TOKEN_HEADER, ""), expected):
            return JSONResponse(status_code=401, content={"detail": "unauthorized"})

    content_length = request.headers.get("content-length")
    if content_length:
        try:
            if int(content_length) > MAX_REQUEST_BODY_BYTES:
                return JSONResponse(status_code=413, content={"detail": "request body too large"})
        except ValueError:
            return JSONResponse(status_code=400, content={"detail": "invalid content-length"})
    body = await request.body()
    if len(body) > MAX_REQUEST_BODY_BYTES:
        return JSONResponse(status_code=413, content={"detail": "request body too large"})
    if request.url.path == "/health":
        return await call_next(request)
    async with _active_requests_lock:
        if _active_requests >= MAX_CONCURRENT_REQUESTS:
            return JSONResponse(status_code=503, content={"detail": "sidecar is busy"})
        _active_requests += 1
    try:
        return await call_next(request)
    finally:
        async with _active_requests_lock:
            _active_requests -= 1


class StrictRequestModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class AnalysisRequest(StrictRequestModel):
    ticker: str = Field(..., min_length=1, max_length=32, pattern=r"^[A-Za-z0-9][A-Za-z0-9.^=_-]{0,31}$")
    llm_provider: str = Field(default="anthropic", min_length=1, max_length=64)
    deep_think_llm: str = Field(default="claude-sonnet-4-6", min_length=1, max_length=256)
    quick_think_llm: str = Field(default="claude-haiku-4-5-20251001", min_length=1, max_length=256)
    backend_url: str | None = Field(default=None, max_length=2048)
    max_debate_rounds: int = Field(default=1, ge=1, le=5)
    max_risk_rounds: int = Field(default=1, ge=1, le=5)
    online_tools: bool = True
    analysts: list[str] = Field(
        default_factory=lambda: ["fundamental", "sentiment", "news", "technical"],
        min_length=1,
        max_length=8,
    )
    analysis_context: dict[str, Any] = Field(default_factory=dict)
    run_id: str | None = Field(default=None, min_length=8, max_length=128)
    resume: bool = False
    last_event_id: int = Field(default=0, ge=0)
    api_key: str | None = Field(default=None, min_length=1, max_length=8192, repr=False)

    @field_validator("analysts")
    @classmethod
    def validate_analysts(cls, values: list[str]) -> list[str]:
        if any(not value.strip() or len(value) > 64 for value in values):
            raise ValueError("analyst IDs must be 1-64 characters")
        return values

    @field_validator("analysis_context")
    @classmethod
    def validate_analysis_context_size(cls, value: dict[str, Any]) -> dict[str, Any]:
        encoded = json.dumps(value, ensure_ascii=False, default=str).encode("utf-8")
        if len(encoded) > 1024 * 1024:
            raise ValueError("analysis_context exceeds 1 MiB")
        return value


class ModelDiscoveryRequest(StrictRequestModel):
    provider: str = Field(..., min_length=1, max_length=64)
    backend_url: str | None = Field(default=None, max_length=2048)
    api_key: str | None = Field(default=None, min_length=1, max_length=8192, repr=False)


class JournalReflectionRequest(StrictRequestModel):
    schema_version: str = "vibe.trade_record.v1"
    records: list[dict[str, Any]] = Field(default_factory=list, max_length=5000)


class BrokerSyncRequest(StrictRequestModel):
    broker: str = Field(..., min_length=1, max_length=32)
    credentials: dict[str, str] = Field(default_factory=dict)
    start_at: datetime
    end_at: datetime


class BrokerGatewayDiscoveryRequest(StrictRequestModel):
    broker: str = Field(..., min_length=1, max_length=32)
    credentials: dict[str, str] = Field(default_factory=dict)


ANALYST_MAP = {
    "fundamental": ("fundamentals", "FundamentalsAnalyst", "正在分析基本面数据..."),
    "fundamentals": ("fundamentals", "FundamentalsAnalyst", "正在分析基本面数据..."),
    "sentiment": ("social", "SentimentAnalyst", "正在分析情绪与社交媒体数据..."),
    "social": ("social", "SentimentAnalyst", "正在分析情绪与社交媒体数据..."),
    "news": ("news", "NewsAnalyst", "正在分析新闻与宏观事件..."),
    "technical": ("market", "TechnicalAnalyst", "正在分析技术指标与价格行为..."),
    "market": ("market", "TechnicalAnalyst", "正在分析技术指标与价格行为..."),
}

REPORT_FIELDS = [
    ("fundamentals_report", "FundamentalsAnalyst"),
    ("sentiment_report", "SentimentAnalyst"),
    ("news_report", "NewsAnalyst"),
    ("market_report", "TechnicalAnalyst"),
    ("investment_debate_state.judge_decision", "ResearchTeam"),
    ("trader_investment_plan", "Trader"),
    ("risk_debate_state.judge_decision", "RiskManagement"),
    ("final_trade_decision", "PortfolioManager"),
]

AGENT_DISPLAY_NAMES = {
    "VibeEvidence": "Vibe 数据证据",
    "DataAuditor": "数据审计员",
    "VibeEvidenceReconciler": "Vibe 证据对账",
    "VibeExecutionValidation": "Vibe 执行验证",
    "FundamentalsAnalyst": "基本面分析师",
    "SentimentAnalyst": "情绪分析师",
    "NewsAnalyst": "新闻分析师",
    "TechnicalAnalyst": "技术分析师",
    "CANSLIMSecondLayerAnalyst": "CANSLIM 二层筛选官",
    "ResearchTeam": "研究团队",
    "Trader": "交易员",
    "RiskManagement": "风险管理团队",
    "PortfolioManager": "投资组合经理",
}

CUSTOM_ANALYST_MAP = {
    "canslim_second_layer": {
        "agent": "CANSLIMSecondLayerAnalyst",
        "message": "正在进行 CANSLIM 第二层严格筛选...",
    },
    "canslimsecondlayeranalyst": {
        "agent": "CANSLIMSecondLayerAnalyst",
        "message": "正在进行 CANSLIM 第二层严格筛选...",
    },
}

CANSLIM_SECOND_LAYER_SYSTEM_PROMPT = """你是一名严格的 CANSLIM 成长股交易系统分析师。
你的任务不是为股票找理由看多，而是对已经通过第一层 CANSLIM 初筛的股票做第二层筛选，重点识别假成长、低质量盈利、流动性陷阱、技术形态缺陷、估值透支和风险收益比不合格的问题。

硬性规则：
1. 只基于用户提供的 JSON 数据分析；没有出现在 JSON 中、为空、为 null 或数据不足的字段，一律标记为“未知”，不得编造。
2. 不要因为它已经符合 CANSLIM 初筛就默认看多。
3. 必须区分“公司质量”和“当前是否适合交易”。
4. 最后必须给出明确分级：A级重点候选、B级观察候选、C级弱候选、D级淘汰。
5. 所有分数都要给出具体数字，满分结构必须合计 100 分。
6. JSON 中的 provided_data.second_layer_data 来自免费数据源增强，通常包含市值、股本、流通股、做空、机构/内部人持仓比例、估值、财报日、季度收入、利润率、现金流、应收、库存、债务和股本变化；其中 second_layer_data.sec.insider_transactions 与 second_layer_data.sec.recent_event_filings 来自 SEC EDGAR，可用于内部人交易和事件风险判断。provided_data.derived 是应用基于原始数据预先计算的二层筛选摘要。二者可优先用于价格、均线、成交额、支撑压力、RS 新高、盈利质量、估值、供需和事件判断；但仍不得外推 JSON 之外的数据。

请严格按照以下结构输出：

一、硬性淘汰检查
逐项判断是否存在：流动性不足、买卖价差或成交额风险、EPS 增长失真、收入增长不足、一次性收益、现金流质量差、股本持续稀释、频繁增发或融资风险、技术形态过度延伸、财报或重大事件风险过高。
结论：是否触发硬性淘汰？如果触发，请直接说明原因。

二、盈利质量分析
判断 EPS 增长是否由收入增长支撑、利润率是否改善、经营现金流是否支持净利润、是否存在应收账款/库存/费用资本化/一次性收益问题、股本是否被稀释。给出 0-20 分评分并解释。

三、增长可持续性分析
判断增长来自长期驱动还是短期因素、指引是否上修、行业是否上行、订单/客户数/backlog/ARR 或其他领先指标是否改善、分析师预期是否上修、未来 2-4 个季度增长是否可能继续。给出 0-15 分评分并解释。

四、行业领导力分析
判断是否是细分行业真正领导股、相对强度是否领先同行、股价是否率先创新高、基本面增速是否优于同行、市场回调时是否更抗跌、上涨时是否比同行更主动。给出 0-15 分评分并解释。

五、供需结构分析
判断 20 日和 50 日平均美元成交额是否足够、流通盘是否过小或过大、是否存在增发/ATM/可转债/认股权证等抛压、机构持仓是否处于合理阶段、内部人是否异常卖出、做空比例是否构成风险或潜在轧空因素。给出 0-15 分评分并解释。

六、技术形态分析
判断当前是否处于可交易买点、是否过度远离 50 日均线、是否有健康整理结构、突破是否放量、回调是否缩量、RS 线是否领先创新高、是否存在假突破风险、跌破哪个位置说明交易假设失效。给出 0-20 分评分并解释。

七、估值与预期分析
判断估值是否合理、是否显著高于同行、上涨主要来自业绩增长还是估值扩张、市场预期是否过满、未来是否还有超预期空间。给出 0-10 分评分并解释。

八、事件风险与风控分析
判断是否临近财报或重大事件，是否有监管、诉讼、融资、解禁、债务、并购等风险；给出合理入场位、止损位、第一目标价或潜在上涨空间；判断风险收益比是否至少 2:1，最好 3:1；说明是否降低单票风险敞口。给出 0-5 分评分并解释。

九、总评分与分级
汇总：盈利质量 /20，增长可持续性 /15，行业领导力 /15，供需结构 /15，技术形态 /20，估值与预期 /10，事件与风控 /5，总分 /100。
分级标准：85-100 为 A级重点候选；75-84 为 B级观察候选；60-74 为 C级弱候选；60 以下为 D级淘汰。

十、最终结论
回答：1. 真成长还是疑似假成长；2. 行业领导股还是跟随股；3. 当前是可交易买点、观察点，还是应该回避；4. 最大三个风险；5. 最吸引人的三个优点；6. 如果交易，合理入场价、止损价、目标价和风险收益比；7. 如果暂不交易，需要等待什么信号。
最后必须给出一句话结论：“这只股票目前属于：A级 / B级 / C级 / D级，原因是……”
"""

REPORT_DELTA_CHARS = int(os.getenv("AI_REPORT_DELTA_CHARS", "120"))
REPORT_DELTA_DELAY = float(os.getenv("AI_REPORT_DELTA_DELAY", "0.006"))
NARRATION_INTERVAL_SECONDS = float(os.getenv("AI_NARRATION_INTERVAL_SECONDS", "2.5"))

OUTPUT_LANGUAGE_INSTRUCTION = (
    "Simplified Chinese. Do not output English section titles, internal reasoning, "
    "tool-call markup, raw JSON schemas, or <think> blocks"
)

CONTROL_TOKEN_RE = re.compile(r"<[｜|][^>\n]{0,120}[｜|]>")
THINK_BLOCK_RE = re.compile(r"<think\b[^>]*>.*?</think>", re.IGNORECASE | re.DOTALL)
TRANSACTION_PROPOSAL_RE = re.compile(
    r"(?im)^\s*(?:F?INAL|INAL)\s+TRANSACTION\s+PROPOSAL\s*:\s*(?:\*\*)?([A-Z/]+)(?:\*\*)?\s*$"
)
MARKDOWN_FENCE_ONLY_RE = re.compile(r"(?m)^\s*```+\s*$")
RAW_TOOL_FUNCTION_RE = re.compile(
    r"(?i)\b(?:function\s*)?get_"
    r"(?:news|global_news|macro_indicators|prediction_markets|fundamentals|"
    r"balance_sheet|cashflow|income_statement|stock_data|indicators|"
    r"verified_market_snapshot|insider_transactions)\b"
)
TOOL_ARG_KEYS = frozenset(
    {
        "ticker",
        "symbol",
        "query",
        "topic",
        "indicator",
        "curr_date",
        "current_date",
        "start_date",
        "end_date",
        "look_back_days",
        "limit",
        "period",
        "interval",
        "market",
        "asset_type",
    }
)
TOOL_WRAPPER_KEYS = frozenset({"arguments", "args", "function", "name", "tool", "tool_name", "type"})

DISPLAY_LABEL_REPLACEMENTS = [
    (re.compile(r"\*\*Overall Sentiment:\*\*", re.IGNORECASE), "**综合情绪**："),
    (re.compile(r"\*\*Confidence:\*\*", re.IGNORECASE), "**置信度**："),
    (re.compile(r"\*\*overall_band:\*\*", re.IGNORECASE), "**综合情绪**："),
    (re.compile(r"\*\*overall_score:\*\*", re.IGNORECASE), "**情绪分数**："),
    (re.compile(r"\*\*confidence:\*\*", re.IGNORECASE), "**置信度**："),
    (re.compile(r"\*\*narrative:\*\*", re.IGNORECASE), "**分析正文**："),
    (re.compile(r"\*\*Action\*\*\s*:", re.IGNORECASE), "**操作建议**："),
    (re.compile(r"\*\*Reasoning\*\*\s*:", re.IGNORECASE), "**理由**："),
    (re.compile(r"\*\*Entry Price\*\*\s*:", re.IGNORECASE), "**入场价**："),
    (re.compile(r"\*\*Stop Loss\*\*\s*:", re.IGNORECASE), "**止损价**："),
    (re.compile(r"\*\*Position Sizing\*\*\s*:", re.IGNORECASE), "**仓位建议**："),
    (re.compile(r"\*\*Rating\*\*\s*:", re.IGNORECASE), "**评级**："),
    (re.compile(r"\*\*Executive Summary\*\*\s*:", re.IGNORECASE), "**执行摘要**："),
    (re.compile(r"\*\*Investment Thesis\*\*\s*:", re.IGNORECASE), "**投资逻辑**："),
    (re.compile(r"\*\*Price Target\*\*\s*:", re.IGNORECASE), "**目标价**："),
    (re.compile(r"\*\*Time Horizon\*\*\s*:", re.IGNORECASE), "**时间周期**："),
    (re.compile(r"\bScore:\s*", re.IGNORECASE), "分数："),
]

DISPLAY_VALUE_REPLACEMENTS = {
    "Strong Buy": "强烈买入",
    "Strong Sell": "强烈卖出",
    "Overweight": "增持",
    "Underweight": "减持",
    "BUY": "买入",
    "SELL": "卖出",
    "HOLD": "持有/观望",
    "Buy": "买入",
    "Sell": "卖出",
    "Hold": "持有/观望",
    "Strongly Bullish": "强烈看涨",
    "Mildly Bullish": "温和看涨",
    "Bullish": "看涨",
    "Neutral": "中性",
    "Mixed": "分歧",
    "Mildly Bearish": "温和看跌",
    "Strongly Bearish": "强烈看跌",
    "Bearish": "看跌",
    "High": "高",
    "Medium": "中等",
    "Low": "低",
    "high": "高",
    "medium": "中等",
    "low": "低",
}


def sse(event: str, data: dict[str, Any]) -> str:
    payload = json.dumps(data, ensure_ascii=False)
    return f"event: {event}\ndata: {payload}\n\n"


def text_chunks(value: str, chunk_size: int) -> Iterable[str]:
    size = max(1, chunk_size)
    for index in range(0, len(value), size):
        yield value[index : index + size]


def translate_display_value(value: Any) -> str:
    return DISPLAY_VALUE_REPLACEMENTS.get(str(value), str(value))


def render_known_structured_payload(payload: dict[str, Any]) -> str | None:
    if {"overall_band", "overall_score", "confidence", "narrative"}.issubset(payload):
        score = payload.get("overall_score")
        try:
            score_text = f"{float(score):.1f}/10"
        except (TypeError, ValueError):
            score_text = str(score)
        return "\n".join(
            [
                f"**综合情绪**：**{translate_display_value(payload.get('overall_band'))}**（分数：{score_text}）",
                f"**置信度**：{translate_display_value(payload.get('confidence'))}",
                "",
                str(payload.get("narrative") or "").strip(),
            ]
        ).strip()

    if {"action", "reasoning"}.issubset(payload):
        parts = [
            f"**操作建议**：{payload.get('action')}",
            "",
            f"**理由**：{payload.get('reasoning')}",
        ]
        if payload.get("entry_price") is not None:
            parts.extend(["", f"**入场价**：{payload.get('entry_price')}"])
        if payload.get("stop_loss") is not None:
            parts.extend(["", f"**止损价**：{payload.get('stop_loss')}"])
        if payload.get("position_sizing"):
            parts.extend(["", f"**仓位建议**：{payload.get('position_sizing')}"])
        return "\n".join(parts).strip()

    if {"rating", "executive_summary", "investment_thesis"}.issubset(payload):
        parts = [
            f"**评级**：{payload.get('rating')}",
            "",
            f"**执行摘要**：{payload.get('executive_summary')}",
            "",
            f"**投资逻辑**：{payload.get('investment_thesis')}",
        ]
        if payload.get("price_target") is not None:
            parts.extend(["", f"**目标价**：{payload.get('price_target')}"])
        if payload.get("time_horizon"):
            parts.extend(["", f"**时间周期**：{payload.get('time_horizon')}"])
        return "\n".join(parts).strip()

    return None


def normalize_structured_json_leaks(text: str) -> str:
    decoder = json.JSONDecoder()
    index = 0
    while index < len(text):
        start = text.find("{", index)
        if start == -1:
            return text
        try:
            payload, end = decoder.raw_decode(text[start:])
        except json.JSONDecodeError:
            index = start + 1
            continue

        rendered = render_known_structured_payload(payload) if isinstance(payload, dict) else None
        if not rendered:
            index = start + 1
            continue

        prefix = text[:start].strip()
        suffix = text[start + end :].strip()
        prefix = TRANSACTION_PROPOSAL_RE.sub("", prefix).replace("∝⇒﹏", "").strip()

        if suffix:
            parts = [part for part in [prefix, suffix] if part]
            return "\n\n".join(parts).strip()
        parts = [part for part in [prefix, rendered] if part]
        return "\n\n".join(parts).strip()
    return text


def remove_internal_planning(text: str) -> str:
    hard_markers = [
        "首先，我需要收集",
        "现在依次调用工具",
        "正在调用工具",
        "### Step",
    ]
    for marker in hard_markers:
        index = text.find(marker)
        if index != -1:
            return text[:index].rstrip()

    markers = [
        "Let's start by calling",
        "We have the following tools",
        "Our task is",
    ]
    tool_terms = ("get_news", "get_global_news", "get_macro_indicators", "get_prediction_markets", "tool")
    cut_at: int | None = None
    for marker in markers:
        index = text.find(marker)
        if index == -1:
            continue
        tail = text[index:]
        if any(term in tail for term in tool_terms):
            cut_at = index if cut_at is None else min(cut_at, index)
    if cut_at is None:
        return text
    return text[:cut_at].rstrip()


def maybe_parse_json_object(value: str) -> Any | None:
    text = value.strip()
    if not text.startswith("{") or not text.endswith("}"):
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return None


def payload_has_tool_arguments(payload: Any) -> bool:
    if not isinstance(payload, dict) or not payload:
        return False
    keys = {str(key) for key in payload.keys()}
    if keys & TOOL_ARG_KEYS:
        return keys <= TOOL_ARG_KEYS | TOOL_WRAPPER_KEYS

    for nested_key in ("arguments", "args"):
        nested = payload.get(nested_key)
        if isinstance(nested, str):
            nested = maybe_parse_json_object(nested) or nested
        if payload_has_tool_arguments(nested):
            return True

    function_payload = payload.get("function")
    if isinstance(function_payload, dict):
        name = str(function_payload.get("name") or "")
        arguments = function_payload.get("arguments")
        if RAW_TOOL_FUNCTION_RE.search(name) or payload_has_tool_arguments(arguments):
            return True

    return False


def line_has_tool_argument_json(line: str) -> bool:
    payload = maybe_parse_json_object(line)
    return payload_has_tool_arguments(payload)


def is_markdown_fence(line: str) -> bool:
    return line.strip().startswith("```")


def probable_tool_fragment_label(line: str) -> bool:
    stripped = line.strip()
    if not stripped or len(stripped) > 40:
        return False
    if RAW_TOOL_FUNCTION_RE.search(stripped):
        return True
    return bool(re.fullmatch(r"[A-Za-z_][A-Za-z0-9_ -]{0,39}", stripped))


def next_nonempty_line(lines: list[str], start: int) -> tuple[int, str] | None:
    for index in range(start, len(lines)):
        stripped = lines[index].strip()
        if stripped:
            return index, stripped
    return None


def next_line_starts_tool_payload(lines: list[str], start: int) -> bool:
    next_line = next_nonempty_line(lines, start)
    if not next_line:
        return False
    index, stripped = next_line
    if line_has_tool_argument_json(stripped) or RAW_TOOL_FUNCTION_RE.search(stripped):
        return True
    if is_markdown_fence(stripped):
        after_fence = next_nonempty_line(lines, index + 1)
        return bool(after_fence and line_has_tool_argument_json(after_fence[1]))
    return False


def skip_tool_payload_lines(lines: list[str], start: int) -> int:
    index = start
    while index < len(lines):
        stripped = lines[index].strip()
        if not stripped or is_markdown_fence(stripped):
            index += 1
            continue
        if line_has_tool_argument_json(stripped) or RAW_TOOL_FUNCTION_RE.search(stripped):
            index += 1
            continue
        if probable_tool_fragment_label(stripped) and next_line_starts_tool_payload(lines, index + 1):
            index += 1
            continue
        break
    return index


def remove_raw_tool_call_blocks(text: str) -> str:
    lines = text.splitlines()
    kept: list[str] = []
    index = 0

    while index < len(lines):
        stripped = lines[index].strip()
        starts_tool_block = (
            RAW_TOOL_FUNCTION_RE.search(stripped)
            or "正在调用工具" in stripped
            or "tool_calls" in stripped.lower()
            or "function_call" in stripped.lower()
        )
        if starts_tool_block:
            index = skip_tool_payload_lines(lines, index + 1)
            continue

        if probable_tool_fragment_label(stripped) and next_line_starts_tool_payload(lines, index + 1):
            index += 1
            continue

        if is_markdown_fence(stripped) and next_line_starts_tool_payload(lines, index + 1):
            index = skip_tool_payload_lines(lines, index + 1)
            continue

        if line_has_tool_argument_json(stripped):
            while kept and not kept[-1].strip():
                kept.pop()
            if kept and probable_tool_fragment_label(kept[-1]):
                kept.pop()
            index += 1
            continue

        kept.append(lines[index])
        index += 1

    return "\n".join(kept).strip()


def cleanup_display_labels(text: str) -> str:
    cleaned = TRANSACTION_PROPOSAL_RE.sub(lambda match: f"**最终交易建议**：{translate_display_value(match.group(1))}", text)
    for pattern, replacement in DISPLAY_LABEL_REPLACEMENTS:
        cleaned = pattern.sub(replacement, cleaned)
    for source, target in DISPLAY_VALUE_REPLACEMENTS.items():
        cleaned = re.sub(rf"\b{re.escape(source)}\s*\([^)]*\)", target, cleaned)
        cleaned = re.sub(rf"\b{re.escape(source)}\b", target, cleaned)
    cleaned = re.sub(r"(?m)^\s*MMARY\s*$", "", cleaned)
    return cleaned


def sanitize_display_text(value: Any) -> str:
    text = str(value or "").replace("\r\n", "\n").replace("\r", "\n")
    text = THINK_BLOCK_RE.sub("", text)
    think_index = text.lower().find("<think")
    if think_index != -1:
        text = text[:think_index]
    text = CONTROL_TOKEN_RE.sub("", text)
    text = text.replace("∝⇒﹏", "")
    text = MARKDOWN_FENCE_ONLY_RE.sub("", text)
    text = normalize_structured_json_leaks(text)
    text = remove_raw_tool_call_blocks(text)
    text = remove_internal_planning(text)
    text = cleanup_display_labels(text)
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def is_internal_live_output(text: str) -> bool:
    if not text.strip():
        return True
    lowered = text.lower()
    return (
        "we have the following tools" in lowered
        or "let's start by calling" in lowered
        or "our task is to" in lowered
        or "tool_calls" in lowered
        or "function_call" in lowered
        or RAW_TOOL_FUNCTION_RE.search(text) is not None
        or "现在依次调用工具" in text
        or "正在调用工具" in text
    )


async def stream_delta(
    event: str,
    content: str,
    metadata: dict[str, Any] | None = None,
    *,
    chunk_size: int = 1,
    delay: float = 0.0,
) -> AsyncIterator[str]:
    base = metadata or {}
    for chunk in text_chunks(content, chunk_size):
        yield sse(event, {**base, "content": chunk})
        if delay > 0:
            await asyncio.sleep(delay)


def content_to_text(content: Any) -> str:
    if isinstance(content, str):
        return sanitize_display_text(content)
    if isinstance(content, list):
        texts: list[str] = []
        for item in content:
            if isinstance(item, str):
                texts.append(item)
            elif isinstance(item, dict):
                item_type = str(item.get("type") or "").lower()
                if "tool" in item_type or "function" in item_type:
                    continue
                texts.append(str(item.get("text") or item.get("content") or ""))
        return sanitize_display_text("\n".join(text for text in texts if text))
    return sanitize_display_text(content)


def message_has_tool_calls(message: Any) -> bool:
    for attr in ("tool_calls", "invalid_tool_calls"):
        if getattr(message, attr, None):
            return True
    for attr in ("additional_kwargs", "response_metadata"):
        payload = getattr(message, attr, None)
        if isinstance(payload, dict):
            if payload.get("tool_calls") or payload.get("function_call"):
                return True
    return False


def response_to_text(response: Any) -> str:
    chunks: list[str] = []
    for generation_group in getattr(response, "generations", []) or []:
        if not generation_group:
            continue
        generation = generation_group[0]
        message = getattr(generation, "message", None)
        if message is not None:
            if message_has_tool_calls(message):
                continue
            text = content_to_text(getattr(message, "content", ""))
        else:
            text = content_to_text(getattr(generation, "text", ""))
        if text.strip():
            chunks.append(text.strip())
    return "\n\n".join(chunks)


class StreamingCallback(BaseCallbackHandler):
    def __init__(self, output_queue: Queue[str]) -> None:
        self.output_queue = output_queue

    def on_llm_end(self, response: Any, **_: Any) -> None:
        text = response_to_text(response)
        if text:
            self.output_queue.put(text)


async def drain_model_output(output_queue: Queue[str]) -> AsyncIterator[str]:
    while True:
        try:
            content = output_queue.get_nowait()
        except Empty:
            return
        content = sanitize_display_text(content)
        if is_internal_live_output(content):
            continue
        async for event in stream_delta(
            "narration_delta",
            "\n\n" + content + "\n",
            {"agent": "Model"},
            chunk_size=REPORT_DELTA_CHARS,
            delay=REPORT_DELTA_DELAY,
        ):
            yield event


def selected_builtin_analysts(request: AnalysisRequest) -> tuple[str, ...]:
    selected: list[str] = []
    has_custom = False
    for raw in request.analysts:
        key = raw.strip().lower()
        mapped = ANALYST_MAP.get(key)
        if mapped and mapped[0] not in selected:
            selected.append(mapped[0])
        if key in CUSTOM_ANALYST_MAP:
            has_custom = True
    if selected:
        return tuple(selected)
    if has_custom:
        return tuple()
    return ("fundamentals", "social", "news", "market")


def selected_custom_analysts(request: AnalysisRequest) -> tuple[str, ...]:
    selected: list[str] = []
    for raw in request.analysts:
        key = raw.strip().lower()
        role = CUSTOM_ANALYST_MAP.get(key)
        if role and role["agent"] not in selected:
            selected.append(role["agent"])
    return tuple(selected)


def configured_graph(
    request: AnalysisRequest,
    analysts: tuple[str, ...],
    callbacks: list[Any] | None = None,
):
    from tradingagents.default_config import DEFAULT_CONFIG
    from tradingagents.graph.trading_graph import TradingAgentsGraph

    config = copy.deepcopy(DEFAULT_CONFIG)
    config["llm_provider"] = request.llm_provider
    config["deep_think_llm"] = request.deep_think_llm
    config["quick_think_llm"] = request.quick_think_llm
    config["backend_url"] = validate_backend_url(request.backend_url)
    if request.api_key:
        config["api_key"] = request.api_key
    config["max_debate_rounds"] = request.max_debate_rounds
    config["max_risk_discuss_rounds"] = request.max_risk_rounds
    config["max_risk_rounds"] = request.max_risk_rounds
    config["online_tools"] = request.online_tools
    config["output_language"] = OUTPUT_LANGUAGE_INSTRUCTION
    configure_analysis_checkpoint(config, request)

    try:
        with patched_tradingagents_graph():
            graph = TradingAgentsGraph(
                selected_analysts=analysts,
                debug=False,
                config=config,
                callbacks=callbacks,
            )
    finally:
        # The LLM clients have already copied the key into their SDK-specific
        # secret fields. Remove it from TradingAgents' shared config so data
        # tools, checkpoints, and later runs cannot observe it.
        config.pop("api_key", None)
    attach_application_context(graph, request)
    return graph


def configure_analysis_checkpoint(config: dict[str, Any], request: AnalysisRequest) -> None:
    config["checkpoint_enabled"] = True
    run_id = validate_run_id(request.run_id or f"legacy-{uuid.uuid4()}")
    checkpoint_root = Path(
        os.getenv("AI_ANALYSIS_CHECKPOINT_DIR", str(Path(__file__).resolve().parent / "data" / "ai-runs"))
    ).expanduser()
    prune_checkpoint_directories(checkpoint_root, keep_run_id=run_id)
    config["data_cache_dir"] = str((checkpoint_root / run_id).resolve())


def prune_checkpoint_directories(checkpoint_root: Path, *, keep_run_id: str) -> None:
    try:
        retention_hours = max(1, int(os.getenv("AI_CHECKPOINT_RETENTION_HOURS", "24")))
    except ValueError:
        retention_hours = 24
    cutoff = time.time() - retention_hours * 60 * 60
    try:
        children = list(checkpoint_root.iterdir())
    except FileNotFoundError:
        return
    except OSError:
        return
    for child in children:
        if child.is_symlink() or not child.is_dir() or child.name == keep_run_id:
            continue
        try:
            if child.stat().st_mtime >= cutoff:
                continue
            for nested in sorted(child.rglob("*"), reverse=True):
                if nested.is_file() or nested.is_symlink():
                    nested.unlink(missing_ok=True)
                elif nested.is_dir():
                    nested.rmdir()
            child.rmdir()
        except OSError:
            continue


def attach_application_context(graph: Any, request: AnalysisRequest) -> None:
    """Inject Go/frontend structured data into every TradingAgents role."""
    if not request.analysis_context:
        return

    original_resolver = graph.resolve_instrument_context
    context_json = compact_context_json(request)
    instruction = (
        "\n\nCANSLIMAgent application-provided structured context follows. "
        "Treat it as point-in-time user/application evidence, preserve unknown values, "
        "and do not invent missing fields. The trade_journal object, when present, "
        "uses vibe.trade_record.v1 fills and describes the user's own executions; "
        "it is behavioral context, not current market data. When vibe_research_pack is present, "
        "use only complete/partial successful coverage as factual support, identify the source "
        "for material claims, explicitly name contrary evidence, and keep unavailable evidence unknown. "
        "When data_audit is present, use only its canonical_facts for audited numerical claims; "
        "never state blocked_claims as facts and disclose unresolved figures as unknown.\n"
        + context_json
    )

    def resolve_with_application_context(ticker: str, asset_type: str = "stock") -> str:
        return original_resolver(ticker, asset_type) + instruction

    graph.resolve_instrument_context = resolve_with_application_context


def run_analysis(request: AnalysisRequest, callbacks: list[Any] | None = None) -> tuple[dict[str, Any], Any]:
    analysts = selected_builtin_analysts(request)
    if not analysts:
        return {}, None
    graph = configured_graph(request, analysts=analysts, callbacks=callbacks)
    state, decision = graph.propagate(request.ticker.strip().upper(), date.today().isoformat())
    return state, decision


def get_nested(data: dict[str, Any], dotted_path: str) -> Any:
    current: Any = data
    for part in dotted_path.split("."):
        if not isinstance(current, dict):
            return None
        current = current.get(part)
    return current


def iter_reports(state: dict[str, Any]) -> Iterable[tuple[str, str]]:
    for field, agent in REPORT_FIELDS:
        value = get_nested(state, field)
        if value:
            content = sanitize_display_text(value)
            if content:
                yield agent, content


def report_heading(agent: str) -> str:
    return AGENT_DISPLAY_NAMES.get(agent, agent)


def compose_report(state: dict[str, Any]) -> str:
    sections: list[str] = []
    for agent, content in iter_reports(state):
        normalized = content.strip()
        if normalized:
            sections.append(f"## {report_heading(agent)}\n\n{normalized}")
    return "\n\n".join(sections)


def compose_custom_reports(reports: list[tuple[str, str]]) -> str:
    sections: list[str] = []
    for agent, content in reports:
        normalized = sanitize_display_text(content)
        if normalized:
            sections.append(f"## {report_heading(agent)}\n\n{normalized}")
    return "\n\n".join(sections)


def render_vibe_evidence(bundle: dict[str, Any]) -> str:
    """Render auditable MCP source traces without exposing bulky raw payloads."""
    summary = bundle.get("summary") if isinstance(bundle.get("summary"), dict) else {}
    traces = bundle.get("source_traces") if isinstance(bundle.get("source_traces"), list) else []
    lines = [
        "Vibe-Trading 已在 TradingAgents 分析前执行数据预取，以下调用结果也已注入各分析角色的结构化上下文。",
        "",
        f"- 状态：**{bundle.get('status', 'unknown')}**",
        f"- 标的：`{bundle.get('market_symbol') or bundle.get('symbol') or '-'}`",
        f"- 工具调用：{summary.get('ok', 0)}/{summary.get('requested', len(traces))} 成功",
        "",
        "| Vibe 工具 | 数据角色 | 状态 | 实际来源 | 耗时 |",
        "|---|---|---|---|---:|",
    ]
    for raw_trace in traces:
        if not isinstance(raw_trace, dict):
            continue
        status = str(raw_trace.get("status") or "unknown")
        status_text = "成功" if status == "ok" else "失败/降级"
        source = str(raw_trace.get("source") or "vibe-trading").replace("|", "\\|")
        tool = str(raw_trace.get("tool") or "unknown").replace("|", "\\|")
        role = str(raw_trace.get("role") or "-").replace("|", "\\|")
        duration = raw_trace.get("duration_ms")
        lines.append(f"| `{tool}` | {role} | {status_text} | `{source}` | {duration or 0} ms |")
        if raw_trace.get("error"):
            error = str(raw_trace["error"]).replace("\n", " ")[:500]
            lines.append(f"\n> `{tool}` 降级原因：{error}")
    if not traces:
        lines.extend(["", "> 本次没有请求 Vibe 工具；TradingAgents 将仅使用应用上下文和自身数据源。"])
    lines.extend(
        [
            "",
            "TradingAgents 自带的 verified snapshot 仍会独立运行，用于与上述 Vibe 证据交叉核验；单个 Vibe 工具失败不会中断整份报告。",
        ]
    )
    return "\n".join(lines).strip()


async def attach_vibe_evidence(request: AnalysisRequest) -> dict[str, Any]:
    """Execute Vibe MCP calls and copy their compact evidence into the request."""
    if request.online_tools:
        roles: tuple[str, ...] | None = selected_builtin_analysts(request)
        if not roles and selected_custom_analysts(request):
            roles = None
        bundle = await prefetch_vibe_evidence_async(
            request.ticker,
            analysis_date=date.today().isoformat(),
            analysts=roles,
        )
    else:
        bundle = {
            "schema_version": "vibe.research_evidence.v1",
            "provider": "vibe-trading",
            "symbol": request.ticker.strip().upper(),
            "status": "disabled",
            "summary": {"requested": 0, "ok": 0, "failed": 0},
            "evidence": {},
            "source_traces": [],
        }
    request.analysis_context = dict(request.analysis_context or {})
    request.analysis_context["vibe_trading"] = bundle
    request.analysis_context["vibe_research_pack"] = build_research_pack(bundle)
    request.analysis_context["data_audit"] = audit_data(bundle, request.analysis_context)
    return bundle


def compact_context_json(request: AnalysisRequest) -> str:
    provided_data = copy.deepcopy(request.analysis_context or {})
    # The Vibe compact financial rows can include comparative values without an
    # XBRL duration.  Do not leave those raw values beside the audit contract:
    # a model could otherwise quote a rejected candidate as a fact.
    vibe_bundle = provided_data.get("vibe_trading")
    if isinstance(vibe_bundle, dict):
        traces = vibe_bundle.get("source_traces")
        if isinstance(traces, list):
            for trace in traces:
                if not isinstance(trace, dict):
                    continue
                if trace.get("evidence_key") in {
                    "stock_profile",
                    "financial_indicators_annual",
                    "financial_indicators_quarterly",
                }:
                    trace["payload"] = {
                        "redacted": "Financial values are governed by data_audit.canonical_facts and blocked_claims."
                    }
    payload = {
        "ticker": request.ticker.strip().upper(),
        "analysis_date": date.today().isoformat(),
        "provided_data": provided_data,
    }
    return json.dumps(payload, ensure_ascii=False, indent=2, default=str)


def create_role_llm(request: AnalysisRequest, callbacks: list[Any] | None = None):
    from tradingagents.llm_clients.factory import create_llm_client

    kwargs: dict[str, Any] = {}
    if callbacks:
        kwargs["callbacks"] = callbacks
    if request.api_key:
        kwargs["api_key"] = request.api_key
    model = request.deep_think_llm or request.quick_think_llm
    client = create_llm_client(
        provider=request.llm_provider,
        model=model,
        base_url=validate_backend_url(request.backend_url),
        **kwargs,
    )
    return client.get_llm()


def run_canslim_second_layer_analysis(
    request: AnalysisRequest,
    callbacks: list[Any] | None = None,
) -> str:
    llm = create_role_llm(request, callbacks=callbacks)
    user_prompt = (
        "下面 JSON 是你唯一可以使用的数据源。"
        "如果某个字段不存在、为空、为 null、状态为 error/insufficient，输出时必须标记为“未知”。\n\n"
        + compact_context_json(request)
    )
    response = llm.invoke(
        [
            SystemMessage(content=CANSLIM_SECOND_LAYER_SYSTEM_PROMPT),
            HumanMessage(content=user_prompt),
        ]
    )
    return sanitize_display_text(content_to_text(getattr(response, "content", response))).strip()


def run_custom_analysis(
    request: AnalysisRequest,
    agent: str,
    callbacks: list[Any] | None = None,
) -> str:
    if agent == "CANSLIMSecondLayerAnalyst":
        return run_canslim_second_layer_analysis(request, callbacks=callbacks)
    raise ValueError(f"unknown custom analyst: {agent}")


def normalize_action(decision: Any) -> str:
    value = str(decision or "").strip().lower()
    if "sell" in value or "underweight" in value:
        return "SELL"
    if "buy" in value or "overweight" in value:
        return "BUY"
    return "HOLD"


def confidence_for_action(action: str, decision: Any) -> str:
    value = str(decision or "").strip().lower()
    if action in {"BUY", "SELL"} and ("buy" in value or "sell" in value):
        return "HIGH"
    if "hold" in value:
        return "MEDIUM"
    return "LOW"


GRADE_LABELS = [
    ("A级", "A级重点候选"),
    ("B级", "B级观察候选"),
    ("C级", "C级弱候选"),
    ("D级", "D级淘汰"),
]
GRADE_LABEL_BY_SHORT = dict(GRADE_LABELS)
GRADE_MATCH_RE = re.compile(r"([ABCD])\s*级")
GRADE_SEGMENT_SPLIT_RE = re.compile(r"[\n。；;]+")
GRADE_SCALE_RE = re.compile(
    r"分级标准|评级标准|等级标准|"
    r"\b85\s*[-~到至]\s*100\b|"
    r"\b75\s*[-~到至]\s*84\b|"
    r"\b60\s*[-~到至]\s*74\b|"
    r"60\s*以下"
)
TOTAL_SCORE_RE = re.compile(r"总分\s*[:：]?\s*(\d{1,3}(?:\.\d+)?)\s*(?:/|／)\s*100")


def grade_label_from_short(short: str) -> str | None:
    return GRADE_LABEL_BY_SHORT.get(f"{short.upper()}级")


def extract_grade_label(text: str) -> str | None:
    match = GRADE_MATCH_RE.search(text)
    if not match:
        return None
    return grade_label_from_short(match.group(1))


def is_grade_scale_segment(text: str) -> bool:
    normalized = text.replace(" ", "")
    if all(short in normalized for short, _ in GRADE_LABELS):
        return True
    return GRADE_SCALE_RE.search(text) is not None


def grade_label_from_score(score: float) -> str:
    if score >= 85:
        return "A级重点候选"
    if score >= 75:
        return "B级观察候选"
    if score >= 60:
        return "C级弱候选"
    return "D级淘汰"


def extract_custom_grade(report: str) -> str:
    text = str(report or "")
    explicit_patterns = [
        r"这只股票目前属于\s*[:：]?\s*(?:\*\*)?\s*([ABCD])\s*级",
        r"(?:一句话结论|最终分级|最终评级|最终等级)\s*[:：]?\s*(?:\*\*)?\s*([ABCD])\s*级",
        r"最终结论\s*[:：]?\s*.{0,160}?(?:属于|评为|分级为|评级为|等级为|结论为)\s*[:：]?\s*(?:\*\*)?\s*([ABCD])\s*级",
    ]
    for pattern in explicit_patterns:
        matches = list(re.finditer(pattern, text, re.IGNORECASE | re.DOTALL))
        for match in reversed(matches):
            label = grade_label_from_short(match.group(1))
            if label:
                return label

    score_matches = list(TOTAL_SCORE_RE.finditer(text))
    for match in reversed(score_matches):
        try:
            score = float(match.group(1))
        except ValueError:
            continue
        return grade_label_from_score(score)

    segments = [segment.strip(" -*#\t") for segment in GRADE_SEGMENT_SPLIT_RE.split(text) if segment.strip()]
    for segment in reversed(segments):
        if is_grade_scale_segment(segment):
            continue
        label = extract_grade_label(segment)
        if label:
            return label
    return "HOLD"


def summarize_custom_report(report: str) -> str:
    lines = [line.strip(" -*#\t") for line in report.splitlines() if line.strip()]
    for line in reversed(lines):
        if "这只股票目前属于" in line or "一句话结论" in line:
            return line
    if not lines:
        return ""
    summary = " ".join(lines[-4:])
    return summary[:800]


def confidence_for_grade(action: str) -> str:
    if action.startswith(("A级", "D级")):
        return "HIGH"
    if action.startswith(("B级", "C级")):
        return "MEDIUM"
    return "LOW"


def parse_error_payload(value: Any) -> dict[str, Any] | None:
    if isinstance(value, dict):
        nested = value.get("error")
        if isinstance(nested, dict):
            return nested
        return value
    if not isinstance(value, str):
        return None

    text = value.strip()
    if "{" in text and "}" in text:
        text = text[text.find("{") : text.rfind("}") + 1]
    if not text.startswith("{"):
        return None

    try:
        parsed = ast.literal_eval(text)
    except (SyntaxError, ValueError):
        try:
            parsed = json.loads(text)
        except json.JSONDecodeError:
            return None
    return parsed if isinstance(parsed, dict) else None


def raw_error_message(exc: Exception) -> str:
    body_payload = parse_error_payload(getattr(exc, "body", None))
    if body_payload:
        body_message = body_payload.get("message") or body_payload.get("detail")
        if body_message:
            return str(body_message)
    return str(exc).strip()


def user_error_message(exc: Exception) -> str:
    message = redact_sensitive_text(raw_error_message(exc))
    payload = parse_error_payload(getattr(exc, "body", None)) or parse_error_payload(message)
    payload_code = str((payload or {}).get("code") or (payload or {}).get("type") or "")
    payload_message = redact_sensitive_text(str((payload or {}).get("message") or message))

    if (
        "NoneType" in payload_message
        and "group" in payload_message
    ) or (
        "AttributeError" in payload_code
        and "group" in payload_message
    ):
        return (
            "AI 模型或网关返回的结构化内容解析失败。"
            "这通常是所选模型与 OpenAI 兼容网关的格式兼容问题；"
            "请换用直连模型，或改用更稳定的轻量模型后重试。"
        )
    if "RestrictedModelsError" in message or "Free tier users do not have access" in message:
        return (
            "当前 Vercel AI Gateway 免费层无法访问所选模型。"
            "请把模型改为 anthropic/claude-haiku-4.5，或在 Vercel AI Gateway 充值后再使用 Sonnet/Opus/GPT 高阶模型。"
        )
    return message or "AI 分析中断，请检查模型和网关配置后重试。"


def redact_sensitive_text(value: str, extra_secrets: Iterable[str] = ()) -> str:
    text = str(value or "")
    for name in (
        "ANTHROPIC_API_KEY",
        "OPENAI_API_KEY",
        "OPENAI_COMPATIBLE_API_KEY",
        "AI_GATEWAY_API_KEY",
        "GOOGLE_API_KEY",
        "FINNHUB_API_KEY",
    ):
        secret = os.getenv(name, "").strip()
        if secret:
            text = text.replace(secret, "[REDACTED]")
    for secret in extra_secrets:
        if secret:
            text = text.replace(secret, "[REDACTED]")
    return re.sub(r"\bsk-[A-Za-z0-9_-]{12,}\b", "[REDACTED]", text)


def resolve_model_endpoint(provider: str, backend_url: str | None) -> str:
    normalized = provider.strip().lower()
    defaults = {
        "openai": "https://api.openai.com/v1/models",
        "anthropic": "https://api.anthropic.com/v1/models",
        "google": "https://generativelanguage.googleapis.com/v1beta/models",
    }
    if normalized in defaults:
        return defaults[normalized]
    if normalized != "openai_compatible":
        raise ValueError("unsupported LLM provider")

    base_url = validate_backend_url(backend_url)
    if not base_url:
        raise ValueError("backend_url is required for an OpenAI-compatible provider")
    parsed = urlparse(base_url)
    path = parsed.path.rstrip("/")
    for suffix in ("/chat/completions", "/responses", "/models"):
        if path.endswith(suffix):
            path = path[: -len(suffix)]
            break
    if not path:
        path = "/v1"
    return parsed._replace(path=path + "/models", query="", fragment="").geturl()


def model_discovery_api_key(provider: str, provided_key: str | None = None) -> str:
    if provided_key and provided_key.strip():
        return provided_key.strip()
    normalized = provider.strip().lower()
    if normalized == "openai_compatible":
        return (
            os.getenv("OPENAI_COMPATIBLE_API_KEY", "").strip()
            or os.getenv("AI_GATEWAY_API_KEY", "").strip()
        )
    env_names = {
        "openai": "OPENAI_API_KEY",
        "anthropic": "ANTHROPIC_API_KEY",
        "google": "GOOGLE_API_KEY",
    }
    return os.getenv(env_names.get(normalized, ""), "").strip()


def extract_model_ids(payload: Any, provider: str) -> list[str]:
    candidates: list[Any] = []
    if isinstance(payload, list):
        candidates = payload
    elif isinstance(payload, dict):
        for key in ("data", "models", "result"):
            value = payload.get(key)
            if isinstance(value, list):
                candidates = value
                break

    normalized = provider.strip().lower()
    excluded_fragments = (
        "embedding",
        "moderation",
        "whisper",
        "dall-e",
        "tts",
        "transcribe",
        "realtime",
        "image",
        "audio",
    )
    models: set[str] = set()
    for candidate in candidates:
        model_id = ""
        if isinstance(candidate, str):
            model_id = candidate
        elif isinstance(candidate, dict):
            if normalized == "google":
                methods = candidate.get("supportedGenerationMethods")
                if isinstance(methods, list) and "generateContent" not in methods:
                    continue
            for key in ("id", "name", "model"):
                value = candidate.get(key)
                if isinstance(value, str) and value.strip():
                    model_id = value
                    break
        model_id = model_id.strip()
        if normalized == "google":
            model_id = model_id.removeprefix("models/")
        if not model_id or any(fragment in model_id.lower() for fragment in excluded_fragments):
            continue
        models.add(model_id)
    return sorted(models)[:500]


def discover_available_models(request: ModelDiscoveryRequest) -> dict[str, Any]:
    provider = request.provider.strip().lower()
    endpoint = resolve_model_endpoint(provider, request.backend_url)
    api_key = model_discovery_api_key(provider, request.api_key)
    headers = {
        "Accept": "application/json",
        "User-Agent": "CANSLIMAgent/1.0 model-discovery",
    }
    if provider == "anthropic":
        if api_key:
            headers["x-api-key"] = api_key
        headers["anthropic-version"] = "2023-06-01"
    elif provider == "google":
        if api_key:
            headers["x-goog-api-key"] = api_key
    elif api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    try:
        response = requests.get(endpoint, headers=headers, timeout=15, allow_redirects=False, stream=True)
    except requests.RequestException as exc:
        raise RuntimeError("模型服务连接失败") from exc
    try:
        if not 200 <= response.status_code < 300:
            hint = "，请检查 sidecar 服务端 API Key" if response.status_code in {401, 403} else ""
            raise RuntimeError(f"模型服务返回 HTTP {response.status_code}{hint}")
        content_length = response.headers.get("content-length", "")
        if content_length.isdigit() and int(content_length) > MAX_MODEL_DISCOVERY_RESPONSE_BYTES:
            raise RuntimeError("模型服务响应过大")
        body = bytearray()
        for chunk in response.iter_content(chunk_size=64 * 1024):
            body.extend(chunk)
            if len(body) > MAX_MODEL_DISCOVERY_RESPONSE_BYTES:
                raise RuntimeError("模型服务响应过大")
        try:
            payload = json.loads(body)
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise RuntimeError("模型服务返回了无效 JSON") from exc
    finally:
        response.close()
    models = extract_model_ids(payload, provider)
    if not models:
        raise RuntimeError("模型服务没有返回可用于分析的模型")
    return {"models": models, "source": urlparse(endpoint).hostname or "model endpoint"}


async def analysis_events(request: AnalysisRequest):
    emit_vibe_report = not request.resume or request.last_event_id == 0
    if emit_vibe_report:
        yield sse(
            "progress",
            {
                "agent": "VibeEvidence",
                "status": "running",
                "message": "正在通过 Vibe-Trading MCP 预取并核验数据来源...",
            },
        )
    try:
        vibe_bundle = await attach_vibe_evidence(request)
        vibe_markdown = render_vibe_evidence(vibe_bundle)
        if emit_vibe_report:
            yield sse("result", {"agent": "VibeEvidence", "content": vibe_markdown})
        vibe_status = "done" if vibe_bundle.get("status") in {"complete", "partial", "disabled", "not_requested"} else "error"
        yield sse(
            "progress",
            {
                "agent": "VibeEvidence",
                "status": vibe_status,
                "message": (
                    ("Vibe 数据预取完成，已注入 TradingAgents" if emit_vibe_report else "Vibe 数据已重新核验，正在从 checkpoint 恢复")
                    if vibe_status == "done"
                    else "Vibe 数据不可用，已记录来源轨迹并降级"
                ),
            },
        )
        data_audit = request.analysis_context.get("data_audit", {})
        if emit_vibe_report:
            yield sse(
                "progress",
                {
                    "agent": "DataAuditor",
                    "status": "running",
                    "message": "正在按来源、报告期和原始表单审计关键数据...",
                },
            )
            yield sse("result", {"agent": "DataAuditor", "content": render_data_audit(data_audit)})
            audit_status = "done" if data_audit.get("status") in {"clean", "issues_found", "not_available"} else "error"
            yield sse(
                "progress",
                {
                    "agent": "DataAuditor",
                    "status": audit_status,
                    "message": "数据审计完成；冲突值已从可引用事实中排除" if audit_status == "done" else "数据审计不可用",
                },
            )
    except Exception as exc:  # noqa: BLE001 - data-source failure must degrade
        request.analysis_context = dict(request.analysis_context or {})
        request.analysis_context["vibe_trading"] = {
            "schema_version": "vibe.research_evidence.v1",
            "provider": "vibe-trading",
            "status": "unavailable",
            "error": str(exc),
            "summary": {"requested": 0, "ok": 0, "failed": 0},
            "evidence": {},
            "source_traces": [],
        }
        request.analysis_context["data_audit"] = audit_data(request.analysis_context["vibe_trading"], request.analysis_context)
        if emit_vibe_report:
            yield sse("result", {"agent": "VibeEvidence", "content": render_vibe_evidence(request.analysis_context["vibe_trading"])})
            yield sse("result", {"agent": "DataAuditor", "content": render_data_audit(request.analysis_context["data_audit"])})
        yield sse(
            "progress",
            {
                "agent": "VibeEvidence",
                "status": "error",
                "message": "Vibe 预取异常，TradingAgents 已降级继续运行",
            },
        )
        yield sse(
            "progress",
            {"agent": "DataAuditor", "status": "error", "message": "原始数据不可用，无法完成可复算审计"},
        )

    selected_keys = set(selected_builtin_analysts(request))
    custom_agents = selected_custom_analysts(request)
    emitted_agents: list[str] = []
    for mapped_key, agent, message in ANALYST_MAP.values():
        if mapped_key in selected_keys and agent not in emitted_agents:
            emitted_agents.append(agent)
            yield sse("progress", {"agent": agent, "status": "running", "message": message})

    if selected_keys:
        for agent, message in [
            ("ResearchTeam", "研究团队正在展开多空辩论..."),
            ("Trader", "交易员正在生成投资计划..."),
            ("RiskManagement", "风险管理团队正在评估仓位与回撤..."),
            ("PortfolioManager", "投资组合经理正在形成最终建议..."),
        ]:
            yield sse("progress", {"agent": agent, "status": "pending", "message": message})

    for agent in custom_agents:
        yield sse(
            "progress",
            {
                "agent": agent,
                "status": "pending",
                "message": CUSTOM_ANALYST_MAP["canslim_second_layer"]["message"],
            },
        )

    model_output_queue: Queue[str] = Queue()
    callbacks = [StreamingCallback(model_output_queue)]
    loop = asyncio.get_running_loop()
    future = loop.run_in_executor(None, run_analysis, request, callbacks)
    custom_reports: list[tuple[str, str]] = []

    try:
        while not future.done():
            async for event in drain_model_output(model_output_queue):
                yield event
            yield sse("heartbeat", {})
            await asyncio.sleep(NARRATION_INTERVAL_SECONDS)

        state, decision = await future
        async for event in drain_model_output(model_output_queue):
            yield event
        for agent, _ in iter_reports(state):
            yield sse("progress", {"agent": agent, "status": "done", "message": "分析完成"})

        selected_roles = selected_builtin_analysts(request)
        yield sse(
            "progress",
            {
                "agent": "VibeEvidenceReconciler",
                "status": "running",
                "message": "正在对账分析师结论与 Vibe 证据覆盖度...",
            },
        )
        reconciliation = render_evidence_reconciliation(
            request.analysis_context.get("vibe_trading", {}), state, selected_roles
        )
        yield sse("result", {"agent": "VibeEvidenceReconciler", "content": reconciliation})
        yield sse(
            "progress",
            {
                "agent": "VibeEvidenceReconciler",
                "status": "done",
                "message": "证据对账完成；缺失数据已标记为未知",
            },
        )

        yield sse(
            "progress",
            {
                "agent": "VibeExecutionValidation",
                "status": "running",
                "message": "正在把 PM 交易建议交给 Vibe 做历史执行验证...",
            },
        )
        if request.online_tools:
            original_pm_decision = state.get("final_trade_decision") or decision
            validation_future = loop.run_in_executor(
                None,
                validate_execution_plan,
                request.ticker,
                original_pm_decision,
                state.get("trader_investment_plan", ""),
            )
            while not validation_future.done():
                yield sse("heartbeat", {})
                await asyncio.sleep(NARRATION_INTERVAL_SECONDS)
            validation = await validation_future
        else:
            validation = {"schema_version": "vibe.execution_validation.v1", "status": "disabled"}
        request.analysis_context["vibe_execution_validation"] = validation
        yield sse("result", {"agent": "VibeExecutionValidation", "content": render_execution_validation(validation)})
        validation_status = "done" if validation.get("status") in {"complete", "not_actionable", "disabled"} else "error"
        validation_message = (
            "历史执行验证完成，最终建议将携带该验证结果"
            if validation_status == "done"
            else "执行验证不可用；最终建议将明确标记为未验证"
        )
        yield sse(
            "progress",
            {
                "agent": "VibeExecutionValidation",
                "status": validation_status,
                "message": validation_message,
            },
        )

        for agent in custom_agents:
            yield sse(
                "progress",
                {
                    "agent": agent,
                    "status": "running",
                    "message": CUSTOM_ANALYST_MAP["canslim_second_layer"]["message"],
                },
            )
            custom_future = loop.run_in_executor(None, run_custom_analysis, request, agent, callbacks)
            while not custom_future.done():
                async for event in drain_model_output(model_output_queue):
                    yield event
                yield sse("heartbeat", {})
                await asyncio.sleep(NARRATION_INTERVAL_SECONDS)
            content = await custom_future
            async for event in drain_model_output(model_output_queue):
                yield event
            if content:
                custom_reports.append((agent, content))
            yield sse("progress", {"agent": agent, "status": "done", "message": "分析完成"})

        report = "\n\n".join(
            part for part in [compose_report(state), compose_custom_reports(custom_reports)] if part
        )
        if report:
            yield sse("report_start", {})
            async for event in stream_delta(
                "report_delta",
                report,
                chunk_size=REPORT_DELTA_CHARS,
                delay=REPORT_DELTA_DELAY,
            ):
                yield event

        if custom_reports:
            latest_custom_report = custom_reports[-1][1]
            action = extract_custom_grade(latest_custom_report)
            summary = summarize_custom_report(latest_custom_report)
            confidence = confidence_for_grade(action)
        else:
            action = normalize_action(decision)
            summary = str(state.get("final_trade_decision") or decision or "")
            confidence = confidence_for_action(action, decision)
            action, execution_gate_note = apply_execution_gate(action, validation)
            if execution_gate_note:
                summary = "\n\n".join(part for part in [summary, f"Vibe 执行门：{execution_gate_note}"] if part)
            if action == "HOLD" and validation.get("status") == "complete":
                confidence = "LOW"
        yield sse(
            "decision",
            {
                "action": action,
                "confidence": confidence,
                "summary": summary,
            },
        )
        yield sse("done", {})
    except Exception as exc:
        if _env_enabled("SIDECAR_DEBUG_TRACEBACK") and os.getenv("APP_ENV", "development").lower() != "production":
            print(
                "AI analysis stream failed:\n"
                + redact_sensitive_text(traceback.format_exc(), [request.api_key or ""]),
                flush=True,
            )
        else:
            print(
                json.dumps(
                    {
                        "event": "analysis_failed",
                        "error_type": type(exc).__name__,
                        "run_id": request.run_id,
                    },
                    ensure_ascii=False,
                ),
                flush=True,
            )
        message = user_error_message(exc)
        if request.api_key:
            message = message.replace(request.api_key, "[REDACTED]")
        yield sse("error", {"message": message})


@app.get("/health")
def health():
    status = runtime_status()
    return {
        "status": "ok",
        "runtime": {
            "mode": status.get("mode"),
            "vendor_available": status.get("vendor_available"),
            "vibe_root_available": status.get("vibe_root_available"),
            "vibe_tools_enabled": status.get("vibe_tools_enabled"),
        },
    }


@app.post("/models")
async def models(request: ModelDiscoveryRequest):
    try:
        return await asyncio.to_thread(discover_available_models, request)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/analyze")
async def analyze(request: AnalysisRequest):
    try:
        request.backend_url = validate_backend_url(request.backend_url)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    run_id = request.run_id or f"run-{uuid.uuid4()}"
    try:
        request.run_id = validate_run_id(run_id)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    fingerprint_payload = {
        "ticker": request.ticker.strip().upper(),
        "llm_provider": request.llm_provider,
        "deep_think_llm": request.deep_think_llm,
        "quick_think_llm": request.quick_think_llm,
        "backend_url": request.backend_url,
        "max_debate_rounds": request.max_debate_rounds,
        "max_risk_rounds": request.max_risk_rounds,
        "online_tools": request.online_tools,
        "analysts": request.analysts,
        "run_id": request.run_id,
    }
    fingerprint = request_fingerprint(fingerprint_payload)
    try:
        run, _ = analysis_run_registry.get_or_create(
            run_id=request.run_id,
            fingerprint=fingerprint,
            last_event_id=request.last_event_id,
            resume=request.resume,
            source_factory=lambda: analysis_events(request),
        )
    except RunConflictError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except RunCapacityError as exc:
        raise HTTPException(status_code=429, detail=str(exc)) from exc
    headers = {
        "Cache-Control": "no-cache",
        "Connection": "keep-alive",
        "X-Accel-Buffering": "no",
        "X-Analysis-Run-ID": request.run_id,
    }
    return StreamingResponse(
        run.stream_after(request.last_event_id),
        media_type="text/event-stream",
        headers=headers,
    )


@app.post("/journal/reflect")
async def reflect_journal(request: JournalReflectionRequest):
    if request.schema_version != "vibe.trade_record.v1":
        raise HTTPException(status_code=400, detail="schema_version must be vibe.trade_record.v1")
    return await asyncio.to_thread(build_journal_reflection, request.records)


@app.post("/broker/sync")
async def sync_broker_account(request: BrokerSyncRequest):
    try:
        return await asyncio.to_thread(sync_broker, request.broker, request.credentials, request.start_at, request.end_at)
    except (RuntimeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


@app.post("/broker/connection-status")
async def broker_connection_status(request: BrokerGatewayDiscoveryRequest):
    try:
        return await asyncio.to_thread(discover_broker_account, request.broker, request.credentials)
    except (RuntimeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


if __name__ == "__main__":
    uvicorn.run(
        "main:app",
        host=os.getenv("SIDECAR_HOST", "127.0.0.1"),
        port=int(os.getenv("SIDECAR_PORT", "8765")),
    )
