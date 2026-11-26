<p align="center">
  <img src="docs/assets/alphalens-title.svg" alt="AlphaLens 2.0 · 以 R 为标尺的盘前风控与行为对账终端" width="920" />
</p>

<h1 align="center">AlphaLens 2.0：让 AI 展示证据，用 1R 守住纪律</h1>

<p align="center">
  面向自主波段交易者（美股 / A 股）的本地优先决策与风险对账工作台<br />
  大盘 M 闸门 → 结构化证据核查 → 45 秒 1R 算仓立计划 → 红队质询 → 全周期四象限 R 复盘
</p>

<p align="center">
  <a href="#快速开始"><img src="https://img.shields.io/badge/Web-4大工作区-0f766e?style=for-the-badge" alt="Web 4大工作区" /></a>
  <a href="ios/README.md"><img src="https://img.shields.io/badge/iOS-Companion客户端-2878ff?style=for-the-badge&amp;logo=apple&amp;logoColor=white" alt="iOS Companion客户端" /></a>
  <a href="docs/product/ALPHALENS_2.0_PLAN.md"><img src="https://img.shields.io/badge/Blueprint-AlphaLens_2.0-334155?style=for-the-badge" alt="AlphaLens 2.0 方案" /></a>
</p>

把一个股票代码交给 AI 很容易，难的是在真金白银下单前管住手，在亏损时不逃避，在靠运气赚钱时保持清醒。

**AlphaLens 2.0**（仓库名：`CANSLIMAgent`）不替你算命涨跌，也不提供自动下单；它是一台围绕 **CANSLIM `M` 大盘闸门**、**`1R` 头寸风控** 与 **全周期四象限 `R` 账本** 构建的个人交易终审终端。

## 4 大时序工作区

```text
作战室 (/desk) ──▶ 研究台 (/research) ──▶ 头寸簿 (/positions) ──▶ 交易系统 (/system)
```

| 工作区 | 核心职责 |
| :--- | :--- |
| **作战室 `/desk`** | 核查美股（`SPY`/`QQQ`）与 A 股（`沪深300`/`创业板指`）的 **CANSLIM `M` 大盘环境状态与建议仓位上限**；监控 **Portfolio Heat（在途总风险敞口）** 与北极星指标 **合规风险闭环率**；快捷调整在途持仓止损、处理待触发计划与待复盘结案。 |
| **研究台 `/research`** | 可折叠 CANSLIM 候选筛选抽屉 + 量价/基本面图表 + **4 张确定性审计证据卡片**（可选单跳 `<5s` **AI 提炼多空反证**）+ 右侧 **45 秒开单舱**（选 Playbook → 填入场/止损与 `0.25R`–`1.5R` 风险档位自动算股数 → 1 句话论点与证伪条件 → 触发 **Pre-Trade Red Team 红队审查**）。 |
| **头寸簿 `/positions`** | 以完整仓位生命周期 `Trade` 为核心：在途持仓移动止损与减平仓、等待计划 1 键记录成交、已结案交易自动计算 **`Realized R`** 与过程×结果四象限徽章（`Model Win` / `Good Loss` / `Unforced Error` / `Toxic Win`）+ 15 秒快速复盘、券商只读成交导入与计划外裸奔单补关联。 |
| **交易系统 `/system`** | 2×2 四象限行为账单（明确正向激励合规止损 `Good Loss`，红色警戒危险的 `Toxic Win`）、账户总净值与 `1R`/Heat 风控底线、Playbook 与受审纪律库（AI 建议纪律默认 `draft` 需人工审批）、LLM Key 加密与券商只读连接。 |

## 核心金融引擎与 AI 手术刀边界

- **CANSLIM `M` 环境闸门**：基于指数日线 `21EMA`、`50SMA`、`200SMA` 排列与近 25 个交易日欧奈尔派发日计数，诚实输出四档趋势状态；数据源不可用时显式返回 `unavailable`，绝不伪造数据。
- **确定性 `1R` 头寸计算器**：根据账户净值、单笔标准风险 `1R`、单票集中度上限、组合总 Heat 与市场手数规则（美股 1 股 / A 股 100 股 / 科创板最低 200 股）自动计算建议股数，并对 `< 0.5 ATR` 过窄止损、`> 8%` 过宽止损和逆势开仓给出硬性警示。
- **AI 手术刀替代多智能体剧场**：基于预注册消融实验结论（[CONCLUSION.md](sidecar/data/ablation-studies/2bba7d4c7b869ecb9ac7ecee4819122b7abfe3dad9066f10e9f29b57d1809275/CONCLUSION.md)），用低延迟的 **Evidence Card Compiler** 与 **Pre-Trade Red Team** 替代高延迟多角色辩论；正则代码级禁止输出 `BUY`/`SELL`/`HOLD` 等交易批准语义，每条高风险提醒必须引用输入 `source_id`。
- **原生 iOS Companion**：`ios/` 端聚焦随身风控，首页提供 **作战室 Companion 视图** 随时查看 `M` 闸门、Portfolio Heat、在途持仓与待触发计划。

## 快速开始

### 环境要求

- macOS 或 Linux · Node.js 22+ · Go 1.25+ · Python 3.12+ · Docker Desktop

```bash
git clone https://github.com/HZXJason/CANSLIMAgent.git
cd CANSLIMAgent

cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
cp sidecar/.env.example sidecar/.env

make dev
```

打开 `http://127.0.0.1:5173`。**无需配置 LLM Key 即可完整使用**大盘 `M` 闸门、CANSLIM 筛选、4 张确定性证据卡、`1R` 头寸计算器、计划与全周期四象限 `R` 账本。

## 架构概览

```text
Web (4-Workspace Terminal) / iOS Companion
        │ REST + SSE (/api/v1)
        ▼
Go Core Engine ───────── PostgreSQL
  ├─ MarketRegimeService (M & 派发日)
  ├─ PositionSizer (1R / ATR / Portfolio Heat)
  ├─ TradeLifecycle (FIFO 聚合 / Realized R / 四象限)
  └─ 凭据 AES-GCM 加密 & 券商只读网关
        │
        ▼
Python AI Sidecar (无状态结构化算子)
  ├─ DataAuditor (SEC 报告期 & 口径审计)
  ├─ Evidence Card Compiler (单跳多空反证)
  └─ Pre-Trade Red Team (下单前魔鬼代言人)
```

## 开发与验证

```bash
cd backend && go test -race ./... && go vet ./...
cd ../sidecar && .venv/bin/python -m unittest discover -s . -p 'test_*.py'
cd ../frontend && corepack pnpm run typecheck && corepack pnpm test -- --run && corepack pnpm run build
```

详细部署与密钥轮换见 [`docs/production-operations.md`](docs/production-operations.md)，演进路线与里程碑见 [`docs/product/ALPHALENS_2.0_PLAN.md`](docs/product/ALPHALENS_2.0_PLAN.md)。

---

AlphaLens 是研究与风险纪律支持工具，不构成投资建议，不承诺收益，也不会自动执行交易。
