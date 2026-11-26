# AlphaLens 2.0 全面改版方案与目标化执行清单

> 状态：进行中（分支 `feat/alphalens-v2`）  
> 目标：把 AlphaLens 从“CANSLIM 选股 + 多智能体辩论壳”重构为面向自主成长波段交易者的**“以 `R` 为标尺、以证据为锚点的盘前风控与行为对账终端”**。

---

## 一、核心诊断：为什么必须改版

1. **做 CANSLIM 却丢了最致命的字母 `M`（Market Direction）**：系统有 C/A/N/S/L/I 六维，却缺少大盘趋势状态与派发日计数，导致大盘下跌回调期仍在引导用户看多突破。
2. **缺少专业交易通用货币 `R` 与组合在途风险（Portfolio Heat）**：入场价、止损价、仓位比例互相割裂，且复盘停留在单笔成交（Fill）而非完整仓位生命周期（Trade），无法算出真实的 `Realized R`、扛单违纪与毒性盈利。
3. **AI 算力投放在已被证伪的“多智能体辩论剧场”**：消融实验（[CONCLUSION.md](../../sidecar/data/ablation-studies/2bba7d4c7b869ecb9ac7ecee4819122b7abfe3dad9066f10e9f29b57d1809275/CONCLUSION.md)）证明多智能体辩论未带来正边际收益，且一致率仅 60%、延迟高；而下单前红队质询与证据结构化提炼才是高价值区。
4. **交互摩擦超标与工程表面积过大**：6+5 个孤岛页面、20+ 表单字段导致盘中极易绕过系统裸奔；`frontend/design2`、`ios/` 根目录截图、外部 `Vibe` 子进程桥接、以及过早抽象的标签元模型消耗维护精力。

---

## 二、产品重定位 2.0 与北极星指标

- **产品定位**：不替你预测明天哪只股票涨，只帮你守住三件事——**逆势管住手（`M` 闸门）、下单算清账（`1R` 头寸与红队挑刺）、事后不撒谎（全周期 `R` 与四象限对账）**。
- **唯一北极星指标（North Star Metric）**：**合规风险闭环率（Disciplined R-Loop Rate）**

$$\text{Disciplined R-Loop Rate} = \frac{\text{关联有效计划} \land \text{经入场前审查} \land \text{非大幅扛单 } (\text{Realized } R \ge -1.15R) \land \text{已结案复盘的交易笔数}}{\text{同期完结交易总笔数}}$$

---

## 三、重组后的 4 大时序工作区（IA）

| 工作区 | 路由 | 替代模块 | 核心职责 |
| :--- | :--- | :--- | :--- |
| **作战室** | `/desk` | `/today` | 看大盘 `M` 状态灯与建议总仓位上限、查组合 `Open Heat` 与 `1R` 预算、处理在途持仓止损 / 待触发计划 / 待复盘结案 |
| **研究台** | `/research` | `/screener` + `/analysis` | 左侧可折叠候选抽屉 + 中央量价/基本面/4 张审计证据卡 + 右侧 **45 秒极速立计划舱（Playbook + `1R` 算仓 + 论点/证伪条件 + 红队审查）** |
| **头寸簿** | `/positions` | `/trade-plans` + `/journal` | 以完整仓位 `Trade` 为主表：在途持仓调止损、待触发计划 1 键记录成交、已结案 `Realized R` 与四象限复盘、券商只读同步与裸奔单归档 |
| **交易系统** | `/system` | `/me` + `/settings` | 四象限行为账单（重点识别 `Good Loss` 与 `Toxic Win`）、账户净值与 `1R`/Heat 风控底线、Playbook 与 Policy 规则库、LLM 与券商连接 |

---

## 四、金融核心引擎与 AI 手术刀规格

