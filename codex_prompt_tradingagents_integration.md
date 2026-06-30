# Codex 实现提示词：TradingAgents 集成到 AlphaLens Module 2

## 任务概述

将 TradingAgents（`TauricResearch/TradingAgents`）以**独立 Python sidecar 进程**的方式集成到 AlphaLens 项目的 Module 2（个股分析）中。

**核心原则：对 TradingAgents 源码改动最小化，所有集成逻辑写在新文件中，不修改其内部 graph/agents/dataflows 结构。**

---

## 项目结构背景

```
alphalens/
├── backend/                  # Go 后端（已存在）
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── handlers/
│   │   │   └── analysis.go   # 已有的个股分析 handler（需扩展）
│   │   └── ...
│   └── go.mod
├── frontend/                 # React + TypeScript + shadcn/ui（已存在）
│   └── src/
│       └── pages/
│           └── StockAnalysis/ # Module 2 页面（需扩展）
└── sidecar/                  # 【新建】Python sidecar 目录
    └── (待创建)
```

---

## Task 1：创建 Python Sidecar 服务

### 目标
新建 `sidecar/` 目录，包含一个 FastAPI HTTP 服务，作为 TradingAgents 的薄包装层。Go 后端通过 HTTP 调用此服务，再将结果以 SSE 形式转发给前端。

### 文件：`sidecar/requirements.txt`

```
fastapi>=0.110.0
uvicorn[standard]>=0.29.0
tradingagents @ git+https://github.com/TauricResearch/TradingAgents.git
python-dotenv>=1.0.0
```

### 文件：`sidecar/main.py`

实现以下逻辑：

1. **启动 FastAPI 应用**，监听 `127.0.0.1:8765`（仅本地，不对外暴露）。

2. **POST `/analyze`** 接口，接收 JSON body：
   ```json
   {
     "ticker": "NVDA",
     "llm_provider": "anthropic",
     "deep_think_llm": "claude-sonnet-4-6",
     "quick_think_llm": "claude-haiku-4-5-20251001",
     "max_debate_rounds": 1,
     "max_risk_rounds": 1,
     "online_tools": true,
     "analysts": ["fundamental", "sentiment", "news", "technical"]
   }
   ```

3. **返回 `StreamingResponse`（text/event-stream）**，在流中 emit 以下格式的 SSE 事件：

   ```
   event: progress
   data: {"agent": "FundamentalsAnalyst", "status": "running", "message": "正在分析基本面数据..."}

   event: progress
   data: {"agent": "SentimentAnalyst", "status": "done", "message": "情绪分析完成"}

   event: result
   data: {"agent": "FundamentalsAnalyst", "content": "## 基本面分析\n..."}

   event: decision
   data: {"action": "BUY", "confidence": "HIGH", "summary": "..."}

   event: done
   data: {}
   ```

4. **调用 TradingAgents 的方式**：
   ```python
   from tradingagents.graph.trading_graph import TradingAgentsGraph
   from tradingagents.default_config import DEFAULT_CONFIG

   config = DEFAULT_CONFIG.copy()
   config["llm_provider"] = request.llm_provider
   config["deep_think_llm"] = request.deep_think_llm
   config["quick_think_llm"] = request.quick_think_llm
   config["max_debate_rounds"] = request.max_debate_rounds
   config["max_risk_rounds"] = request.max_risk_rounds
   config["online_tools"] = request.online_tools

   ta = TradingAgentsGraph(debug=False, config=config)
   ```

5. **流式输出处理**：TradingAgents 的 `propagate()` 是同步阻塞调用，使用 `asyncio.get_event_loop().run_in_executor(None, ...)` 在线程池中运行，避免阻塞事件循环。
   - propagate 执行期间，每 2 秒 emit 一次 `event: heartbeat` 保持连接活跃。
   - propagate 返回后，解析 `state` 和 `decision`，提取各 agent 报告内容，依次 emit `event: result`，最后 emit `event: decision` 和 `event: done`。

6. **`analysts` 字段过滤**：请求中的 `analysts` 列表决定启用哪些分析师角色。通过设置 `config` 中对应的布尔字段控制（查阅 `tradingagents/default_config.py` 中实际的字段名，例如 `use_fundamentals_analyst`、`use_sentiment_analyst` 等；若字段名不同，以实际源码为准）。

7. **`GET /health`** 接口，返回 `{"status": "ok"}`。

8. **错误处理**：任何异常在 SSE 流中以 `event: error` + `data: {"message": "..."}` 形式返回，不崩溃进程。

### 文件：`sidecar/.env.example`

```env
# TradingAgents 数据 API
FINNHUB_API_KEY=your_finnhub_key_here

# LLM Provider Keys（按需填写）
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
GOOGLE_API_KEY=

# Sidecar 监听配置
SIDECAR_HOST=127.0.0.1
SIDECAR_PORT=8765
```

