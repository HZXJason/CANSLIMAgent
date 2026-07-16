import asyncio
import ast
import copy
import hashlib
import hmac
import ipaddress
import json
import math
import os
import re
import socket
import time
import traceback
import uuid
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from queue import Empty, Queue
from threading import Event, Lock, Thread
from datetime import date, datetime, timedelta
from pathlib import Path
from typing import Any, AsyncIterator, Callable, Iterable
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
    build_research_pack,
    render_evidence_reconciliation,
)
from quant_trend import InsufficientDataError, PriceDataError, QuantTrendService
from ablation_study import run_study as run_ablation_study

load_dotenv()

app = FastAPI(title="AlphaLens TradingAgents Sidecar")
quant_trend_service = QuantTrendService()

# This is a protocol header name, not a password.
SIDECAR_TOKEN_HEADER = "X-Sidecar-Token"  # nosec B105
MAX_REQUEST_BODY_BYTES = int(os.getenv("SIDECAR_MAX_REQUEST_BODY_BYTES", str(4 * 1024 * 1024)))
MAX_MODEL_DISCOVERY_RESPONSE_BYTES = 2 * 1024 * 1024
MAX_CONCURRENT_REQUESTS = max(1, int(os.getenv("SIDECAR_MAX_CONCURRENT_REQUESTS", "16")))
_active_requests = 0
_active_requests_lock = asyncio.Lock()
_calibration_cancel_events: dict[str, Event] = {}
_calibration_cancel_lock = Lock()
_ablation_cancel_events: dict[str, Event] = {}
_ablation_cancel_lock = Lock()


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


class CalibrationRequest(AnalysisRequest):
    job_id: str | None = Field(default=None, min_length=1, max_length=128, pattern=r"^[A-Za-z0-9_-]+$")
    sample_count: int = Field(default=10, ge=5, le=30)
    horizon_days: int = Field(default=20, ge=5, le=252)
    force_refresh: bool = False


class AblationStudyRequest(AnalysisRequest):
    job_id: str | None = Field(default=None, min_length=1, max_length=128, pattern=r"^[A-Za-z0-9_-]+$")
    start_date: date = date(2022, 1, 3)
    end_date: date = date(2025, 6, 30)
    horizon_days: int = Field(default=20, ge=5, le=60)
    maximum_nodes: int = Field(default=45, ge=5, le=60)
    repeats: int = Field(default=1, ge=1, le=5)
    concurrency: int = Field(default=7, ge=1, le=10)
    transaction_cost_bps: float = Field(default=10.0, ge=0, le=500)
    max_attempts: int = Field(default=3, ge=1, le=5)
    adaptive_stages: bool = True
    stage_one_nodes: int = Field(default=30, ge=5, le=55)
    reproducibility_nodes: int = Field(default=5, ge=1, le=20)

    @field_validator("analysts")
    @classmethod
    def validate_ablation_analysts(cls, values: list[str]) -> list[str]:
        normalized = [value.strip().lower() for value in values]
        if normalized not in (["technical"], ["market"]):
            raise ValueError("confirmatory ablation requires the point-in-time technical/market analyst only")
        return ["technical"]


class QuantTrendRequest(StrictRequestModel):
    symbol: str = Field(..., min_length=1, max_length=32, pattern=r"^[A-Za-z0-9][A-Za-z0-9.^=_-]{0,31}$")
    as_of_date: date = Field(default_factory=date.today)
    horizon_days: int = Field(default=20, ge=5, le=252)
    universe: list[str] = Field(default_factory=list, max_length=500)
    universe_version: str | None = Field(default=None, min_length=1, max_length=128)
    force_refresh: bool = False

    @field_validator("universe")
    @classmethod
    def validate_universe(cls, values: list[str]) -> list[str]:
        symbol_pattern = re.compile(r"^[A-Za-z0-9][A-Za-z0-9.^=_-]{0,31}$")
        if any(not symbol_pattern.fullmatch(value.strip()) for value in values):
            raise ValueError("universe symbols must be 1-32 characters")
        return values


class QuantTrendEvaluationRequest(StrictRequestModel):
    symbol: str = Field(..., min_length=1, max_length=32, pattern=r"^[A-Za-z0-9][A-Za-z0-9.^=_-]{0,31}$")
    start_date: date
    end_date: date
    horizon_days: int = Field(default=20, ge=5, le=252)
    step_days: int | None = Field(default=None, ge=1, le=252)
    maximum_nodes: int = Field(default=500, ge=1, le=1000)
    transaction_cost_bps: float = Field(default=10.0, ge=0, le=500)


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


