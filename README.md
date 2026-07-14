# CANSLIMAgent

CANSLIMAgent 是一个面向美股研究的本地优先工作台：从 CANSLIM 选股、个股证据核查，到多智能体研究、数据审计、交易计划和交易复盘，形成一条可复查的人机协作流程。它是研究决策支持工具，不会自动下单，也不构成投资建议或收益承诺。

界面与 AI 报告默认使用中文；数据、推理过程和最终动作都应由使用者自行复核。

## 核心能力

- 扫描 S&P 500、Nasdaq 100 与 Russell 2000 代理股票池，并按 CANSLIM 指标筛选。
- 展示价格、相对强度、EPS、营收、毛利率、公司概况与机构持仓等研究证据。
- 运行基本面、情绪、新闻、技术、看多/看空、交易员、风控与投资组合经理组成的多智能体分析。
- 在分析前通过 Vibe-Trading MCP 预取申报文件、财务报表、行情、新闻、期权和公司资料。
- 使用确定性的 `DataAuditor` 按来源、申报类型、报告期间和 accession 审计冲突数据；可验证事实进入分析上下文，期间不明或冲突的数值会被阻止引用并明确显示为未知。
- 记录交易日志、生成 `vibe.trade_record.v1` 兼容导出，并给出确定性的交易行为复盘。
- 可在设置中连接长桥、老虎、富途 OpenD 或 IBKR 本地 API，会先预览只读成交，再标准化导入交易日志；不提供自动下单。
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
  └─ Vibe MCP / 独立的交易日志复盘运行时
```

浏览器只访问 Go 服务的 `/api/v1`；Go 服务再代理到可替换的 Python sidecar（`AI_ANALYSIS_UPSTREAM_URL`）。因此前端、API 与 AI 编排解耦，同时保持流式事件契约稳定。

项目拥有的 AI 源码均在 `sidecar/`：

- `sidecar/main.py`：编排、审计接入与 SSE 生产者。
- `sidecar/ai_runtime.py`：内置运行时和 Vibe 工具加载。
- `sidecar/vendor/tradingagents/`：随项目提交的 TradingAgents 图运行时。
- `sidecar/vendor/ta_vibe_bridge/`：随项目提交的 MCP 桥接层。
- `sidecar/data_auditor.py`：来源与期间感知的数据审计。
- `sidecar/vibe_decision_flow.py`：证据协调。

`Vibe-Trading` 本身保持为独立运行时：其 MCP 服务与交易日志复盘工具有单独的依赖栈。仅在启用 Vibe 工具或日志复盘时需要一个本地 checkout，并通过 `VIBE_TRADING_ROOT` 指定；核心 AI 编排与桥接源码不再依赖 `TradingAgent2.0` 目录。

## AI 研究流程

```text
Vibe MCP 证据预取
  → DataAuditor（来源、表单、期间、accession）
  → 分析师团队
  → 看多 / 看空研究
  → 交易员与风控审阅
  → 投资组合经理初步结论
  → Vibe 证据协调
  → 面向人工的最终动作
```

DataAuditor 不是第二个会“猜测赢家”的 LLM。例如，一条没有 XBRL 期间长度的简化 10-K 数值，不能覆盖可追溯的 SEC 年度序列。冲突与排除原因会保留在输出中，歧义值不会进入分析师提示词。

## 技术栈与要求

- 前端：React、TypeScript、Vite、Zustand、TanStack Query、Tailwind CSS。
- 后端：Go 1.25+（发布构建固定 toolchain 1.26.5）、Gin、PostgreSQL 15。
- AI sidecar：Python 3.12+、FastAPI、LangGraph、TradingAgents；推荐用 `uv` 管理 Python 环境。
- 运行环境：macOS 或 Linux、Node.js 22+、Docker Desktop。
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
| `frontend/.env` | 可选的浏览器 API 基地址；默认使用同源 `/api/v1` |

启用券商绑定前，在 `backend/.env` 生成并保存一把只属于本机的加密主密钥：

```bash
openssl rand -base64 32
```

将结果写入 `BROKER_CREDENTIALS_ENCRYPTION_KEY`。没有这把 32 字节密钥，后端会拒绝保存任何券商凭据。凭据从不发送给浏览器、不会被 API 回显，且仅在一次同步期间通过本机 Go → sidecar 调用使用。长桥使用 OAuth access token；富途和 IBKR 连接用户已经登录的本地 OpenD / TWS 或 IB Gateway；老虎使用其开放平台凭据。

本机开发时券商 sidecar 固定为 `BROKER_SYNC_UPSTREAM_URL=http://127.0.0.1:8765`。容器部署只允许带 32 字符以上共享密钥的固定 `sidecar` 服务名，浏览器不能提供或覆盖该地址。