### 文件：`sidecar/start.sh`

```bash
#!/bin/bash
set -e
cd "$(dirname "$0")"
if [ ! -d ".venv" ]; then
  python3 -m venv .venv
fi
source .venv/bin/activate
pip install -q -r requirements.txt
uvicorn main:app --host "${SIDECAR_HOST:-127.0.0.1}" --port "${SIDECAR_PORT:-8765}"
```

---

## Task 2：扩展 Go 后端

### 目标
在现有 Go 后端中新增两个端点，作为前端与 sidecar 之间的代理层：
1. **启动分析**：接收前端配置，调用 sidecar，将 SSE 流透传给前端
2. **查询配置**：返回可用的 LLM provider 和模型列表（静态配置）

### 文件：`backend/internal/handlers/ai_analysis.go`（新建）

实现以下两个 handler：

**`POST /api/v1/analysis/:symbol/ai`**

Request body：
```json
{
  "llm_provider": "anthropic",
  "deep_think_llm": "claude-sonnet-4-6",
  "quick_think_llm": "claude-haiku-4-5-20251001",
  "max_debate_rounds": 1,
  "max_risk_rounds": 1,
  "online_tools": true,
  "analysts": ["fundamental", "sentiment", "news", "technical"]
}
```

逻辑：
- 从路径参数取 `symbol`，合并进 body，构造对 `http://127.0.0.1:8765/analyze` 的 POST 请求
- 设置请求 `Content-Type: application/json`，响应 header `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`
- 流式读取 sidecar 响应并逐行转发给客户端（透明代理，不解析内容）
- 超时设置：读取超时 10 分钟（分析耗时较长）
- sidecar 不可达时返回 `503 Service Unavailable` + JSON 错误信息

**`GET /api/v1/analysis/ai/config`**

返回静态 JSON，列出可用的 provider 和模型选项供前端渲染配置面板：
```json
{
  "providers": [
    {
      "id": "anthropic",
      "name": "Anthropic",
      "models": {
        "deep": ["claude-opus-4-6", "claude-sonnet-4-6"],
        "quick": ["claude-sonnet-4-6", "claude-haiku-4-5-20251001"]
      }
    },
    {
      "id": "openai",
      "name": "OpenAI",
      "models": {
        "deep": ["gpt-5.4", "o4-mini"],
        "quick": ["gpt-5.4-mini", "gpt-4.1-nano"]
      }
    },
    {
      "id": "google",
      "name": "Google Gemini",
      "models": {
        "deep": ["gemini-3.0-flash-thinking", "gemini-3.0-pro"],
        "quick": ["gemini-3.0-flash", "gemini-3.1-flash"]
      }
    }
  ],
  "analysts": [
    {"id": "fundamental", "name": "基本面分析师", "description": "评估公司财务和业绩指标"},
    {"id": "sentiment", "name": "情绪分析师", "description": "聚合新闻和社交媒体情绪"},
    {"id": "news", "name": "新闻分析师", "description": "监控全球新闻和宏观指标"},
    {"id": "technical", "name": "技术分析师", "description": "基于技术指标检测交易信号"}
  ],
  "debate_rounds": [1, 2, 3],
  "risk_rounds": [1, 2, 3]
}
```

### 文件：`backend/internal/handlers/ai_analysis_test.go`（新建）

编写基础测试，mock sidecar 服务，验证：
- 正常 SSE 代理流程
- sidecar 不可达时返回 503
- symbol 路径参数正确传递

### 路由注册

在现有的路由注册文件（通常是 `backend/cmd/server/main.go` 或 `backend/internal/routes/routes.go`）中注册这两个新端点。查找现有路由注册的模式并保持一致。

---

## Task 3：React 前端配置面板

### 目标
在 Module 2 页面（`frontend/src/pages/StockAnalysis/`）的 AI 分析区域**上方**新增一个配置面板组件，允许用户在启动分析前配置 TradingAgents 参数。

### 组件：`frontend/src/components/analysis/AIAnalysisConfig.tsx`（新建）

**UI 布局（从上到下）：**

1. **顶部标题栏**：`AI 多智能体分析配置`，右侧有一个折叠/展开按钮（默认展开）

2. **LLM 配置区**（两列）：
   - 左列：`LLM 提供商` — Select 下拉（从 `/api/v1/analysis/ai/config` 加载）
   - 右列：`分析深度` — Select（`标准 / 深度`），对应 `max_debate_rounds: 1 / 2`

3. **模型配置区**（两列）：
   - 左列：`深度思考模型`（复杂推理角色使用）— Select
   - 右列：`快速响应模型`（简单任务使用）— Select
   - 两个 Select 的选项根据已选 provider 动态更新