class EvidenceUsageCallback(BaseCallbackHandler):
    """Best-effort non-secret usage ledger across all LLM calls in one graph."""

    def __init__(self) -> None:
        self._lock = Lock()
        self._calls = 0
        self._reported_calls = 0
        self._prompt_tokens = 0
        self._completion_tokens = 0
        self._total_tokens = 0
        self._seen_llm_runs: set[str] = set()
        self._tool_starts: dict[str, dict[str, Any]] = {}
        self._tool_calls: list[dict[str, Any]] = []

    @staticmethod
    def _usage_from_response(response: Any) -> dict[str, Any]:
        output = getattr(response, "llm_output", None)
        if isinstance(output, dict):
            for key in ("token_usage", "usage"):
                usage = output.get(key)
                if isinstance(usage, dict):
                    return usage
        for generation_group in getattr(response, "generations", None) or []:
            for generation in generation_group or []:
                message = getattr(generation, "message", None)
                usage = getattr(message, "usage_metadata", None)
                if isinstance(usage, dict):
                    return usage
        return {}

    @staticmethod
    def _int(usage: dict[str, Any], *keys: str) -> int:
        for key in keys:
            value = usage.get(key)
            if isinstance(value, int) and not isinstance(value, bool):
                return max(0, value)
        return 0

    @classmethod
    def _raw_value(cls, value: Any) -> Any:
        if value is None or isinstance(value, (str, int, float, bool)):
            return value
        if isinstance(value, dict):
            return {str(key): cls._raw_value(child) for key, child in value.items()}
        if isinstance(value, (list, tuple)):
            return [cls._raw_value(child) for child in value]
        content = getattr(value, "content", None)
        if content is not None and content is not value:
            return cls._raw_value(content)
        return str(value)

    def on_llm_end(self, response: Any, **kwargs: Any) -> None:
        usage = self._usage_from_response(response)
        prompt = self._int(usage, "prompt_tokens", "input_tokens", "prompt_token_count")
        completion = self._int(usage, "completion_tokens", "output_tokens", "candidates_token_count")
        total = self._int(usage, "total_tokens", "total_token_count") or prompt + completion
        run_id = str(kwargs.get("run_id") or "")
        with self._lock:
            if run_id and run_id in self._seen_llm_runs:
                return
            if run_id:
                self._seen_llm_runs.add(run_id)
            self._calls += 1
            if usage:
                self._reported_calls += 1
            self._prompt_tokens += prompt
            self._completion_tokens += completion
            self._total_tokens += total

    def on_tool_start(
        self,
        serialized: dict[str, Any],
        input_str: str,
        **kwargs: Any,
    ) -> None:
        run_id = str(kwargs.get("run_id") or uuid.uuid4())
        name = str((serialized or {}).get("name") or kwargs.get("name") or "unknown")
        inputs = kwargs.get("inputs")
        with self._lock:
            self._tool_starts[run_id] = {
                "run_id": run_id,
                "name": name,
                "input": self._raw_value(inputs if isinstance(inputs, dict) else input_str),
                "started_monotonic": time.monotonic(),
            }

    def _finish_tool(self, run_id: Any, *, output: Any = None, error: Any = None) -> None:
        key = str(run_id or "")
        with self._lock:
            started = self._tool_starts.pop(key, None) or {
                "run_id": key,
                "name": "unknown",
                "input": None,
                "started_monotonic": time.monotonic(),
            }
            elapsed = time.monotonic() - float(started.pop("started_monotonic"))
            self._tool_calls.append(
                {
                    **started,
                    "output": self._raw_value(output),
                    "error": str(error) if error is not None else None,
                    "duration_seconds": round(elapsed, 6),
                }
            )

    def on_tool_end(self, output: Any, **kwargs: Any) -> None:
        self._finish_tool(kwargs.get("run_id"), output=output)

    def on_tool_error(self, error: BaseException, **kwargs: Any) -> None:
        self._finish_tool(kwargs.get("run_id"), error=error)

    def snapshot(self) -> dict[str, Any]:
        with self._lock:
            return {
                "llm_calls": self._calls,
                "usage_reported_calls": self._reported_calls,
                "usage_missing_calls": self._calls - self._reported_calls,
                "prompt_tokens": self._prompt_tokens,
                "completion_tokens": self._completion_tokens,
                "total_tokens": self._total_tokens,
                "tool_calls": copy.deepcopy(self._tool_calls),
                "unfinished_tool_calls": len(self._tool_starts),
            }


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
    configure_historical_isolation(config, request)
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


def configure_historical_isolation(config: dict[str, Any], request: AnalysisRequest) -> bool:
    calibration_context = (
        request.analysis_context.get("calibration")
        if isinstance(request.analysis_context, dict)
        else None
    )
    if not isinstance(calibration_context, dict):
        return False
    # Historical nodes must be independent. The normal global memory log
    # contains later decisions and realized outcomes, and concurrent replay
    # arms would otherwise read/write each other's state.
    config["memory_log_path"] = None
    config["historical_isolation"] = True
    return True


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


def run_analysis(
    request: AnalysisRequest,
    callbacks: list[Any] | None = None,
    *,
    analysis_date: date | None = None,
    analysts: tuple[str, ...] | None = None,
) -> tuple[dict[str, Any], Any]:
    analysts = analysts if analysts is not None else selected_builtin_analysts(request)
    if not analysts:
        return {}, None
    graph = configured_graph(request, analysts=analysts, callbacks=callbacks)
    as_of = analysis_date or date.today()
    state, decision = graph.propagate(request.ticker.strip().upper(), as_of.isoformat())
    return state, decision


def _calibration_price_frame(ticker: str):
    """Load one aligned stock/benchmark frame for selecting and scoring nodes."""
    import pandas as pd
    import yfinance as yf
    from tradingagents.dataflows.symbol_utils import normalize_symbol

    end = date.today() + timedelta(days=1)
    start = end - timedelta(days=365 * 5)
    stock = yf.Ticker(normalize_symbol(ticker)).history(start=start.isoformat(), end=end.isoformat(), auto_adjust=True)
    benchmark = yf.Ticker("SPY").history(start=start.isoformat(), end=end.isoformat(), auto_adjust=True)
    if stock.empty or benchmark.empty:
        raise ValueError("无法取得标的或 SPY 的历史价格")
    stock_index = pd.to_datetime(stock.index).tz_localize(None)
    benchmark_index = pd.to_datetime(benchmark.index).tz_localize(None)
    stock_frame = pd.DataFrame(
        {
            "stock_open": stock["Open"].to_numpy(),
            "stock_high": stock["High"].to_numpy(),
            "stock_low": stock["Low"].to_numpy(),
            "stock_close": stock["Close"].to_numpy(),
        },
        index=stock_index,
    )
    benchmark_frame = pd.DataFrame(
        {
            "benchmark_open": benchmark["Open"].to_numpy(),
            "benchmark_close": benchmark["Close"].to_numpy(),
        },
        index=benchmark_index,
    )
    frame = pd.concat([stock_frame, benchmark_frame], axis=1, join="inner").dropna()
    # Close aliases preserve the small public surface used by node selection and
    # older callers while scoring uses the full point-in-time OHLC path.
    frame["stock"] = frame["stock_close"]
    frame["benchmark"] = frame["benchmark_close"]
    if len(frame) < 400:
        raise ValueError("历史价格不足，无法构造可靠的时间截面")
    return frame


def _calibration_node_indexes(frame: Any, sample_count: int, horizon_days: int) -> list[int]:
    """Choose evenly distributed point-in-time nodes from the latest three years."""
    latest = len(frame) - horizon_days - 1
    earliest_date = frame.index[latest] - timedelta(days=365 * 3)
    earliest = int(frame.index.searchsorted(earliest_date))
    if latest <= earliest or latest - earliest + 1 < sample_count:
        raise ValueError("可验证历史节点不足")
    if sample_count == 1:
        return [latest]
    span = latest - earliest
    return sorted({earliest + round(span * index / (sample_count - 1)) for index in range(sample_count)})


def _calibration_confidence(
    correct_count: int,
    sample_count: int,
    best_baseline_accuracy: float = 0.0,
) -> str:
    """Grade only demonstrated skill above a constant-direction baseline."""
    if sample_count < 10:
        return "INSUFFICIENT"
    accuracy = correct_count / sample_count
    lift = accuracy - best_baseline_accuracy
    if accuracy >= 0.8 and lift >= 0.1:
        return "HIGH"
    if accuracy >= 0.6 and lift > 0:
        return "MEDIUM"
    return "LOW"


