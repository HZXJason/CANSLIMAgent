# AlphaLens MVP 指标体系

日期：2026-08-04

使用技能：North Star Metric

## Business Game

AlphaLens 属于 Productivity Game：帮助用户更稳定、高效地完成交易准备、风险检查和复盘。产品不优化停留时长或交易次数。

## North Star Metric

> 纪律交易闭环覆盖率（Disciplined Trade Loop Coverage）

### MVP 口径

```text
在首次成交前已有交易计划，并在交易关闭后 48 小时内完成复盘的闭合交易
────────────────────────────────────────────
所有可识别的闭合真实交易
```

### 口径规则

- “首次成交前已有计划”按计划创建时间与成交时间判断；
- 同一计划多次部分成交仍属于一个计划闭环，但执行差异保留每笔成交；
- 计划外成交进入分母，不能通过不关联来提高覆盖率；
- 未闭合仓位不进入闭合交易分母，但单独显示未闭合数量；
- 关闭时间无法确定时标记不可计算，不伪造 48 小时窗口；
- 后续版本可加入“明确决定不交易”的复盘，但 MVP 不把不可观察事件加入分母。

## Seven-Criteria Check

| 标准 | 判断 |
| --- | --- |
| 易理解 | 表示多少真实交易完成了计划与复盘 |
| 用户中心 | 反映用户得到交易纪律和学习价值 |
| 可持续 | 随长期使用和历史积累增强 |
| 愿景一致 | 对应“清楚开始、诚实结束” |
| 可量化 | 可由计划、成交和复盘时间计算 |
| 可行动 | 计划、审查、导入和复盘流程都能影响 |
| 领先指标 | 比短期收益更早反映产品是否形成习惯 |

## Input Metrics

1. **入场前计划覆盖率**：首次成交前已有计划的闭合交易 / 所有闭合交易；
2. **计划风险审查完成率**：完成风险审查的有效计划 / 进入等待或执行状态的有效计划；
3. **48 小时复盘完成率**：关闭后 48 小时内完成复盘的闭合交易 / 所有可计算闭合交易；
4. **计划—执行可比较率**：具备触发、计划价格或风险字段且关联成交的计划 / 所有关联成交计划；
5. **每周有效今日会话天数**：一周内完成候选加入、计划审查或复盘待办至少一项的独立交易日。

## Outcome and Diagnostic Metrics

| 指标 | 目的 |
| --- | --- |
| 计划外交易率 | 观察是否绕过计划流程 |
| 风险提醒导致的计划修改率 | 判断审查是否产生具体价值 |
| 高风险提醒忽略率 | 识别提醒疲劳或不可信提示 |
| 复盘平均完成时间 | 控制盘后摩擦 |
| 行为观察反馈率 | 判断用户是否认可或反驳画像 |
| 候选进入研究比例 | 判断发现质量而非候选数量 |
| 数据过期/未知比例 | 约束盘前能力真实性 |
| AI 不可用下核心流程完成率 | 证明非 AI 基础仍可用 |

## Guardrails

- 用户交易频率不作为增长目标；
- 短期收益、胜率、AI 方向正确率不作为 MVP 成功指标；
- 用户与 AI 一致率不应被最大化；
- 无来源或过期的关键事实不能进入高置信度提醒；
- 计划填写时间的 P50/P90 应持续监控，避免用户绕过流程；
- 提醒被标记为“泛化/不相关”的比例必须可追踪；
- 不得为了提高覆盖率自动生成空计划或自动完成复盘。

## Event Taxonomy

| Event | Required properties |
| --- | --- |
| `today_session_opened` | market, session_state, timezone, occurred_at |
| `candidate_added` | candidate_snapshot_id, symbol, screen_id, source_freshness |
| `research_case_created` | research_case_id, candidate_snapshot_id, symbol |
| `user_thesis_saved` | thesis_id, research_case_id, completeness, occurred_at |
| `trade_plan_created` | plan_id, thesis_id, created_at, status |
| `risk_review_completed` | review_id, plan_id, warning_counts, source_freshness |
| `risk_warning_acknowledged` | review_id, warning_id, action, reason_present |
| `execution_imported` | execution_id, broker, executed_at, plan_link_status |
| `trade_review_completed` | trade_review_id, plan_id, closed_at, completed_at |
| `behavior_observation_generated` | observation_id, behavior_type, sample_size, confidence |
| `behavior_observation_feedback` | observation_id, feedback, occurred_at |

所有事件使用服务端可校验 ID 和时间。涉及用户文本、券商凭据和敏感交易内容时，只记录计算所需的最少属性，不把原文发送到分析埋点。

## MVP Measurement Readiness

在私测前必须证明：

1. 计划、成交和复盘可以稳定关联；
2. 时间统一存储并按用户时区展示；
3. 部分成交和未闭合仓位不会误算；
4. 指标查询能够解释分子和分母中的原始记录；
5. 删除/重新导入不会重复计算同一券商成交。
