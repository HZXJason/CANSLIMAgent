# AlphaLens MVP 产品基线

日期：2026-08-04

状态：MVP 实施基线

分支：`codex/mvp`

## 目的

本目录把 AlphaLens 从“CANSLIM 研究工作台”升级为“交易前的理性外脑、交易后的行为镜子”所需的产品决策固化为可追溯文档。后续设计、开发、测试和验收必须能够指向这里的用户问题、产品结果或验收场景。

## 权威顺序

发生冲突时按以下顺序处理：

1. [`05-PRD-AlphaLens-MVP.md`](05-PRD-AlphaLens-MVP.md)：MVP 业务范围、Top 5 能力、约束和完成定义；
2. [`10-acceptance-test-scenarios.md`](10-acceptance-test-scenarios.md)：可观察验收行为；
3. [`06-metrics.md`](06-metrics.md)：成功、护栏和埋点定义；
4. [`08-outcome-roadmap.md`](08-outcome-roadmap.md)：实施顺序和阶段退出条件；
5. 其余发现文档：解释为什么做出这些选择。

现有 [`../../../PRODUCT.md`](../../../PRODUCT.md) 与 [`../../../DESIGN.md`](../../../DESIGN.md) 提供产品和视觉背景；[`../../superpowers/specs/2026-08-04-evidence-ledger-redesign.md`](../../superpowers/specs/2026-08-04-evidence-ledger-redesign.md) 继续约束视觉、响应式和真实状态，但不替代本目录的业务范围。

## 文档清单

| 文档 | 回答的问题 |
| --- | --- |
| [`01-product-vision.md`](01-product-vision.md) | 产品最终希望改变什么？ |
| [`02-market-segments-and-jtbd.md`](02-market-segments-and-jtbd.md) | 首先服务谁，他们在什么情境下需要产品？ |
| [`03-customer-journey-map.md`](03-customer-journey-map.md) | 如何形成盘前、入场前、盘后和周期复盘习惯？ |
| [`04-opportunity-solution-tree.md`](04-opportunity-solution-tree.md) | 哪些问题最值得解决，候选方案与实验是什么？ |
| [`05-PRD-AlphaLens-MVP.md`](05-PRD-AlphaLens-MVP.md) | MVP 要交付什么、明确不做什么？ |
| [`06-metrics.md`](06-metrics.md) | 如何证明产品提供了纪律价值？ |
| [`07-feature-prioritization.md`](07-feature-prioritization.md) | Top 5 为什么是这五项？ |
| [`08-outcome-roadmap.md`](08-outcome-roadmap.md) | 应按什么结果顺序实施？ |
| [`09-pre-mortem.md`](09-pre-mortem.md) | 产品最可能怎样失败，发布前要阻断什么？ |
| [`10-acceptance-test-scenarios.md`](10-acceptance-test-scenarios.md) | 如何端到端验收？ |
| [`11-implementation-audit.md`](11-implementation-audit.md) | Top 5 到了哪里，哪些证据已验证，哪些边界仍然保留？ |

## 决策原则

- AI 不替用户做交易决定；AI 负责澄清、质疑、审查和复盘。
- 盈利不等于高质量交易，亏损也不等于低质量交易。
- CANSLIM 是可解释的筛选模板，不是唯一选股逻辑。
- 盘前候选必须说明入选原因、排除风险、来源和数据新鲜度。
- 个性化提醒必须引用样本、场景和证据，不给用户贴永久人格标签。
- 不自动下单，不刺激交易频率，不以 AI 一致率或短期收益作为成功指标。
- 加载、未知、冲突、过期和失败都必须保持真实状态。

## 变更规则

任何扩大 MVP 范围的改动，必须同时更新 PRD、验收场景和路线图。任何改变北极星指标口径的改动，必须说明历史数据如何迁移或重新解释。产品假设被用户研究推翻时，应修改对应发现文档并在 PRD 的假设部分记录原因。