CALIBRATION_SCHEMA_VERSION = "vibe.decision_calibration.v5"
CALIBRATION_NODE_SCHEMA_VERSION = "vibe.decision_calibration.node.v5"
CALIBRATION_AUDIT_SCHEMA_VERSION = "vibe.decision_calibration.audit.v5"
LEGACY_CALIBRATION_NODE_SCHEMA_VERSION = "vibe.decision_calibration.node.v4"
OLDER_CALIBRATION_NODE_SCHEMA_VERSION = "vibe.decision_calibration.node.v3"
LEGACY_CALIBRATION_NODE_SCHEMA_VERSIONS = (
    LEGACY_CALIBRATION_NODE_SCHEMA_VERSION,
    OLDER_CALIBRATION_NODE_SCHEMA_VERSION,
)


def _calibration_node_cache_path(
    request: CalibrationRequest,
    as_of: date,
    safe_analysts: tuple[str, ...],
    schema_version: str = CALIBRATION_NODE_SCHEMA_VERSION,
) -> Path:
    fingerprint = {
        "schema_version": schema_version,
        "symbol": request.ticker.strip().upper(),
        "provider": request.llm_provider,
        "deep_think_llm": request.deep_think_llm,
        "quick_think_llm": request.quick_think_llm,
        "backend_url": (request.backend_url or "").rstrip("/"),
        "max_debate_rounds": request.max_debate_rounds,
        "max_risk_rounds": request.max_risk_rounds,
        "online_tools": request.online_tools,
        "analysts": sorted(safe_analysts),
        "sample_count": request.sample_count,
        "horizon_days": request.horizon_days,
        "as_of_date": as_of.isoformat(),
    }
    encoded = json.dumps(fingerprint, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    digest = hashlib.sha256(encoded).hexdigest()
    root = Path(
        os.getenv(
            "AI_CALIBRATION_NODE_CACHE_DIR",
            str(Path(__file__).resolve().parent / "data" / "calibration-nodes"),
        )
    ).expanduser()
    return root / f"{digest}.json"


def _load_calibration_node(
    path: Path,
    schema_version: str = CALIBRATION_NODE_SCHEMA_VERSION,
) -> dict[str, Any] | None:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (FileNotFoundError, OSError, json.JSONDecodeError):
        return None
    if not isinstance(payload, dict) or payload.get("schema_version") != schema_version:
        return None
    node = payload.get("node")
    return node if isinstance(node, dict) else None


def _store_calibration_node(path: Path, node: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    payload = {"schema_version": CALIBRATION_NODE_SCHEMA_VERSION, "node": node}
    temporary.write_text(json.dumps(payload, ensure_ascii=False, sort_keys=True), encoding="utf-8")
    temporary.replace(path)


def _store_calibration_audit(
    cache_path: Path,
    *,
    state: dict[str, Any],
    raw_decision: Any,
    rating: str,
    direction: str,
    decision_source: str,
) -> str:
    """Persist full reasoning beside the compact node cache for later audit."""
    audit_path = cache_path.with_suffix(".audit.json")
    fields = (
        "market_report",
        "fundamentals_report",
        "news_report",
        "sentiment_report",
        "investment_plan",
        "trader_investment_plan",
        "final_trade_decision",
        "investment_debate_state",
        "risk_debate_state",
    )
    payload = {
        "schema_version": CALIBRATION_AUDIT_SCHEMA_VERSION,
        "rating": rating,
        "direction": direction,
        "decision_source": decision_source,
        "raw_decision": str(raw_decision or ""),
        "state": {field: state.get(field) for field in fields if state.get(field)},
    }
    audit_path.parent.mkdir(parents=True, exist_ok=True)
    temporary = audit_path.with_name(f".{audit_path.name}.{uuid.uuid4().hex}.tmp")
    temporary.write_text(
        json.dumps(payload, ensure_ascii=False, sort_keys=True, default=str),
        encoding="utf-8",
    )
    temporary.replace(audit_path)
    return audit_path.name


def _load_calibration_audit(cache_path: Path) -> dict[str, Any] | None:
    try:
        payload = json.loads(cache_path.with_suffix(".audit.json").read_text(encoding="utf-8"))
    except (FileNotFoundError, OSError, json.JSONDecodeError):
        return None
    return payload if isinstance(payload, dict) and isinstance(payload.get("state"), dict) else None


def _prefer_legacy_calibration_index(
    frame: Any,
    frame_index: int,
    request: CalibrationRequest,
    safe_analysts: tuple[str, ...],
) -> int:
    """Keep historical cutoffs stable when the latest Yahoo bar temporarily shifts."""
    for offset in (0, 1, -1, 2, -2, 3, -3):
        candidate = frame_index + offset
        if candidate < 0 or candidate + request.horizon_days >= len(frame):
            continue
        as_of = frame.index[candidate].date()
        for schema_version in LEGACY_CALIBRATION_NODE_SCHEMA_VERSIONS:
            legacy_path = _calibration_node_cache_path(
                request,
                as_of,
                safe_analysts,
                schema_version=schema_version,
            )
            if (
                _load_calibration_node(legacy_path, schema_version=schema_version) is not None
                and _load_calibration_audit(legacy_path) is not None
            ):
                return candidate
    return frame_index


def _calibration_pending_path(cache_path: Path) -> Path:
    return cache_path.with_suffix(".pending.json")


def _load_or_create_calibration_run_id(cache_path: Path) -> tuple[str, bool]:
    pending_path = _calibration_pending_path(cache_path)
    try:
        payload = json.loads(pending_path.read_text(encoding="utf-8"))
        run_id = str(payload.get("run_id") or "") if isinstance(payload, dict) else ""
        if run_id.startswith("cal-"):
            return run_id, True
    except (FileNotFoundError, OSError, json.JSONDecodeError):
        pass
    run_id = f"cal-{uuid.uuid4()}"
    pending_path.parent.mkdir(parents=True, exist_ok=True)
    temporary = pending_path.with_name(f".{pending_path.name}.{uuid.uuid4().hex}.tmp")
    temporary.write_text(json.dumps({"run_id": run_id}), encoding="utf-8")
    temporary.replace(pending_path)
    return run_id, False


def _calibration_concurrency(sample_count: int) -> int:
    try:
        configured = int(os.getenv("AI_CALIBRATION_CONCURRENCY", "3"))
    except ValueError:
        configured = 3
    return max(1, min(10, sample_count, configured))


def _score_calibration_path(
    frame: Any,
    *,
    frame_index: int,
    horizon_days: int,
    action: str,
) -> dict[str, Any]:
    """Score a decision by the first volatility-adjusted barrier hit.

    Entry is the next aligned trading-day open. A directional signal succeeds
    only when its profit barrier is reached before its stop; no barrier within
    the evaluation window is neutral and counts as correct. With daily OHLC we
    cannot know intraday order when both barriers are touched on the same day,
    so that case is conservatively marked ambiguous and incorrect.
    """
    entry_index = frame_index + 1
    outcome_index = frame_index + horizon_days
    window = frame.iloc[entry_index : outcome_index + 1]
    if len(window) != horizon_days:
        raise ValueError("历史价格不足，无法完成未来路径判分")

    entry_stock = float(window.iloc[0]["stock_open"])
    entry_benchmark = float(window.iloc[0]["benchmark_open"])
    if entry_stock <= 0 or entry_benchmark <= 0:
        raise ValueError("历史开盘价格无效")

    atr_start = max(1, frame_index - 19)
    true_ranges: list[float] = []
    for index in range(atr_start, frame_index + 1):
        row = frame.iloc[index]
        previous_close = float(frame.iloc[index - 1]["stock_close"])
        high = float(row["stock_high"])
        low = float(row["stock_low"])
        true_ranges.append(max(high - low, abs(high - previous_close), abs(low - previous_close)))
    if not true_ranges:
        raise ValueError("历史波动率数据不足")
    as_of_close = float(frame.iloc[frame_index]["stock_close"])
    atr_percent = sum(true_ranges) / len(true_ranges) / as_of_close
    horizon_scale = math.sqrt(max(1.0, horizon_days / 20.0))
    profit_threshold = max(0.03, atr_percent) * horizon_scale
    stop_threshold = max(0.02, atr_percent * 0.75)

    rating = action.upper()
    highs = [float(value) for value in window["stock_high"]]
    lows = [float(value) for value in window["stock_low"]]
    stock_close_returns = [float(value) / entry_stock - 1 for value in window["stock_close"]]
    benchmark_close_returns = [float(value) / entry_benchmark - 1 for value in window["benchmark_close"]]
    alpha_path = [stock_return - benchmark_return for stock_return, benchmark_return in zip(stock_close_returns, benchmark_close_returns)]
    if rating == "SELL":
        favorable_moves = [(entry_stock - low) / entry_stock for low in lows]
        adverse_moves = [(high - entry_stock) / entry_stock for high in highs]
        scoring_basis = "absolute"
    elif rating == "BUY":
        favorable_moves = [(high - entry_stock) / entry_stock for high in highs]
        adverse_moves = [(entry_stock - low) / entry_stock for low in lows]
        scoring_basis = "absolute"
    elif rating == "UNDERWEIGHT":
        favorable_moves = [-alpha for alpha in alpha_path]
        adverse_moves = alpha_path
        scoring_basis = "relative_to_spy"
    elif rating == "OVERWEIGHT":
        favorable_moves = alpha_path
        adverse_moves = [-alpha for alpha in alpha_path]
        scoring_basis = "relative_to_spy"
    else:
        favorable_moves = [max((high - entry_stock) / entry_stock, (entry_stock - low) / entry_stock) for high, low in zip(highs, lows)]
        adverse_moves = favorable_moves
        scoring_basis = "absolute_neutral"

    outcome = "neutral"
    first_barrier = "none"
    barrier_hit_day: int | None = None
    barrier_hit_date: str | None = None
    for offset, (timestamp, row) in enumerate(window.iterrows(), start=1):
        high = float(row["stock_high"])
        low = float(row["stock_low"])
        if rating == "SELL":
            profit_hit = (entry_stock - low) / entry_stock >= profit_threshold
            stop_hit = (high - entry_stock) / entry_stock >= stop_threshold
        elif rating == "BUY":
            profit_hit = (high - entry_stock) / entry_stock >= profit_threshold
            stop_hit = (entry_stock - low) / entry_stock >= stop_threshold
        elif rating in {"UNDERWEIGHT", "OVERWEIGHT"}:
            alpha = alpha_path[offset - 1]
            favorable = -alpha if rating == "UNDERWEIGHT" else alpha
            adverse = alpha if rating == "UNDERWEIGHT" else -alpha
            profit_hit = favorable >= profit_threshold
            stop_hit = adverse >= stop_threshold
        else:
            up_hit = (high - entry_stock) / entry_stock >= profit_threshold
            down_hit = (entry_stock - low) / entry_stock >= profit_threshold
            if up_hit and down_hit:
                outcome, first_barrier = "ambiguous", "both"
            elif up_hit or down_hit:
                outcome = "wrong"
                first_barrier = "up_move" if up_hit else "down_move"
            else:
                continue
            barrier_hit_day = offset
            barrier_hit_date = timestamp.date().isoformat()
            break
        if profit_hit and stop_hit:
            outcome, first_barrier = "ambiguous", "both"
        elif profit_hit:
            outcome, first_barrier = "correct", "profit"
        elif stop_hit:
            outcome = "wrong"
            first_barrier = "stop"
        else:
            continue
        barrier_hit_day = offset
        barrier_hit_date = timestamp.date().isoformat()
        break

    raw_return = float(window.iloc[-1]["stock_close"]) / entry_stock - 1
    benchmark_return = float(window.iloc[-1]["benchmark_close"]) / entry_benchmark - 1
    execution_outcome = outcome
    execution_correct = execution_outcome in {"correct", "neutral"}
    opportunity_correct = (
        execution_correct
        if rating == "HOLD"
        else execution_outcome == "neutral" or max(favorable_moves) >= profit_threshold
    )
    opportunity_outcome = "correct" if opportunity_correct else "wrong"
    return {
        "outcome_date": window.index[-1].date().isoformat(),
        "outcome": opportunity_outcome,
        "correct": opportunity_correct,
        "opportunity_outcome": opportunity_outcome,
        "opportunity_correct": opportunity_correct,
        "execution_outcome": execution_outcome,
        "execution_correct": execution_correct,
        "scoring_basis": scoring_basis,
        "first_barrier": first_barrier,
        "barrier_hit_day": barrier_hit_day,
        "barrier_hit_date": barrier_hit_date,
        "entry_price": round(entry_stock, 6),
        "atr_percent": round(atr_percent, 6),
        "profit_threshold": round(profit_threshold, 6),
        "horizon_threshold_scale": round(horizon_scale, 6),
        "stop_threshold": round(stop_threshold, 6),
        "mfe": round(max(0.0, max(favorable_moves)), 6),
        "mae": round(max(0.0, max(adverse_moves)), 6),
        "raw_return": round(raw_return, 6),
        "benchmark_return": round(benchmark_return, 6),
        "alpha_return": round(raw_return - benchmark_return, 6),
    }


def _run_calibration_node(
    request: CalibrationRequest,
    *,
    ordinal: int,
    total_nodes: int,
    frame_index: int,
    frame: Any,
    safe_analysts: tuple[str, ...],
    cache_path: Path,
) -> dict[str, Any]:
    as_of = frame.index[frame_index].date()
    state: dict[str, Any] = {}
    decision: Any = None
    reused_analysis = False
    if not request.force_refresh:
        for schema_version in LEGACY_CALIBRATION_NODE_SCHEMA_VERSIONS:
            legacy_cache_path = _calibration_node_cache_path(
                request,
                as_of,
                safe_analysts,
                schema_version=schema_version,
            )
            legacy_node = _load_calibration_node(legacy_cache_path, schema_version=schema_version)
            legacy_audit = _load_calibration_audit(legacy_cache_path) if legacy_node is not None else None
            if legacy_audit is None:
                continue
            legacy_state = legacy_audit["state"]
            legacy_decision = _calibration_raw_decision(
                legacy_state,
                legacy_audit.get("raw_decision"),
                safe_analysts,
            )
            if not _legacy_calibration_decision_is_reusable(legacy_decision):
                continue
            state = legacy_state
            decision = legacy_audit.get("raw_decision")
            reused_analysis = True
            break

    node_request = None
    if not reused_analysis:
        node_run_id, should_resume = _load_or_create_calibration_run_id(cache_path)
        node_request = request.model_copy(
            update={
                "analysis_context": {
                    "calibration": {
                        "schema_version": CALIBRATION_SCHEMA_VERSION,
                        "as_of_date": as_of.isoformat(),
                        "horizon_trading_days": request.horizon_days,
                        "task": (
                            f"Forecast the price direction over the next {request.horizon_days} trading days. "
                            "Keep directional outlook separate from the current position action."
                        ),
                        "rule": "Use only information available on or before as_of_date.",
                    }
                },
                "run_id": node_run_id,
                "resume": should_resume,
                "last_event_id": 0,
            }
        )
    try:
        max_attempts = max(1, min(5, int(os.getenv("AI_CALIBRATION_NODE_MAX_ATTEMPTS", "3"))))
    except ValueError:
        max_attempts = 3
    for attempt in (range(1, max_attempts + 1) if node_request is not None else ()):
        try:
            state, decision = run_analysis(node_request, analysis_date=as_of, analysts=safe_analysts)
            break
        except Exception as exc:  # noqa: BLE001 - isolate transient LLM failures per historical node
            if attempt >= max_attempts:
                detail = user_error_message(exc, request)
                raise RuntimeError(
                    f"历史校准节点 {ordinal}/{total_nodes}（{as_of.isoformat()}）"
                    f"连续失败 {max_attempts} 次：{detail}"
                ) from exc
            node_request.resume = True
            time.sleep(min(2 ** (attempt - 1), 8))

    if len(safe_analysts) == 1:
        decision_source = safe_analysts[0]
        raw_decision = _calibration_raw_decision(state, decision, safe_analysts)
    else:
        decision_source = "portfolio_manager"
        raw_decision = state.get("final_trade_decision") or decision
    direction = normalize_directional_outlook(raw_decision)
    action = directional_outlook_action(direction)
    rating = normalize_rating(raw_decision)
    evidence_status = normalize_evidence_status(raw_decision)
    scorable = evidence_status != "INSUFFICIENT"
    score = _score_calibration_path(
        frame,
        frame_index=frame_index,
        horizon_days=request.horizon_days,
        action=action,
    )
    baseline_scores = {
        baseline_action: _compact_baseline_score(
            _score_calibration_path(
                frame,
                frame_index=frame_index,
                horizon_days=request.horizon_days,
                action=baseline_action,
            )
        )
        for baseline_action in ("BUY", "SELL", "HOLD")
    }
    if not scorable:
        score.update(
            {
                "outcome": "unscorable",
                "correct": None,
                "opportunity_outcome": "unscorable",
                "opportunity_correct": None,
                "execution_outcome": "unscorable",
                "execution_correct": None,
            }
        )
    node = {
        "ordinal": ordinal,
        "as_of_date": as_of.isoformat(),
        "action": action,
        "direction": direction,
        "rating": rating,
        "evidence_status": evidence_status,
        "scorable": scorable,
        "unscorable_reason": None if scorable else "point_in_time_evidence_insufficient",
        "decision_source": decision_source,
        "reused_analysis": reused_analysis,
        "baseline_scores": baseline_scores,
        **score,
    }
    node["audit_file"] = _store_calibration_audit(
        cache_path,
        state=state,
        raw_decision=raw_decision,
        rating=rating,
        direction=direction,
        decision_source=decision_source,
    )
    _store_calibration_node(cache_path, node)
    try:
        _calibration_pending_path(cache_path).unlink(missing_ok=True)
    except OSError:
        pass
    return node


def run_historical_calibration(
    request: CalibrationRequest,
    progress_callback: Callable[[dict[str, int]], None] | None = None,
    cancel_event: Event | None = None,
) -> dict[str, Any]:
    """Replay the configured model at ten historical cutoffs and score later alpha.

    Live social feeds and custom application-context analysts are intentionally
    excluded because they cannot be reconstructed point-in-time. Their omission
    is explicit in the Run Card instead of silently introducing look-ahead bias.
    """
    if cancel_event is not None and cancel_event.is_set():
        raise RuntimeError("用户已停止历史校准")
    frame = _calibration_price_frame(request.ticker)
    indexes = _calibration_node_indexes(frame, request.sample_count, request.horizon_days)
    selected = selected_builtin_analysts(request)
    safe_analysts = tuple(role for role in selected if role != "social")
    excluded = []
    if "social" in selected:
        excluded.append("sentiment")
    excluded.extend(str(item) for item in request.analysts if item.strip().lower() in CUSTOM_ANALYST_MAP)
    if not safe_analysts:
        raise ValueError("所选分析师都无法进行无未来数据的历史重放")
    if not request.force_refresh:
        indexes = [
            _prefer_legacy_calibration_index(frame, frame_index, request, safe_analysts)
            for frame_index in indexes
        ]

    nodes_by_ordinal: dict[int, dict[str, Any]] = {}
    pending: list[tuple[int, int, Path]] = []
    for ordinal, frame_index in enumerate(indexes, start=1):
        as_of = frame.index[frame_index].date()
        cache_path = _calibration_node_cache_path(request, as_of, safe_analysts)
        cached_node = None if request.force_refresh else _load_calibration_node(cache_path)
        if cached_node is not None:
            cached_node = dict(cached_node)
            cached_node["ordinal"] = ordinal
            nodes_by_ordinal[ordinal] = cached_node
        else:
            if request.force_refresh:
                try:
                    _calibration_pending_path(cache_path).unlink(missing_ok=True)
                except OSError:
                    pass
            pending.append((ordinal, frame_index, cache_path))

    cached_count = len(nodes_by_ordinal)
    failed_count = 0

    def report_progress() -> None:
        if progress_callback is not None:
            progress_callback(
                {
                    "completed_nodes": len(nodes_by_ordinal) + failed_count,
                    "total_nodes": len(indexes),
                    "cached_nodes": cached_count,
                    "failed_nodes": failed_count,
                }
            )

    report_progress()

    failures: list[tuple[int, Exception]] = []
    if pending:
        executor = ThreadPoolExecutor(
            max_workers=_calibration_concurrency(len(pending)),
            thread_name_prefix="ai-calibration",
        )
        futures = {
            executor.submit(
                _run_calibration_node,
                request,
                ordinal=ordinal,
                total_nodes=len(indexes),
                frame_index=frame_index,
                frame=frame,
                safe_analysts=safe_analysts,
                cache_path=cache_path,
            ): ordinal
            for ordinal, frame_index, cache_path in pending
        }
        unfinished = set(futures)
        try:
            while unfinished:
                if cancel_event is not None and cancel_event.is_set():
                    raise RuntimeError("用户已停止历史校准")
                completed, unfinished = wait(unfinished, timeout=0.25, return_when=FIRST_COMPLETED)
                for future in completed:
                    ordinal = futures[future]
                    try:
                        node = future.result()
                        nodes_by_ordinal[ordinal] = node
                        if node.get("reused_analysis"):
                            cached_count += 1
                    except Exception as exc:  # noqa: BLE001 - allow other nodes to finish and cache
                        failures.append((ordinal, exc))
                        failed_count += 1
                    report_progress()
        except Exception:
            for future in unfinished:
                future.cancel()
            executor.shutdown(wait=False, cancel_futures=True)
            raise
        else:
            executor.shutdown(wait=True)

    if failures:
        failures.sort(key=lambda item: item[0])
        first_ordinal, first_error = failures[0]
        raise RuntimeError(
            f"{len(failures)} 个历史节点失败；其他成功节点已缓存。"
            f"首个失败节点 {first_ordinal}/{len(indexes)}：{first_error}"
        ) from first_error

    nodes = [nodes_by_ordinal[ordinal] for ordinal in sorted(nodes_by_ordinal)]
    scorable_nodes = [node for node in nodes if node.get("scorable", True)]
    sample_count = len(scorable_nodes)
    correct_count = sum(int(bool(node.get("opportunity_correct", node.get("correct")))) for node in scorable_nodes)
    execution_correct_count = sum(int(bool(node.get("execution_correct"))) for node in scorable_nodes)
    opportunity_accuracy = round(correct_count / sample_count, 6) if sample_count else 0.0
    execution_accuracy = round(execution_correct_count / sample_count, 6) if sample_count else 0.0
    baselines = _calibration_baselines(scorable_nodes)
    best_opportunity_baseline = max(
        (item["opportunity_accuracy"] for item in baselines.values()),
        default=0.0,
    )
    best_execution_baseline = max(
        (item["execution_accuracy"] for item in baselines.values()),
        default=0.0,
    )
    return {
        "schema_version": CALIBRATION_SCHEMA_VERSION,
        "symbol": request.ticker.strip().upper(),
        "status": "complete",
        "sample_count": sample_count,
        "total_node_count": len(nodes),
        "unscorable_count": len(nodes) - sample_count,
        "coverage": round(sample_count / len(nodes), 6) if nodes else 0.0,
        "correct_count": correct_count,
        "accuracy": opportunity_accuracy,
        "opportunity_correct_count": correct_count,
        "opportunity_accuracy": opportunity_accuracy,
        "execution_correct_count": execution_correct_count,
        "execution_accuracy": execution_accuracy,
        "baselines": baselines,
        "opportunity_lift_vs_best_baseline": round(opportunity_accuracy - best_opportunity_baseline, 6),
        "execution_lift_vs_best_baseline": round(execution_accuracy - best_execution_baseline, 6),
        "confidence": _calibration_confidence(correct_count, sample_count, best_opportunity_baseline),
        "horizon_days": request.horizon_days,
        "benchmark": "SPY",
        "scoring_rule": "directional_outlook_with_baseline_adjusted_skill",
        "neutral_counts_as_correct": True,
        "point_in_time_analysts": list(safe_analysts),
        "excluded_analysts": sorted(set(excluded)),
        "nodes": nodes,
    }


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
    calibration = provided_data.get("calibration")
    analysis_date = date.today().isoformat()
    if isinstance(calibration, dict):
        historical_date = str(calibration.get("as_of_date") or "").strip()
        if re.fullmatch(r"\d{4}-\d{2}-\d{2}", historical_date):
            analysis_date = historical_date
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
        "analysis_date": analysis_date,
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


def normalize_rating(decision: Any) -> str:
    """Extract the explicit structured decision without scanning its thesis.

    Portfolio-manager prose often discusses rejected alternatives such as
    "do not Sell". Searching the entire document therefore turns Hold or
    Overweight decisions into false SELL signals. Prefer the rendered Rating
    field, then the Trader's Action/final proposal for legacy output.
    """
    value = str(decision or "").strip()
    patterns = (
        r"(?im)^\s*\**\s*rating\s*\**\s*[:：-]\s*\**\s*(buy|overweight|hold|underweight|sell)\b",
        r"(?im)^\s*\**\s*action\s*\**\s*[:：-]\s*\**\s*(buy|hold|sell)\b",
        r"(?im)^\s*final transaction proposal\s*[:：-]\s*\**\s*(buy|hold|sell)\b",
    )
    for pattern in patterns:
        match = re.search(pattern, value)
        if match:
            return match.group(1).upper()
    stripped = value.strip(" *.:：-").upper()
    if stripped in {"BUY", "OVERWEIGHT", "HOLD", "UNDERWEIGHT", "SELL"}:
        return stripped
    return "HOLD"


def normalize_direction(decision: Any) -> str:
    value = str(decision or "").strip()
    match = re.search(
        r"(?im)^\s*\**\s*direction\s*\**\s*[:：-]\s*\**\s*(bullish|neutral|bearish)\b",
        value,
    )
    if match:
        return {
            "bullish": "BUY",
            "neutral": "HOLD",
            "bearish": "SELL",
        }[match.group(1).lower()]
    return normalize_action(value)


def normalize_directional_outlook(decision: Any) -> str:
    """Extract a forecast label without conflating it with position action."""
    value = str(decision or "").strip()
    patterns = (
        r"(?im)^\s*\**\s*directional\s+outlook\s*\**\s*[:：-]\s*\**\s*(bullish|neutral|bearish)\b",
        r"(?im)^\s*\**\s*direction\s*\**\s*[:：-]\s*\**\s*(bullish|neutral|bearish)\b",
    )
    for pattern in patterns:
        match = re.search(pattern, value)
        if match:
            return match.group(1).upper()
    action = normalize_action(value)
    return {"BUY": "BULLISH", "SELL": "BEARISH"}.get(action, "NEUTRAL")


def directional_outlook_action(direction: str) -> str:
    return {
        "BULLISH": "BUY",
        "BEARISH": "SELL",
        "NEUTRAL": "HOLD",
    }.get(str(direction or "").upper(), "HOLD")


def normalize_evidence_status(decision: Any) -> str:
    value = str(decision or "").strip()
    patterns = (
        r"(?im)^\s*\**\s*evidence\s+status\s*\**\s*[:：-]\s*\**\s*(sufficient|insufficient)\b",
        r"(?im)^\s*\**\s*data\s+quality\s*\**\s*[:：-]\s*\**\s*(sufficient|partial|insufficient)\b",
    )
    for pattern in patterns:
        match = re.search(pattern, value)
        if match:
            return "INSUFFICIENT" if match.group(1).lower() == "insufficient" else "SUFFICIENT"
    return "SUFFICIENT"


def _calibration_raw_decision(
    state: dict[str, Any],
    fallback: Any,
    safe_analysts: tuple[str, ...],
) -> Any:
    report_fields = {
        "fundamentals": "fundamentals_report",
        "news": "news_report",
        "market": "market_report",
    }
    if len(safe_analysts) == 1:
        return state.get(report_fields.get(safe_analysts[0], "")) or fallback
    return state.get("final_trade_decision") or fallback


def _legacy_calibration_decision_is_reusable(decision: Any) -> bool:
    """Reuse old reports only when their directional meaning is unambiguous."""
    value = str(decision or "")
    if re.search(r"(?im)^\s*\**\s*(?:directional\s+outlook|direction)\s*\**\s*[:：-]", value):
        return True
    return normalize_action(value) in {"BUY", "SELL"}


def _compact_baseline_score(score: dict[str, Any]) -> dict[str, Any]:
    return {
        "opportunity_correct": bool(score.get("opportunity_correct")),
        "execution_correct": bool(score.get("execution_correct")),
    }


def _calibration_baselines(nodes: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    sample_count = len(nodes)
    summaries: dict[str, dict[str, Any]] = {}
    for action in ("BUY", "SELL", "HOLD"):
        opportunity_correct = sum(
            int(bool((node.get("baseline_scores") or {}).get(action, {}).get("opportunity_correct")))
            for node in nodes
        )
        execution_correct = sum(
            int(bool((node.get("baseline_scores") or {}).get(action, {}).get("execution_correct")))
            for node in nodes
        )
        summaries[action] = {
            "sample_count": sample_count,
            "opportunity_correct_count": opportunity_correct,
            "opportunity_accuracy": round(opportunity_correct / sample_count, 6) if sample_count else 0.0,
            "execution_correct_count": execution_correct,
            "execution_accuracy": round(execution_correct / sample_count, 6) if sample_count else 0.0,
        }
    return summaries


def normalize_action(decision: Any) -> str:
    rating = normalize_rating(decision)
    if rating in {"BUY", "OVERWEIGHT"}:
        return "BUY"
    if rating in {"SELL", "UNDERWEIGHT"}:
        return "SELL"
    return "HOLD"


def confidence_for_action(action: str, decision: Any) -> str:
    rating = normalize_rating(decision)
    if rating in {"BUY", "SELL"}:
        return "HIGH"
    if rating in {"OVERWEIGHT", "UNDERWEIGHT", "HOLD"}:
        return "MEDIUM"
    return "LOW"


def apply_historical_calibration(confidence: str, analysis_context: dict[str, Any]) -> str:
    """Use cached empirical evidence conservatively: it may downgrade, never upgrade."""
    calibration = analysis_context.get("model_calibration") if isinstance(analysis_context, dict) else None
    if not isinstance(calibration, dict) or int(calibration.get("sample_count") or 0) < 10:
        return confidence
    historical = str(calibration.get("confidence") or "").upper()
    rank = {"LOW": 0, "MEDIUM": 1, "HIGH": 2}
    if historical not in rank or confidence not in rank:
        return confidence
    return min((confidence, historical), key=lambda value: rank[value])


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


def user_error_message(exc: Exception, request: AnalysisRequest | None = None) -> str:
    message = redact_sensitive_text(raw_error_message(exc))
    payload = parse_error_payload(getattr(exc, "body", None)) or parse_error_payload(message)
    payload_code = str((payload or {}).get("code") or (payload or {}).get("type") or "")
    payload_message = redact_sensitive_text(str((payload or {}).get("message") or message))

    # Generic OpenAI-compatible clients use a placeholder for keyless local
    # servers.  A remote gateway that requires a key consequently surfaces as
    # an unhelpful SDK-level "Connection error".  Preserve keyless gateways,
    # but make the missing credential visible when that is the actual setup.
    if (
        request is not None
        and request.llm_provider.strip().lower() == "openai_compatible"
        and not model_discovery_api_key(request.llm_provider, request.api_key)
        and payload_message.rstrip(".") == "Connection error"
    ):
        return (
            "OpenAI 兼容网关未检测到 API Key。若当前网关需要鉴权，请在设置中保存与该地址匹配的 Key；"
            "若网关允许匿名调用，请检查自定义 API 地址和网络连接。"
        )

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
        confidence = apply_historical_calibration(confidence, request.analysis_context)
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
        message = user_error_message(exc, request)
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


@app.post("/quant/trend/predict")
async def quant_trend_predict(request: QuantTrendRequest):
    """Return a cached, LLM-free point-in-time trend snapshot."""

    try:
        return await asyncio.to_thread(
            quant_trend_service.predict,
            symbol=request.symbol,
            as_of=request.as_of_date,
            horizon_days=request.horizon_days,
            universe=tuple(request.universe),
            universe_version=request.universe_version,
            force_refresh=request.force_refresh,
        )
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except InsufficientDataError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except PriceDataError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/quant/trend/evaluate")
async def quant_trend_evaluate(request: QuantTrendEvaluationRequest):
    """Run a deterministic single-symbol evaluation without any model call."""

    try:
        result = await asyncio.to_thread(
            quant_trend_service.evaluate,
            symbol=request.symbol,
            start_date=request.start_date,
            end_date=request.end_date,
            horizon_days=request.horizon_days,
            step_days=request.step_days,
            maximum_nodes=request.maximum_nodes,
            transaction_cost_bps=request.transaction_cost_bps,
        )
        # The service returns a local path for the CLI, but an HTTP response
        # must not disclose the sidecar filesystem. The content-addressed run
        # ID is the stable artifact handle for future backend persistence.
        return {
            **{key: value for key, value in result.items() if key != "artifact_path"},
            "artifact_id": result.get("run_id"),
        }
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except InsufficientDataError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except PriceDataError as exc:
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


def calibration_event_stream(request: CalibrationRequest, cancel_event: Event):
    events: Queue[dict[str, Any]] = Queue()

    def publish_progress(progress: dict[str, int]) -> None:
        events.put({"type": "progress", "progress": progress})

    def execute() -> None:
        try:
            result = run_historical_calibration(request, progress_callback=publish_progress, cancel_event=cancel_event)
            events.put({"type": "result", "result": result})
        except Exception as exc:  # noqa: BLE001 - terminal stream event is redacted below
            message = user_error_message(exc, request)
            print(
                json.dumps(
                    {
                        "event": "calibration_failed",
                        "error_type": type(exc).__name__,
                        "symbol": request.ticker.strip().upper(),
                        "message": message,
                    },
                    ensure_ascii=False,
                ),
                flush=True,
            )
            events.put({"type": "error", "error": message})
        finally:
            if request.job_id:
                with _calibration_cancel_lock:
                    if _calibration_cancel_events.get(request.job_id) is cancel_event:
                        _calibration_cancel_events.pop(request.job_id, None)

    events.put(
        {
            "type": "progress",
            "progress": {"completed_nodes": 0, "total_nodes": request.sample_count, "cached_nodes": 0, "failed_nodes": 0},
        }
    )
    Thread(target=execute, name="calibration-stream", daemon=True).start()
    while True:
        event = events.get()
        yield json.dumps(event, ensure_ascii=False, default=str) + "\n"
        if event.get("type") in {"result", "error"}:
            return


@app.post("/calibrate")
async def calibrate(request: CalibrationRequest):
    try:
        request.backend_url = validate_backend_url(request.backend_url)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    cancel_event = Event()
    if request.job_id:
        with _calibration_cancel_lock:
            _calibration_cancel_events[request.job_id] = cancel_event
    return StreamingResponse(calibration_event_stream(request, cancel_event), media_type="application/x-ndjson")


@app.post("/calibrate/{job_id}/cancel")
async def cancel_calibration(job_id: str):
    with _calibration_cancel_lock:
        cancel_event = _calibration_cancel_events.get(job_id)
    if cancel_event is not None:
        cancel_event.set()
    return {"status": "stopping" if cancel_event is not None else "not_running", "job_id": job_id}


def ablation_event_stream(request: AblationStudyRequest, cancel_event: Event):
    events: Queue[dict[str, Any]] = Queue()

    def publish_progress(progress: dict[str, Any]) -> None:
        events.put({"type": "progress", "progress": progress})

    def execute() -> None:
        try:
            artifact_root = Path(
                os.getenv(
                    "AI_ABLATION_ARTIFACT_DIR",
                    str(Path(__file__).resolve().parent / "data" / "ablation-studies"),
                )
            )
            result = run_ablation_study(
                request,
                quant_service=quant_trend_service,
                run_analysis=run_analysis,
                callback_factory=EvidenceUsageCallback,
                artifact_root=artifact_root,
                repo_root=Path(__file__).resolve().parent.parent,
                progress_callback=publish_progress,
                cancel_event=cancel_event,
            )
            # The backend receives stable identifiers and the analysis, but not
            # the sidecar's absolute filesystem path.
            events.put(
                {
                    "type": "result",
                    "result": {key: value for key, value in result.items() if key != "artifact_root"},
                }
            )
        except Exception as exc:  # noqa: BLE001 - terminal evidence error is persisted by the runner
            message = user_error_message(exc, request)
            print(
                json.dumps(
                    {
                        "event": "ablation_failed",
                        "error_type": type(exc).__name__,
                        "symbol": request.ticker.strip().upper(),
                        "message": message,
                    },
                    ensure_ascii=False,
                ),
                flush=True,
            )
            events.put({"type": "error", "error": message})
        finally:
            if request.job_id:
                with _ablation_cancel_lock:
                    if _ablation_cancel_events.get(request.job_id) is cancel_event:
                        _ablation_cancel_events.pop(request.job_id, None)

    total_runs = (
        request.maximum_nodes * 2 + request.reproducibility_nodes * 2
        if request.adaptive_stages
        else request.maximum_nodes * request.repeats * 2
    )
    events.put(
        {
            "type": "progress",
            "progress": {
                "status": "preparing",
                "completed_runs": 0,
                "total_runs": total_runs,
                "cached_runs": 0,
                "failed_runs": 0,
                "independent_nodes": request.maximum_nodes,
            },
        }
    )
    Thread(target=execute, name="ablation-stream", daemon=True).start()
    while True:
        event = events.get()
        yield json.dumps(event, ensure_ascii=False, default=str) + "\n"
        if event.get("type") in {"result", "error"}:
            return


@app.post("/ablate")
async def ablate(request: AblationStudyRequest):
    try:
        request.backend_url = validate_backend_url(request.backend_url)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    if request.end_date < request.start_date:
        raise HTTPException(status_code=400, detail="end_date must not precede start_date")
    if request.adaptive_stages and (
        request.stage_one_nodes >= request.maximum_nodes
        or request.reproducibility_nodes > request.stage_one_nodes
        or request.repeats != 1
    ):
        raise HTTPException(
            status_code=400,
            detail=(
                "adaptive ablation requires repeats=1, stage_one_nodes < maximum_nodes, "
                "and reproducibility_nodes <= stage_one_nodes"
            ),
        )
    cancel_event = Event()
    if request.job_id:
        with _ablation_cancel_lock:
            _ablation_cancel_events[request.job_id] = cancel_event
    return StreamingResponse(ablation_event_stream(request, cancel_event), media_type="application/x-ndjson")


@app.post("/ablate/{job_id}/cancel")
async def cancel_ablation(job_id: str):
    with _ablation_cancel_lock:
        cancel_event = _ablation_cancel_events.get(job_id)
    if cancel_event is not None:
        cancel_event.set()
    return {"status": "stopping" if cancel_event is not None else "not_running", "job_id": job_id}


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
