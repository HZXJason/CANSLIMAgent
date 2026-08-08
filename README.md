<p align="center">
  <img src="docs/assets/alphalens-title.svg" alt="AlphaLens · AI 原生投资研究工作台" width="920" />
</p>

<h1 align="center">AlphaLens：让 AI 展示证据，而不是只给结论</h1>

<p align="center">
  覆盖美股与 A 股的本地优先投资研究工作台<br />
  从发现候选、核查证据、AI 多视角讨论，到交易计划与复盘，一条链路完成
</p>

<p align="center">
  <a href="#快速开始"><img src="https://img.shields.io/badge/Web-立即体验-0f766e?style=for-the-badge" alt="Web 立即体验" /></a>
  <a href="ios/README.md"><img src="https://img.shields.io/badge/iOS-原生客户端-2878ff?style=for-the-badge&amp;logo=apple&amp;logoColor=white" alt="iOS 原生客户端" /></a>
  <a href="#它如何工作"><img src="https://img.shields.io/badge/Workflow-研究闭环-334155?style=for-the-badge" alt="研究闭环" /></a>
  <a href="docs/images/ios/README.md"><img src="https://img.shields.io/badge/Screenshots-真实界面-16a579?style=for-the-badge" alt="真实界面" /></a>
</p>

把一个股票代码交给 AI 很容易。难的是知道：它用了什么证据、遗漏了什么、不同观点在哪里冲突，以及这个结论是否值得你采纳。

**AlphaLens**（仓库名：CANSLIMAgent）把原本散落在行情、财报、新闻、AI 对话和交易记录里的工作，收进一条可追溯的研究链路。它不替你下注，也不把模型输出包装成交易信号；它负责整理证据、暴露分歧、记录决策，让最终判断始终留在人手里。

![AlphaLens CANSLIM 选股工作台](docs/images/demo-screener.jpg)

## 为什么值得一试

| 你真正需要的 | AlphaLens 带来的体验 |
| --- | --- |
| 不再在行情、财报、新闻和聊天窗口之间来回切换 | 从 CANSLIM 初筛直接进入同一标的的证据、分析、计划与复盘 |
| 不只看到 AI 的答案，还能判断答案是否可信 | 审计来源、报告期间与数据冲突；缺失信息明确显示为未知 |
| 不被单一看多或看空叙事牵着走 | 基本面、技术、新闻、情绪、看多、看空与风险角色逐层审阅 |
| 美股与 A 股都能研究，又不会混用错误证据 | 两个市场共享研究语言，但隔离股票池、数据源与证据边界 |
| 保留自己的数据和最终决定 | 本地优先、券商只读、导入前确认、不提供自动下单 |

一句话概括它的工作方式：

```text
发现标的 → 核查证据 → 多视角讨论 → 人工决策 → 记录执行 → 交易复盘
```

## 快速开始

### 环境要求

- macOS 或 Linux
- Node.js 22+
- Go 1.25+
- Python 3.12+
- Docker Desktop

### 启动 Web 工作台

```bash
git clone https://github.com/HZXJason/CANSLIMAgent.git
cd CANSLIMAgent

cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
cp sidecar/.env.example sidecar/.env

make dev
```

打开 `http://127.0.0.1:5173`。首次启动会创建数据库并安装前端与 Python 依赖，因此会比后续启动更久一些。

**没有 LLM Key 也可以先体验**跨市场筛选、个股资料、图表、交易计划和日志。要运行 AI 研究，可在 `sidecar/.env` 中配置任一受支持的模型服务；也可以在 `backend/.env` 中设置 `LLM_CREDENTIALS_ENCRYPTION_KEY`，再从产品设置页安全保存 Key。

推荐从这条最短路径开始：

1. 在“筛选器”选择美股或 A 股股票池；
2. 打开一个候选标的，检查 K 线、RS、增长指标、来源与日期；
3. 配置模型后运行 AI 分析，观察不同角色如何形成、质疑和修正观点；
4. 把认可的条件写进交易计划，之后用真实执行记录完成复盘。

原生 iPhone 版本的构建、连接与完整功能说明见 [AlphaLens iOS](ios/README.md)。

## 它如何工作

### 1. 先从跨市场候选池缩小范围

美股与 A 股使用各自独立的数据路由、股票池和视觉主题。用户可以用同一套 CANSLIM 语言筛选，但系统不会把 SEC、美国内部人或美股期权等证据错误套用到 A 股。

| 美股市场 | A 股市场 |
| --- | --- |
| ![美股 CANSLIM 筛选](docs/images/demo-screener.jpg) | ![A 股创业板 CANSLIM 筛选](docs/images/demo-a-share.jpg) |

筛选结果不只给一个难以解释的总分。C、A、N、S、L、I 标签分别保留达标状态与原始值，让你可以继续追问“为什么它会入选”。

### 2. 在同一页核查价格、增长与相对强度

进入个股后，价格走势、成交量、RS 相对强度、EPS、营收、毛利率、公司资料、来源与数据日期会集中展示。某项数据缺失或请求失败时，界面保留真实状态，不用伪造数据填满页面。

![个股证据与图表工作台](docs/images/demo-analysis.jpg)

### 3. 看见 AI 结论是怎样形成的

