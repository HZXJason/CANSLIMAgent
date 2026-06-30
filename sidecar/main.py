import asyncio
import copy
import json
import os
from datetime import date
from typing import Any, Iterable

import uvicorn
from dotenv import load_dotenv
from fastapi import FastAPI
from fastapi.responses import StreamingResponse
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


def configured_graph(request: AnalysisRequest):
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
    )


def run_analysis(request: AnalysisRequest) -> tuple[dict[str, Any], Any]:
    graph = configured_graph(request)
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


def user_error_message(exc: Exception) -> str:
    message = str(exc)
    if "RestrictedModelsError" in message or "Free tier users do not have access" in message:
        return (
            "当前 Vercel AI Gateway 免费层无法访问所选模型。"
            "请把模型改为 anthropic/claude-haiku-4.5，或在 Vercel AI Gateway 充值后再使用 Sonnet/Opus/GPT 高阶模型。"
        )
    return message


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

    loop = asyncio.get_running_loop()
    future = loop.run_in_executor(None, run_analysis, request)

    try:
        while not future.done():
            yield sse("heartbeat", {})
            await asyncio.sleep(2)

        state, decision = await future
        for agent, content in iter_reports(state):
            yield sse("progress", {"agent": agent, "status": "done", "message": "分析完成"})
            yield sse("result", {"agent": agent, "content": content})

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
