# CANSLIMAgent / AlphaLens

CANSLIMAgent 是一个面向美股 CANSLIM 研究流程的本地 Web 工作台。项目包含：

- Module 1：CANSLIM 选股器
- Module 2：个股深度分析
- Module 3：交易日志与复盘
- TradingAgents Python sidecar：多智能体 AI 分析，支持 SSE 流式输出

前端、后端和 AI 分析输出默认使用中文。

## 技术栈

- 前端：React 18、TypeScript、Zustand、TanStack Query、Tailwind CSS、Recharts
- 后端：Go、Gin、GORM、PostgreSQL
- AI Sidecar：Python、FastAPI、TradingAgents
- 数据源：FMP、Polygon、Yahoo Finance、yfinance

## 目录结构

```text
CANSLIMAgent/
├── backend/      # Go API 服务
├── frontend/     # React 前端
├── sidecar/      # TradingAgents FastAPI sidecar
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

- `backend/.env`：填写 `FMP_API_KEY`、`POLYGON_API_KEY`
- `sidecar/.env`：填写 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`GOOGLE_API_KEY`，或 `OPENAI_COMPATIBLE_API_KEY`
- `frontend/.env`：默认指向 `http://localhost:8080/api/v1`

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

TradingAgents 的报告输出语言已在 sidecar 中强制设置为中文：

```python
config["output_language"] = "Chinese"
```

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

## 常用命令

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
python -m py_compile sidecar/main.py
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

## macOS 使用说明

另一台 MacBook 可以正常使用。流程是：

1. 安装 Go、Node.js、Python、Docker Desktop
2. `git clone git@github.com:HZXJason/CANSLIMAgent.git`
3. 复制 `.env.example` 为 `.env`
4. 填入自己的 API Key
5. 按“快速启动”依次启动 PostgreSQL、后端、sidecar、前端

注意：

- API Key 不会跟随 Git 同步，需要在 MacBook 上重新填写
- `sidecar/.venv`、`frontend/node_modules` 不会提交，需要在 MacBook 上重新安装
- Windows 使用 `sidecar/start.ps1`，macOS 使用 `sidecar/start.sh`
- 如果使用 Vercel AI Gateway，MacBook 上同样填写 `OPENAI_COMPATIBLE_API_KEY` 或在前端配置面板填写即可
