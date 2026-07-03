# AlphaLens / CANSLIMAgent

AlphaLens / CANSLIMAgent 是一个面向美股 CANSLIM 研究流程的本地 Web 工作台。它把股票池扫描、指标缓存、个股研究、AI 多智能体分析、N 标签写回和交易日志放在一个可重复运行的本地环境里，定位是 human-in-the-loop 的投资研究决策支持工具，而不是自动交易系统或收益承诺工具。

前端、后端和 AI 分析输出默认使用中文。

## 核心能力

- Module 1：CANSLIM 选股器，支持 SP500、Nasdaq 100、Russell 2000 代理股票池扫描
- Module 2：个股深度分析，覆盖价格 K 线、RS、EPS、营收、毛利率等研究指标
- Module 3：交易日志与复盘，用于记录观察、操作与后续复盘
- TradingAgents Python sidecar：多智能体 AI 分析，支持 SSE 流式输出、中文报告和 N 标签写回
- 免费优先数据源：SEC EDGAR、公开股票池、Stooq、Yahoo Finance、yfinance 优先，FMP/Polygon 作为可选增强
- 可复用本地开发：`make dev` / `scripts/dev.sh` 一键启动 PostgreSQL、Go API、Python sidecar 和 Vite 前端

## 技术栈

- 前端：React 18、TypeScript、Zustand、TanStack Query、Tailwind CSS、Recharts
- 后端：Go、Gin、GORM、PostgreSQL
- AI Sidecar：Python、FastAPI、TradingAgents
- 数据源：SEC EDGAR、免费股票池、Stooq、Yahoo Finance、yfinance；FMP/Polygon 可作为可选增强

## 架构

项目采用本地优先的四层结构：

- React 前端工作台：承载选股器、个股分析、AI 流式报告、参数设置和交易日志
- Go API 层：提供 REST / SSE 接口、股票池扫描、指标聚合、AI sidecar 代理和日志接口
- PostgreSQL 数据层：缓存股票、指标、筛选阈值、AI 写回结果和交易日志
- Python AI sidecar：封装 TradingAgents 多智能体流程，并把进度、叙述和报告分段推回前端

组合架构图见 [docs/portfolio-architecture.md](docs/portfolio-architecture.md)。

## 目录结构

```text
CANSLIMAgent/
├── backend/      # Go API 服务
├── docs/         # 项目架构与作品集图
├── frontend/     # React 前端
├── scripts/      # 本地开发启动脚本
├── sidecar/      # TradingAgents FastAPI sidecar
├── Makefile      # make dev 入口
└── docker-compose.yml
```

## 环境要求

- Go 1.22+
- Node.js 20+
- Python 3.10+
- Docker / Docker Compose
- PostgreSQL 15（可直接使用本项目的 docker-compose）

macOS、Windows 都可以运行。macOS 上建议用 Homebrew 安装 Go、Node、Python，并使用 `python3`/`pip3`。

## 快速启动

### 一键开发启动

```bash
make dev
```

等价于：

```bash
./scripts/dev.sh
```

脚本会启动 Docker PostgreSQL，并复用本机 Go、Node、uv 管理的 Python sidecar 环境。若某个服务已经在本机运行，脚本会复用它而不是重复启动。服务启动后默认地址：

- 前端：`http://127.0.0.1:5173/`
- 后端：`http://localhost:8080`
- Sidecar：`http://127.0.0.1:8765`

按 `Ctrl+C` 会停止本次脚本启动的本地服务；Docker PostgreSQL 默认保留运行。若希望退出时也停止数据库：

```bash
STOP_DOCKER_ON_EXIT=1 make dev
```

### 1. 克隆项目

```bash
git clone git@github.com:HZXJason/CANSLIMAgent.git
cd CANSLIMAgent
```

### 2. 准备环境变量

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
cp sidecar/.env.example sidecar/.env
```

按需编辑：

- `backend/.env`：默认启用免费数据源；如需增强行情可填写 `FMP_API_KEY`、`POLYGON_API_KEY`
- `sidecar/.env`：填写 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`GOOGLE_API_KEY`，或 `OPENAI_COMPATIBLE_API_KEY`
- `frontend/.env`：默认指向 `http://localhost:8080/api/v1`

关键后端开关：

- `ENABLE_FREE_UNIVERSE=true`：启用免费股票池
- `ENABLE_SEC_DATA=true`：启用 SEC EDGAR 财务数据
- `ENABLE_STOOQ_DATA=true`：启用 Stooq 价格/K 线数据
- `ENABLE_YAHOO_FALLBACK=true`、`ENABLE_YFINANCE_FALLBACK=true`：保留 Yahoo/yfinance 兜底
- `REQUEST_TIMEOUT_SECONDS=20`、`SCAN_DELAY_MS=80`：控制外部请求超时和扫描节奏

后端默认数据源优先级：

- 成分股：免费股票池（SP500 CSV、Nasdaq 100 JSON、IWM 持仓代理 Russell 2000、Nasdaq Trader 目录补充） -> FMP -> yfinance/Wikipedia
- 财报/EPS/营收/毛利率：SEC EDGAR -> FMP -> yfinance
- 价格/K 线：Stooq -> Polygon -> FMP -> Yahoo Finance -> yfinance