AI 分析不是一次性问答，而是一个带角色分工的研究流程：基本面、技术、新闻和情绪分析师先形成证据，再由看多 / 看空研究员、交易员、风险角色和投资组合经理逐层审阅。

在证据进入分析前，确定性的 `DataAuditor` 会检查来源、申报类型、报告期间与 accession。期间不明、口径冲突或无法验证的数值不会被当作事实送入分析上下文。

```text
市场证据预取
  → 来源与期间审计
  → 分析师团队
  → 看多 / 看空研究
  → 交易与风险审阅
  → 面向人工的最终观点
```

完整报告通过 SSE 实时展示，可暂停、续传并导出 Markdown。方向判断与仓位建议被明确区分，避免把“看好一家公司”直接误解成“应该投入多少”。

### 4. 不让研究止步于一份报告

交易计划记录触发条件、风险条件和研究理由；交易日志支持手工记录，也可以从长桥、老虎、富途 OpenD 或 IBKR 本地会话只读预览成交。任何导入都需要用户确认，系统不具备下单能力。

![交易日志与复盘工作流](docs/images/demo-journal.jpg)

日志可以导出为 `vibe.trade_record.v1` 兼容记录，并生成确定性的行为复盘。下一次研究参考的是你的真实执行，而不只是模型上一次说了什么。

### 5. 桌面深度研究，手机随时跟进

Web 端适合长时间研究与多区域并排查看；原生 iOS 客户端则按 iPhone 的导航、触控节奏和信息密度重新设计。两端共享同一套数据含义与 REST/SSE 契约。

<p align="center">
  <img src="docs/images/demo-mobile.jpg" alt="AlphaLens 移动端 CANSLIM 筛选" width="390" />
</p>

## 这不是一个“AI 荐股器”

AlphaLens 有意保留这些边界：

- **不自动下单**：研究、计划与执行记录相连，但最终操作始终由用户完成；
- **不承诺收益**：AI 输出是可审阅的研究材料，不是准确率或回报率保证；
- **不伪造完整性**：数据缺失显示未知，接口失败显示错误，不用占位内容制造确定感；
- **不让多数意见代替判断**：多角色讨论用于暴露证据与分歧，不把“投票”包装成真相；
- **不把方向等同于仓位**：市场观点、风险条件和资金投入是不同层次的问题。

## 当前能力

- S&P 500、Nasdaq 100、Russell 2000 与沪深主板、科创板、创业板、北交所扫描；
- CANSLIM 标签、阈值方案、组合筛选与候选排序；
- K 线、RS、EPS、营收、毛利率、公司资料和机构证据展示；
- 多智能体研究、证据协调、数据审计与流式报告；
- 交易计划、交易日志、券商只读成交预览与行为复盘；
- LLM Key 服务端加密存储、模型发现、断线恢复和报告导出；
- 响应式 Web 工作台与原生 SwiftUI iOS 客户端。

## 架构概览

```text
Web / native iOS
        │ REST + SSE
        ▼
      Go API ───────── PostgreSQL
        │
        ▼
Python AI Sidecar
  ├─ TradingAgents 研究图
  ├─ Vibe MCP 桥接
  ├─ DataAuditor
  └─ 日志复盘运行时
```

客户端只访问 Go 服务的 `/api/v1`。Go 负责稳定 API、凭据安全与流式代理，Python sidecar 负责 AI 编排和研究工具接入；两者通过可替换的上游地址解耦。

### 数据策略

- 美股财报与申报：SEC EDGAR 优先，FMP 与 yfinance 可选补充；
- 美股价格与图表：Stooq、Yahoo Finance 与 yfinance 兜底；
- A 股资料与财务指标：东方财富；复权行情与相对强度：腾讯证券；
- 来源失败、数据日期与证据不足会直接展示，不生成替代结果。

### 安全边界

- LLM Key 使用 AES-GCM 加密保存，前端只能读取配置状态；
- 券商 OAuth 凭据和本地网关配置不会返回浏览器；
- 富途与 IBKR 只允许 loopback 地址，券商同步保持只读；
- 生产部署默认使用非 root 容器、内部网络、共享密钥和显式 CORS / 网关白名单。

详细部署与密钥轮换说明见 [`docs/production-operations.md`](docs/production-operations.md)，漏洞报告流程见 [`SECURITY.md`](SECURITY.md)。

## 开发与验证

```bash
cd backend && go test ./...
cd ../sidecar && .venv/bin/python -m unittest discover -s . -p 'test_*.py'
cd ../frontend && corepack pnpm run typecheck && corepack pnpm test && corepack pnpm run build
```

如果启动后无法连接服务，可分别检查：

```bash
curl http://127.0.0.1:8765/health
curl http://127.0.0.1:8080/api/v1/health
curl http://127.0.0.1:8080/api/v1/analysis/ai/config
```

贡献前请阅读 [`CONTRIBUTING.md`](CONTRIBUTING.md)。本 README 中的界面截图来自本地真实服务与缓存数据，不是设计稿；截图中的数据日期仅代表当时可用的数据快照。

---

AlphaLens 是研究与决策支持工具，不构成投资建议，不承诺收益，也不会自动执行交易。
