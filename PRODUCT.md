# Product

<!-- impeccable:product-schema 1 -->

## Platform

web + iOS companion

## Users

主要使用者是产品作者本人（自主决策型成长波段交易者）：作为日常盘前风控、研究立计划与盘后行为对账系统使用，同时作为工程与金融产品能力的公开作品集项目。

## Product Purpose

AlphaLens 2.0 是覆盖美股与 A 股的本地优先投资研究与纪律对账终端。它将交易全生命周期收敛为四个连续工作区：`作战室 (/desk)` → `研究台 (/research)` → `头寸簿 (/positions)` → `交易系统 (/system)`。北极星指标为**合规风险闭环率（Disciplined R-Loop Rate）**：衡量真正走完计划、红队审查、受控止损与结案复盘的交易占比。

## Positioning

“让 AI 展示证据，用 1R 守住纪律”。产品不预测股价，不提供自动下单，不把 AI 多数意见包装成 BUY/SELL 信号。所有确定性金融计算（CANSLIM `M` 大盘状态、派发日计数、`1R` 头寸计算、Portfolio Heat、FIFO 仓位聚合、`Realized R` 与四象限分类）均由 Go 后端确定性完成；AI 仅作为 `Evidence Card Compiler` 与 `Pre-Trade Red Team` 两把低延迟手术刀运行。

## Operating Context

- 自托管全栈：Go API + PostgreSQL + 无状态 Python AI Sidecar + React 18 / Vite / Tailwind CSS 3 / Recharts / TanStack Query；原生 SwiftUI iOS 作为随身风控 Companion。
- 市场隔离：US（`SPY`/`QQQ`，S&P 500 / Nasdaq 100 / Russell 2000）与 CN（`000300.SH`/`399006.SZ`，沪深主板 / 科创板 / 创业板 / 北交所）完全隔离。

## Product Principles

1. **大盘环境优先（M）**：无视大盘回调期的个股突破是系统性亏损主因。
2. **以 `R` 统一度量**：入场前先算清止损与 `1R` 股数，控制组合总 `Open Heat`。
3. **过程与结果分离**：公开肯定严格止损的 `Good Loss`，红色警戒靠扛单或乱买赚来的 `Toxic Win`。
4. **证据先于结论 & 诚实降级**：数据缺失显示未知，数据源失败显示不可用，绝不伪造确定感。
