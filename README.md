# CANSLIMAgent

CANSLIMAgent 是一个面向美股研究的本地优先工作台：从 CANSLIM 选股、个股证据核查，到多智能体研究、数据审计、交易计划和交易复盘，形成一条可复查的人机协作流程。它是研究决策支持工具，不会自动下单，也不构成投资建议或收益承诺。

界面与 AI 报告默认使用中文；数据、推理过程和最终动作都应由使用者自行复核。

## 核心能力

- 扫描 S&P 500、Nasdaq 100 与 Russell 2000 代理股票池，并按 CANSLIM 指标筛选。
- 展示价格、相对强度、EPS、营收、毛利率、公司概况与机构持仓等研究证据。
- 运行基本面、情绪、新闻、技术、看多/看空、交易员、风控与投资组合经理组成的多智能体分析。
- 在分析前通过 Vibe-Trading MCP 预取申报文件、财务报表、行情、新闻、期权和公司资料。
- 使用确定性的 `DataAuditor` 按来源、申报类型、报告期间和 accession 审计冲突数据；可验证事实进入分析上下文，期间不明或冲突的数值会被阻止引用并明确显示为未知。
- 在投资组合经理初步结论后执行 Vibe 历史执行验证。完成但统计证据不足的验证可将 BUY/SELL 拦截为 HOLD；无法执行时会如实标记为“未验证”，不会伪造成功结果。
- 记录交易日志、生成 `vibe.trade_record.v1` 兼容导出，并给出确定性的交易行为复盘。
- 通过 SSE 实时显示进度和报告，支持暂停/续传，并可将完整 AI 报告另存为 Markdown。

## 架构

```text
React + TypeScript
        │ REST / SSE
        ▼
Go API（Gin） ── PostgreSQL
        │
        ▼
Python sidecar（FastAPI）
  ├─ 内置 TradingAgents 图
  ├─ 内置 Vibe 桥接层
  ├─ DataAuditor
  └─ Vibe MCP / 独立的 Vibe 回测运行时
```

浏览器只访问 Go 服务的 `/api/v1`；Go 服务再代理到可替换的 Python sidecar（`AI_ANALYSIS_UPSTREAM_URL`）。因此前端、API 与 AI 编排解耦，同时保持流式事件契约稳定。

项目拥有的 AI 源码均在 `sidecar/`：

- `sidecar/main.py`：编排、审计接入与 SSE 生产者。
- `sidecar/ai_runtime.py`：内置运行时和 Vibe 工具加载。
- `sidecar/vendor/tradingagents/`：随项目提交的 TradingAgents 图运行时。
- `sidecar/vendor/ta_vibe_bridge/`：随项目提交的 MCP 与回测桥接层。
- `sidecar/data_auditor.py`：来源与期间感知的数据审计。
- `sidecar/vibe_decision_flow.py`：证据协调与执行验证门。

`Vibe-Trading` 本身保持为独立运行时：其 MCP 服务与回测引擎有单独的依赖栈。仅在启用 Vibe 工具、执行验证或日志复盘时需要一个本地 checkout，并通过 `VIBE_TRADING_ROOT` 指定；核心 AI 编排与桥接源码不再依赖 `TradingAgent2.0` 目录。

## AI 研究流程

```text
Vibe MCP 证据预取
  → DataAuditor（来源、表单、期间、accession）
  → 分析师团队
  → 看多 / 看空研究
  → 交易员与风控审阅
  → 投资组合经理初步结论
  → Vibe 证据协调
  → Vibe 历史执行验证
  → 面向人工的最终动作
```

DataAuditor 不是第二个会“猜测赢家”的 LLM。例如，一条没有 XBRL 期间长度的简化 10-K 数值，不能覆盖可追溯的 SEC 年度序列。冲突与排除原因会保留在输出中，歧义值不会进入分析师提示词。

## 技术栈与要求