### 1. CANSLIM `M` 大盘闸门（`MarketRegimeService`）
- 美股基准：`SPY` + `QQQ`；A 股基准：`000300.SH` + `399006.SZ`（两市场严格隔离）。
- 确定性计算：`21EMA` / `50SMA` / `200SMA` 排列 + 近 25 个交易日欧奈尔派发日（下跌 $\ge 0.2\%$ 且放量，若后续反弹 $\ge 5\%$ 则失效）。
- 输出四档状态与仓位上限：`confirmed_uptrend` (100%) / `uptrend_under_pressure` (50%) / `rally_attempt` (25%) / `correction` (0%)；行情源不可用时诚实返回 `unavailable` 或 `insufficient_data`。

### 2. 确定性 `1R` 头寸计算器 & Portfolio Heat
- 输入：Playbook + 入场价 + 止损价 + 风险档位（`0.25R` / `0.5R` / `1.0R` / `1.5R`）。
- 规则：$\text{Shares} = \lfloor \min(\frac{\text{Equity} \times 1R\% \times \text{Multiple}}{|P_{\text{entry}} - P_{\text{stop}}|}, \frac{\text{Equity} \times \text{MaxPos\%}}{P_{\text{entry}}}) \rfloor$，自动按美股 1 股 / A 股 100 股 / 科创板最低 200 股取整。
- 校验：止损 $< 0.5\times\text{ATR(14)}$ 标红提醒、止损 $> 8\%$ 提醒、组合 `Open Heat` 超限拦截提示、潜在盈亏比 $< 2R$ 提示。

### 3. 全周期 `Trade` 账本与四象限矩阵
- 由后端 FIFO 确定性聚合多笔买卖 `TradeJournalEntry` 成为完整 `Trade`，计算 `Realized PnL`、`Realized R`、`Entry Slippage (R)` 与自动违纪抓拍（`off_plan_unlinked`、`entry_chase_over_0_25r`、`stop_loss_violated`）。
- 四象限归类：
  - `model_win`（好过程 + 盈利）
  - `good_loss`（好过程 + 亏损：合规止损，给予正向激励）
  - `unforced_error`（坏过程 + 亏损）
  - `toxic_win`（坏过程 + 盈利：最高优先级红色风险警示）

### 4. AI 架构瘦身
- 保留确定性 `DataAuditor`，移除外部 `Vibe` 子进程调用。
- 算子 1：**`Evidence Card Compiler`**（4 张确定性审计卡片 + 单跳可选多空反证提炼，禁止 `BUY`/`SELL`/`HOLD`）。
- 算子 2：**`Pre-Trade Red Team`**（接入 `M` 状态、`ATR` 止损比、`Portfolio Heat` 与历史违纪）。

---

## 五、里程碑目标追踪（Milestone Tracker）

- [x] **M0 — 分支清理、遗留包袱清除与方案归档**：GitHub/本地仅保留 `dev` 与 `feat/alphalens-v2`；删除 `frontend/design2` 与 `ios/` 散落截图；更新 CI 分支；落盘本蓝图。
- [x] **M1 — Go 后端金融核心底座**：裁剪过度复杂的 `ProfileTag` 分类学；扩展 `1R`/Heat 配置与预设 Playbooks；上线 `MarketRegimeService`（`M`）、`PositionSizer`（`1R`）、`TradeLifecycle`（FIFO 聚合 + `Realized R` + 四象限）、`/api/v1/desk` 与 `/api/v1/trades`；升级确定性风控审查。
- [x] **M2 — Sidecar AI 手术与 `Vibe` 解耦**：移除 `vibe_journal_runner.py` 子进程依赖；新增 `evidence_cards.py` 与 `/api/v1/stocks/:symbol/evidence-cards`；升级 `Pre-Trade Red Team` 提示词与上下文契约。
- [x] **M3 — 前端 4 大工作区重构 & iOS Companion**：重构导航为 `/desk`、`/research`、`/positions`、`/system`；实现 45 秒研究开单舱、四象限行为账单、清理废弃页面；iOS 端增补 `作战室` Companion 视图并验证 Xcode 编译。
- [ ] **M4 — 文档同步、全量 CI 验收与远端推送**：同步更新 `README.md`、`PRODUCT.md`、`DESIGN.md`、`CHANGELOG.md`；跑通 Go / Python / Frontend / iOS 全量测试构建；清理推送至 `origin/feat/alphalens-v2`。