4. **分析师选择区**：
   - 标题：`启用的分析师角色`
   - 四个 Checkbox（水平排列）：基本面分析师 / 情绪分析师 / 新闻分析师 / 技术分析师
   - 默认全选

5. **底部操作区**：
   - 左侧显示预估说明文字：`深度模式约需 5-15 分钟`
   - 右侧：`开始 AI 分析` 按钮（primary 样式，宽度固定）

**Props 接口：**
```typescript
interface AIAnalysisConfigProps {
  symbol: string
  onStartAnalysis: (config: AnalysisConfig) => void
  isAnalyzing: boolean
}

interface AnalysisConfig {
  llm_provider: string
  deep_think_llm: string
  quick_think_llm: string
  max_debate_rounds: number
  max_risk_rounds: number
  online_tools: boolean
  analysts: string[]
}
```

**状态管理：**
- 使用本地 `useState` 管理配置面板状态（不需要进 Zustand store）
- 配置项变更时通过 `localStorage` 持久化，下次打开自动恢复上次选择
- `isAnalyzing=true` 时禁用所有表单控件和按钮，按钮显示加载状态

**样式要求：**
- 使用 shadcn/ui 组件：`Card`、`Select`、`Checkbox`、`Button`、`Label`
- 整体使用 `Card` 包裹，与页面其他 Card 视觉一致
- 语言：所有标签、提示文字使用中文

### 组件：`frontend/src/components/analysis/AIAnalysisStream.tsx`（新建或扩展已有组件）

负责接收 SSE 流并渲染分析进度和结果：

1. **进度区域**（分析进行中显示）：
   - 每个 agent 一行，左侧图标（⏳运行中 / ✅完成 / ❌失败），右侧 agent 名称和状态消息
   - 顺序：基本面分析师 → 情绪分析师 → 新闻分析师 → 技术分析师 → 研究团队 → 交易员 → 风险管理 → 投资组合经理

2. **结果区域**（各 agent 完成后逐步追加）：
   - 每个 agent 结果用一个可折叠的 `Collapsible` 组件展示
   - 内容用 `ReactMarkdown` 渲染（已有依赖则复用）

3. **决策卡片**（`event: decision` 到达后显示）：
   - 突出显示最终操作建议（BUY / SELL / HOLD）
   - 配色：BUY=绿色 / SELL=红色 / HOLD=黄色

**SSE 连接实现：**
```typescript
// 使用 fetch + ReadableStream，不使用 EventSource（需要发送 POST body）
const response = await fetch(`/api/v1/analysis/${symbol}/ai`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(config),
  signal: abortController.signal,
})
// 逐行读取流并解析 SSE 格式
```

### 集成到页面

在 `frontend/src/pages/StockAnalysis/index.tsx`（或对应页面文件）中：
1. 在现有 AI 分析区域的**最顶部**插入 `<AIAnalysisConfig>` 组件
2. `onStartAnalysis` 回调触发 SSE 连接，将 stream 数据传入 `<AIAnalysisStream>`
3. 支持重新分析（显示确认 Dialog，与 Module 2 PRD 中定义的行为一致）

---

## Task 4：部署配置

### 文件：`sidecar/Dockerfile`（新建）

```dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY main.py .
EXPOSE 8765
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8765"]
```

### 文件：`docker-compose.yml`（新建或追加 sidecar service）

如果项目已有 `docker-compose.yml`，追加以下 service；否则新建：

```yaml
services:
  sidecar:
    build: ./sidecar
    ports:
      - "127.0.0.1:8765:8765"
    env_file:
      - ./sidecar/.env
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8765/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

---

## 实现顺序

1. **先实现 sidecar/main.py** — 在本地用 `curl` 测试 `/analyze` 接口是否返回正确 SSE 格式
2. **再实现 Go handler** — 验证透传代理正确性
3. **最后实现前端组件** — 确保配置面板和流式渲染正常工作

---

## 约束和注意事项

- **不要修改** `tradingagents/` 包内的任何文件（`graph/`、`agents/`、`dataflows/`、`llm_clients/`）
- **不要修改** TradingAgents 的 `default_config.py`，只在 sidecar 中 `.copy()` 后覆盖
- sidecar 只能被 Go 后端本地访问（绑定 127.0.0.1），不对外暴露
- Go 后端的 SSE 透传不解析、不修改 sidecar 返回的数据格式，保持透明代理
- `analysts` 字段过滤：实现前先读取 `tradingagents/default_config.py` 确认实际控制各分析师的配置键名，再在 sidecar 中正确设置
- TradingAgents 单次分析可能耗时 5-15 分钟，所有超时设置需覆盖这个范围
- 前端 SSE 连接使用 `fetch` + `ReadableStream`（非 `EventSource`），因为需要发送 POST body
