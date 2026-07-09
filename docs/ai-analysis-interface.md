# AI 分析接口契约

本项目把筛选器和 AI 分析视为两个独立模块。筛选器负责股票池、CANSLIM 指标、图表数据和本地状态；AI 分析模块只通过稳定的 HTTP/SSE 契约接入。

## 边界

前端只调用 Go API：

- `GET /api/v1/analysis/ai/config`
- `POST /api/v1/analysis/{symbol}/ai`

Go API 只依赖 `services.AIAnalysisUpstream`。当前默认实现是 `HTTPAIAnalysisUpstream`，它把请求转发到 `AI_ANALYSIS_UPSTREAM_URL` 指向的上游服务；未设置时默认使用本地 TradingAgents sidecar：`http://127.0.0.1:8765/analyze`。

## 环境变量

- `AI_ANALYSIS_UPSTREAM_URL`：新的 AI 分析上游地址，可以是独立项目服务地址。若地址不以 `/analyze` 结尾，Go adapter 会自动追加。
- `AI_ANALYSIS_SIDECAR_URL`：兼容旧命名，优先级低于 `AI_ANALYSIS_UPSTREAM_URL`。
- `AI_ANALYSIS_TIMEOUT_SECONDS`：单次 AI 分析代理超时时间，默认 `1200` 秒。

## 请求

`POST /api/v1/analysis/{symbol}/ai` 的 body 是 `ai-analysis.v1` 契约：

```json
{
  "llm_provider": "openai_compatible",
  "deep_think_llm": "anthropic/claude-haiku-4.5",
  "quick_think_llm": "anthropic/claude-haiku-4.5",
  "backend_url": "https://ai-gateway.vercel.sh/v1",
  "max_debate_rounds": 1,
  "max_risk_rounds": 1,
  "online_tools": true,
  "analysts": ["fundamental", "technical"],
  "api_keys": {},
  "analysis_context": {}
}
```

Go API 会以路径中的 `{symbol}` 为准，规范化后写入上游请求的 `ticker` 字段。

## SSE 事件

上游需要返回 `text/event-stream`。前端稳定识别这些事件：

- `progress`：`{"agent":"FundamentalsAnalyst","status":"running","message":"..."}`
- `narration_delta`：`{"agent":"AnalysisAgent","content":"..."}`
- `report_start`：报告开始，前端会清空实时叙述区
- `report_delta`：`{"content":"..."}`，最终 Markdown 报告增量
- `result`：`{"agent":"TechnicalAnalyst","content":"..."}`，可选的结构化分角色结果
- `thought`：兼容旧本地流
- `report`：兼容旧本地流
- `decision`：`{"action":"HOLD","confidence":"MEDIUM","summary":"..."}`
- `done`：分析完成
- `error`：`{"message":"..."}`
- `heartbeat`：保持连接

未来的新 AI 分析项目只要保持这组事件语义，筛选器 UI 和状态管理不需要改。