- 前端：React、TypeScript、Vite、Zustand、TanStack Query、Tailwind CSS。
- 后端：Go 1.22+、Gin、PostgreSQL 15。
- AI sidecar：Python 3.12+、FastAPI、LangGraph、TradingAgents；推荐用 `uv` 管理 Python 环境。
- 运行环境：macOS 或 Linux、Node.js 20+、Docker Desktop。
- AI 分析：至少配置一种受支持的 LLM API Key。

## 快速启动

```bash
git clone git@github.com:HZXJason/CANSLIMAgent.git
cd CANSLIMAgent

cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
cp sidecar/.env.example sidecar/.env

make dev
```

打开 `http://127.0.0.1:5173`。`make dev` 会在服务未运行时启动 Docker PostgreSQL、Python sidecar、Go API 和 Vite 前端。

## 配置

本地配置不会提交到 Git：

| 文件 | 用途 |
|---|---|
| `backend/.env` | 数据库、免费/可选行情源与 AI 上游地址 |
| `sidecar/.env` | LLM 凭据、Vibe 运行时位置与 AI 开关 |
| `frontend/.env` | 浏览器 API 基地址 |

关键 sidecar 配置：

```dotenv
# 只填写实际使用的模型供应商
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
GOOGLE_API_KEY=
OPENAI_COMPATIBLE_API_KEY=

# 独立的 Vibe-Trading 运行时；未设置时仅兼容旧的同级安装
VIBE_TRADING_ROOT=/absolute/path/to/Vibe-Trading
ENABLE_VIBE_MCP_TOOLS=true

# 在投资组合经理初步建议之后执行历史验证
ENABLE_VIBE_EXECUTION_VALIDATION=true
VIBE_EXECUTION_VALIDATION_TIMEOUT_SECONDS=90
```

Vibe 功能是可选的。禁用或不可用时，系统会明确记录能力降级，而不会生成替代证据。

## 数据策略

项目坚持免费优先和可追溯：

- 财报与申报：SEC EDGAR 优先。
- 价格与图表：Stooq、Yahoo Finance 与 yfinance 兜底；FMP/Polygon 可选增强。
- 公司资料、期权与新闻：可用时使用公开的 Yahoo 衍生路线。
- 机构持仓：Nasdaq 的逐股票持仓数据。

来源失败、报告日期和数据不足会显示给用户；缺失数据不会被当作 0 或确认信号。

## 主要 API

```text
GET   /api/v1/health
GET   /api/v1/screener/scan
GET   /api/v1/screener/results
POST  /api/v1/screener/refresh
GET   /api/v1/stocks/:symbol/summary
GET   /api/v1/stocks/:symbol/profile
GET   /api/v1/stocks/:symbol/kline
GET   /api/v1/stocks/:symbol/rs
GET   /api/v1/stocks/:symbol/eps/quarterly
GET   /api/v1/stocks/:symbol/revenue/annual
POST  /api/v1/analysis/:symbol/ai
GET   /api/v1/analysis/ai/config
POST  /api/v1/analysis/ai/models
GET   /api/v1/journal
POST  /api/v1/journal
GET   /api/v1/journal/export
POST  /api/v1/journal/reflections
GET   /api/v1/trade-plans
POST  /api/v1/trade-plans
```

AI 分析会转发 `progress`、`result`、`report_start`、`report_delta`、`decision`、`done` 和 `error` 等 SSE 事件。

## 验证

在仓库根目录运行：

```bash
cd backend && go test ./...
cd ../sidecar && .venv/bin/python -m unittest test_main.py test_ai_runtime.py test_journal_reflection.py test_analysis_runs.py
cd ../frontend && pnpm run typecheck && pnpm run build
docker compose build sidecar
```

基本健康检查：

```bash
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8765/health
```

## 仓库约定

提交的内容只包括运行源码、测试、配置示例、必要的第三方许可证和本 README。`frontend/dist`、Python `__pycache__`、本地虚拟环境、分析 checkpoint、`.env` 与其他生成文件均不纳入版本控制。
