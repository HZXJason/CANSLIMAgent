import asyncio
import ast
import copy
import json
import os
import traceback
from queue import Empty, Queue
from datetime import date
from typing import Any, AsyncIterator, Iterable

import uvicorn
from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from langchain_core.callbacks import BaseCallbackHandler
from pydantic import BaseModel, Field

load_dotenv()

app = FastAPI(title="AlphaLens TradingAgents Sidecar")


class AnalysisRequest(BaseModel):
    ticker: str = Field(..., min_length=1)
    llm_provider: str = "anthropic"
    deep_think_llm: str = "claude-sonnet-4-6"
    quick_think_llm: str = "claude-haiku-4-5-20251001"
    backend_url: str | None = None
    max_debate_rounds: int = Field(default=1, ge=1, le=5)
    max_risk_rounds: int = Field(default=1, ge=1, le=5)
    online_tools: bool = True
    analysts: list[str] = Field(
        default_factory=lambda: ["fundamental", "sentiment", "news", "technical"]
    )
    api_keys: dict[str, str] = Field(default_factory=dict)


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
    "FundamentalsAnalyst": "基本面分析师",
    "SentimentAnalyst": "情绪分析师",
    "NewsAnalyst": "新闻分析师",
    "TechnicalAnalyst": "技术分析师",
    "ResearchTeam": "研究团队",
    "Trader": "交易员",
    "RiskManagement": "风险管理团队",
    "PortfolioManager": "投资组合经理",
}

REPORT_DELTA_CHARS = int(os.getenv("AI_REPORT_DELTA_CHARS", "3"))
REPORT_DELTA_DELAY = float(os.getenv("AI_REPORT_DELTA_DELAY", "0.012"))
NARRATION_INTERVAL_SECONDS = float(os.getenv("AI_NARRATION_INTERVAL_SECONDS", "2.5"))

LLM_API_ENV = {
    "anthropic": "ANTHROPIC_API_KEY",
    "openai": "OPENAI_API_KEY",
    "openai_compatible": "OPENAI_COMPATIBLE_API_KEY",
    "ai_gateway": "OPENAI_COMPATIBLE_API_KEY",
    "google": "GOOGLE_API_KEY",
}


def sse(event: str, data: dict[str, Any]) -> str:
    payload = json.dumps(data, ensure_ascii=False)
    return f"event: {event}\ndata: {payload}\n\n"


def text_chunks(value: str, chunk_size: int) -> Iterable[str]:
    size = max(1, chunk_size)
    for index in range(0, len(value), size):
        yield value[index : index + size]


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
        return content
    if isinstance(content, list):
        texts: list[str] = []
        for item in content:
            if isinstance(item, str):
                texts.append(item)
            elif isinstance(item, dict):
                texts.append(str(item.get("text") or item.get("content") or ""))
        return "\n".join(text for text in texts if text)
    return str(content or "")


def response_to_text(response: Any) -> str:
    chunks: list[str] = []
    for generation_group in getattr(response, "generations", []) or []:
        if not generation_group:
            continue
        generation = generation_group[0]
        message = getattr(generation, "message", None)
        if message is not None:
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
        async for event in stream_delta(
            "narration_delta",
            "\n\n" + content + "\n",
            {"agent": "Model"},
            chunk_size=REPORT_DELTA_CHARS,
            delay=REPORT_DELTA_DELAY,
        ):
            yield event


def selected_analysts(request: AnalysisRequest) -> tuple[str, ...]:
    selected: list[str] = []
    for raw in request.analysts:
        key = raw.strip().lower()
        mapped = ANALYST_MAP.get(key)
        if mapped and mapped[0] not in selected:
            selected.append(mapped[0])
    return tuple(selected or ["fundamentals", "social", "news", "market"])


def apply_request_api_keys(request: AnalysisRequest) -> None:
    for provider, env_name in LLM_API_ENV.items():
        key = (
            request.api_keys.get(provider)
            or request.api_keys.get(env_name)
            or request.api_keys.get(env_name.lower())
        )
        if key and key.strip():
            os.environ[env_name] = key.strip()
            if provider in {"openai_compatible", "ai_gateway"}:
                os.environ["AI_GATEWAY_API_KEY"] = key.strip()
    gateway_key = os.environ.get("AI_GATEWAY_API_KEY")
    if gateway_key and not os.environ.get("OPENAI_COMPATIBLE_API_KEY"):
        os.environ["OPENAI_COMPATIBLE_API_KEY"] = gateway_key