### 券商授权接入前检查清单

1. 在 `backend/.env` 生成并设置 `BROKER_CREDENTIALS_ENCRYPTION_KEY`；没有它，系统不会保存任何连接凭据。
2. 分别在长桥或老虎开放平台注册**服务器端 OAuth 开发者应用**。默认本地 API 的回调地址必须逐字登记为：

   ```text
   http://127.0.0.1:8080/api/v1/brokers/oauth/longport/callback
   http://127.0.0.1:8080/api/v1/brokers/oauth/tiger/callback
   ```

   若修改 API 的 `PORT` 或部署地址，请同时将 `BROKER_*_REDIRECT_URL` 和开发者平台中登记的回调地址改成同一个 HTTPS/HTTP URL。
3. 仅在 `backend/.env` 中填入 OAuth 服务器配置。长桥和老虎都需要各自的 `CLIENT_ID`、`CLIENT_SECRET`、`AUTHORIZE_URL`、`TOKEN_URL`、`ACCOUNT_URL`、`REDIRECT_URL` 与 `SCOPES`：

   ```dotenv
   BROKER_LONGPORT_CLIENT_ID=
   BROKER_LONGPORT_CLIENT_SECRET=
   BROKER_LONGPORT_AUTHORIZE_URL=
   BROKER_LONGPORT_TOKEN_URL=
   BROKER_LONGPORT_ACCOUNT_URL=
   BROKER_LONGPORT_REDIRECT_URL=
   BROKER_LONGPORT_SCOPES=

   BROKER_TIGER_CLIENT_ID=
   BROKER_TIGER_CLIENT_SECRET=
   BROKER_TIGER_AUTHORIZE_URL=
   BROKER_TIGER_TOKEN_URL=
   BROKER_TIGER_ACCOUNT_URL=
   BROKER_TIGER_REDIRECT_URL=
   BROKER_TIGER_SCOPES=
   ```

   `SCOPES` 以逗号分隔，必须至少包含一项明确的只读权限；不要留空，也不要填入下单、交易或写入权限。它在 OAuth 启动时单独接受只读范围校验。
4. 使用富途前，在本机启动并登录 OpenD；使用 IBKR 前，在本机启动并登录 TWS 或 IB Gateway。默认本机网关配置为：

   ```dotenv
   BROKER_FUTU_HOST=127.0.0.1
   BROKER_FUTU_PORT=11111
   BROKER_FUTU_ACCOUNT=
   BROKER_IBKR_HOST=127.0.0.1
   BROKER_IBKR_PORT=7497
   BROKER_IBKR_CLIENT_ID=9901
   BROKER_IBKR_ACCOUNT=
   ```

   后端只允许 loopback 网关地址；不要将其改成远程主机。
5. 浏览器只接收服务端生成的授权跳转，并在回调完成后刷新连接状态。客户端密钥、OAuth access token、私钥、OpenD/TWS 网关地址或账户密码都绝不能放进浏览器、`frontend/.env` 或前端表单。

长桥和老虎的 OAuth 卡片始终会显示，且不会因为服务端 OAuth 应用尚未配置而预先禁用。点击“连接账户”后，若缺少 `CLIENT_ID`、`CLIENT_SECRET` 或任一 OAuth 端点/回调 URL 配置，服务端会返回 HTTP 409 和 `OAuth 开发者应用尚未配置`，前端会显示该配置错误；不会退回为让用户在浏览器粘贴令牌或私钥。

`*_SCOPES` 不参与上述开发者应用是否已配置的判定：即使客户端与端点配置齐全，空的或不符合只读规则的 scope 仍会在 OAuth 启动时被拒绝，并不保证返回该 409 配置错误。请改为至少一项显式只读 scope 后再重试。富途和 IBKR 则只检查已经登录的本机 loopback 网关。若本机发现多个账户，请在服务器环境变量中设置对应的 `BROKER_FUTU_ACCOUNT` 或 `BROKER_IBKR_ACCOUNT`；这些配置只会传给本机 sidecar，不会发送到浏览器。