建议在 `backend/.env` 设置 `SEC_USER_AGENT`，写成能联系到你的格式，例如 `CANSLIMAgent/1.0 your-email@example.com`。
如果 Stooq 在当前网络返回浏览器验证页，后端会在本次运行中自动跳过它；也可以设置 `ENABLE_STOOQ_DATA=false`。
罗素 2000 使用 BlackRock/iShares IWM 持仓下载作为免费股票池代理；这不是 FTSE Russell 官方授权成分股文件，适合本地研究筛选，不适合当作正式指数授权数据。

环境变量文件不会提交到 Git。

### 3. 启动 PostgreSQL

```bash
docker compose up -d postgres
```

### 4. 启动后端

```bash
cd backend
go mod download
go run ./cmd/server
```

默认地址：

```text
http://localhost:8080
```

### 5. 启动 AI sidecar

Windows PowerShell：

```powershell
cd sidecar
.\start.ps1
```

macOS / Linux：

```bash
cd sidecar
chmod +x start.sh
./start.sh
```

默认地址：

```text
http://127.0.0.1:8765
```

健康检查：

```bash
curl http://127.0.0.1:8765/health
```

### 6. 启动前端

```bash
cd frontend
npm install
npm run dev
```

默认地址：

```text
http://localhost:5173
```

## AI 多智能体分析

个股分析页顶部有“AI 多智能体分析配置”面板，可配置：

- LLM 提供商
- 深度思考模型
- 快速响应模型
- 分析深度
- 分析师角色
- API Key
- OpenAI-compatible 自定义 API 地址

分析过程会通过 SSE 持续输出：

- `progress`：当前智能体或阶段状态
- `narration_delta`：等待期间的增量叙述，避免页面长时间空白
- `report_start` / `report_delta`：最终研究报告分段输出
- `decision`：最终动作、摘要和 N 标签写回信息
- `done`：本次分析结束

TradingAgents 的报告输出语言已在 sidecar 中强制设置为中文：

```python
config["output_language"] = "Chinese"
```

AI 结论会写回股票的 `N` 标签。选股器工具栏可以直接筛选 `N`，用于把 AI 识别出的“新高、新闻、新产品、新管理层、新产业变化”等线索纳入后续人工复核。

### Vercel AI Gateway

选择 `OpenAI 兼容网关` 时，默认 API 地址为：

```text
https://ai-gateway.vercel.sh/v1
```

Vercel AI Gateway 使用 OpenAI Chat Completions 兼容接口，模型名通常是：

```text
anthropic/claude-haiku-4.5
google/gemini-3.1-flash-lite-preview
openai/...
```

免费层可能无法使用 Sonnet、Opus、GPT 高阶模型。如果遇到 403 `Free tier users do not have access to this model`，请换成：

```text
anthropic/claude-haiku-4.5
```

或给 Vercel AI Gateway 充值后再使用受限模型。

## 验证命令

后端测试：

```bash
cd backend
go test ./...
```

前端类型检查：

```bash
cd frontend
npm run typecheck
```

前端构建：

```bash
cd frontend
npm run build
```

sidecar 语法检查：

```bash
python3 -m py_compile sidecar/main.py
```

本地服务健康检查：

```bash
curl http://localhost:8080/api/v1/health
curl http://127.0.0.1:8765/health
```

## 关键 API

- `GET /api/v1/health`
- `GET /api/v1/screener/scan?market=SP500`
- `GET /api/v1/screener/results?market=SP500`
- `POST /api/v1/screener/refresh`
- `GET /api/v1/stocks/:symbol/summary`
- `GET /api/v1/stocks/:symbol/rs`
- `GET /api/v1/stocks/:symbol/eps/quarterly`
- `GET /api/v1/stocks/:symbol/revenue/annual`
- `GET /api/v1/stocks/:symbol/margin/gross/quarterly`
- `PATCH /api/v1/stocks/:symbol/n-label`
- `GET /api/v1/analysis/ai/config`
- `POST /api/v1/analysis/:symbol/ai`
- `GET /api/v1/journal`
- `POST /api/v1/journal`
- `PATCH /api/v1/journal/:id`

## Docker

当前 `docker-compose.yml` 包含 PostgreSQL 和 sidecar：

```bash
docker compose up -d postgres sidecar
```

运行 sidecar 前请先准备：

```bash
cp sidecar/.env.example sidecar/.env
```

## 跨机器使用说明

换到另一台 Mac、Windows 或 Linux 机器时，建议按下面流程恢复：

1. 安装 Go、Node.js、Python、Docker Desktop
2. `git clone git@github.com:HZXJason/CANSLIMAgent.git`
3. 复制 `.env.example` 为 `.env`
4. 填入自己的 API Key 和 `SEC_USER_AGENT`
5. 优先运行 `make dev`；如果需要排错，再按“快速启动”中的分步命令启动 PostgreSQL、后端、sidecar、前端

注意：

- API Key 不会跟随 Git 同步，需要在目标机器上重新填写
- `sidecar/.venv`、`frontend/node_modules` 不会提交，需要在目标机器上重新安装
- Windows 使用 `sidecar/start.ps1`，macOS 使用 `sidecar/start.sh`
- 如果使用 Vercel AI Gateway，目标机器上同样填写 `OPENAI_COMPATIBLE_API_KEY` 或在前端配置面板填写即可

## 使用边界

本项目用于研究流程、数据整理、AI 辅助分析和交易复盘。AI 输出和筛选结果都应由使用者复核，不构成投资建议，也不代表自动交易信号。