def configured_graph(request: AnalysisRequest, callbacks: list[Any] | None = None):
    apply_request_api_keys(request)

    from tradingagents.default_config import DEFAULT_CONFIG
    from tradingagents.graph.trading_graph import TradingAgentsGraph

    config = copy.deepcopy(DEFAULT_CONFIG)
    config["llm_provider"] = request.llm_provider
    config["deep_think_llm"] = request.deep_think_llm
    config["quick_think_llm"] = request.quick_think_llm
    config["backend_url"] = request.backend_url.strip() if request.backend_url else None
    config["max_debate_rounds"] = request.max_debate_rounds
    config["max_risk_discuss_rounds"] = request.max_risk_rounds
    config["max_risk_rounds"] = request.max_risk_rounds
    config["online_tools"] = request.online_tools
    config["output_language"] = "Chinese"

    return TradingAgentsGraph(
        selected_analysts=selected_analysts(request),
        debug=False,
        config=config,
        callbacks=callbacks,
    )


def run_analysis(request: AnalysisRequest, callbacks: list[Any] | None = None) -> tuple[dict[str, Any], Any]:
    graph = configured_graph(request, callbacks=callbacks)
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
            yield agent, str(value)


def report_heading(agent: str) -> str:
    return AGENT_DISPLAY_NAMES.get(agent, agent)


def compose_report(state: dict[str, Any]) -> str:
    sections: list[str] = []
    for agent, content in iter_reports(state):
        normalized = content.strip()
        if normalized:
            sections.append(f"## {report_heading(agent)}\n\n{normalized}")
    return "\n\n".join(sections)


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
    message = raw_error_message(exc)
    payload = parse_error_payload(getattr(exc, "body", None)) or parse_error_payload(message)
    payload_code = str((payload or {}).get("code") or (payload or {}).get("type") or "")
    payload_message = str((payload or {}).get("message") or message)

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


async def analysis_events(request: AnalysisRequest):
    selected_keys = set(selected_analysts(request))
    emitted_agents: list[str] = []
    for mapped_key, agent, message in ANALYST_MAP.values():
        if mapped_key in selected_keys and agent not in emitted_agents:
            emitted_agents.append(agent)
            yield sse("progress", {"agent": agent, "status": "running", "message": message})

    for agent, message in [
        ("ResearchTeam", "研究团队正在展开多空辩论..."),
        ("Trader", "交易员正在生成投资计划..."),
        ("RiskManagement", "风险管理团队正在评估仓位与回撤..."),
        ("PortfolioManager", "投资组合经理正在形成最终建议..."),
    ]:
        yield sse("progress", {"agent": agent, "status": "pending", "message": message})

    model_output_queue: Queue[str] = Queue()
    callbacks = [StreamingCallback(model_output_queue)]
    loop = asyncio.get_running_loop()
    future = loop.run_in_executor(None, run_analysis, request, callbacks)

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

        report = compose_report(state)
        if report:
            yield sse("report_start", {})
            async for event in stream_delta(
                "report_delta",
                report,
                chunk_size=REPORT_DELTA_CHARS,
                delay=REPORT_DELTA_DELAY,
            ):
                yield event

        action = normalize_action(decision)
        summary = str(state.get("final_trade_decision") or decision or "")
        yield sse(
            "decision",
            {
                "action": action,
                "confidence": confidence_for_action(action, decision),
                "summary": summary,
            },
        )
        yield sse("done", {})
    except Exception as exc:
        print("AI analysis stream failed:\n" + traceback.format_exc(), flush=True)
        yield sse("error", {"message": user_error_message(exc)})


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/analyze")
async def analyze(request: AnalysisRequest):
    headers = {
        "Cache-Control": "no-cache",
        "Connection": "keep-alive",
        "X-Accel-Buffering": "no",
    }
    return StreamingResponse(
        analysis_events(request),
        media_type="text/event-stream",
        headers=headers,
    )


if __name__ == "__main__":
    uvicorn.run(
        "main:app",
        host=os.getenv("SIDECAR_HOST", "127.0.0.1"),
        port=int(os.getenv("SIDECAR_PORT", "8765")),
    )