LLM Key 可以直接在设置页填写。Go 后端会使用 AES-GCM 加密后存入 PostgreSQL，前端只能读取“已配置/未配置”状态，不能取回原始 Key。下面的 sidecar 环境变量仍可作为无人值守部署的后备配置：

```dotenv
# 只填写实际使用的模型供应商
OPENAI_API_KEY=
ANTHROPIC_API_KEY=
GOOGLE_API_KEY=
OPENAI_COMPATIBLE_API_KEY=

# 独立的 Vibe-Trading MCP / 交易日志复盘运行时；未设置时仅兼容旧的同级安装
VIBE_TRADING_ROOT=/absolute/path/to/Vibe-Trading
ENABLE_VIBE_MCP_TOOLS=true
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
GET   /api/v1/llm/credentials/:provider
PUT   /api/v1/llm/credentials/:provider
DELETE /api/v1/llm/credentials/:provider
GET   /api/v1/journal
POST  /api/v1/journal
GET   /api/v1/journal/export
POST  /api/v1/journal/reflections
GET   /api/v1/trade-plans
POST  /api/v1/trade-plans
GET   /api/v1/brokers
POST  /api/v1/brokers/connect/:broker
POST  /api/v1/brokers/:id/sync
POST  /api/v1/brokers/:id/import
DELETE /api/v1/brokers/:id
```

AI 分析会转发 `progress`、`result`、`report_start`、`report_delta`、`decision`、`done` 和 `error` 等 SSE 事件。设置页会通过 Go → sidecar 自动读取当前供应商可用模型；用户填写的 LLM Key 由 Go 加密保存，按提供商及自定义网关主机隔离，并只在一次固定 Go → sidecar 请求期间解密使用。

本地开发允许设置页连接经过校验的公网 HTTPS 自定义网关；仍会拒绝内网、loopback、凭据型 URL 和重定向。生产环境始终强制 `AI_ALLOWED_BACKEND_HOSTS` 精确白名单；开发环境也可设置 `AI_REQUIRE_BACKEND_ALLOWLIST=true` 使用同一策略。

## 验证

在仓库根目录运行：

```bash
cd backend && go test ./...
cd ../sidecar && .venv/bin/python -m unittest test_main.py test_ai_runtime.py test_journal_reflection.py test_analysis_runs.py
cd ../frontend && pnpm run typecheck && pnpm run build
docker compose build sidecar
```

## 生产部署安全边界

`docker-compose.prod.yml` 提供非 root、最小权限、内部网络部署基线。先复制 `.env.production.example` 并生成三类彼此独立的随机秘密：

```bash
cp .env.production.example .env.production
openssl rand -hex 32   # API_AUTH_TOKEN
openssl rand -hex 32   # SIDECAR_SHARED_SECRET
openssl rand -base64 32 # BROKER_CREDENTIALS_ENCRYPTION_KEY
openssl rand -base64 32 # LLM_CREDENTIALS_ENCRYPTION_KEY
docker compose --env-file .env.production -f docker-compose.prod.yml up --build -d
```

容器入口仅绑定到宿主机 `127.0.0.1:${APP_PORT:-8088}`。发布到公网前，必须在其前方配置 HTTPS 和身份感知反向代理；代理应覆盖而不是转发客户端提供的 `X-Internal-API-Token`，并实施用户级限流。LLM Key 只允许以加密形式保存在后端数据库或作为 sidecar 服务端环境变量存在；自定义模型网关必须加入 `AI_ALLOWED_BACKEND_HOSTS` 精确白名单。

上线、回滚、备份、密钥轮换和事件响应要求见 [`docs/production-operations.md`](docs/production-operations.md)；漏洞报告流程见 [`SECURITY.md`](SECURITY.md)。

基本健康检查：

```bash
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8765/health
```

## 仓库约定

提交的内容只包括运行源码、测试、配置示例、必要的第三方许可证和本 README。`frontend/dist`、Python `__pycache__`、本地虚拟环境、分析 checkpoint、`.env` 与其他生成文件均不纳入版本控制。
