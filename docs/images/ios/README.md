# CANSLIM iOS 截图清单

这些截图保留自 `CANSLIM-IOS` 的原生 SwiftUI 实现，供后续项目 README、发布说明和功能验收引用。
截图使用 iPhone 17 / iOS 26.5 Simulator 和本地真实 API 数据；画面不包含访问令牌、LLM 密钥或券商凭据。

| 文件 | 页面与状态 | README 建议用途 |
| --- | --- | --- |
| [`01-screener.png`](01-screener.png) | 美股选股结果、CANSLIM 标签、筛选与刷新入口 | 产品首页、移动端信息架构 |
| [`02-analysis-overview.png`](02-analysis-overview.png) | MU 公司资料与二层研究数据 | 基本面证据与数据来源 |
| [`03-analysis-charts.png`](03-analysis-charts.png) | K 线、RS 与财务趋势图 | 原生图表能力 |
| [`04-analysis-ai.png`](04-analysis-ai.png) | AI 配置已读取、分析入口待开始 | 多智能体工作流入口；未产生模型调用费用 |
| [`05-trade-plans.png`](05-trade-plans.png) | 交易计划列表空状态 | 完整状态设计与新建入口 |
| [`06-journal.png`](06-journal.png) | 真实交易日志与行为复盘 | 交易闭环与复盘能力 |
| [`07-settings.png`](07-settings.png) | API、市场、AI 与券商设置入口 | 本地优先和安全配置 |
| [`08-analysis-new-plan.png`](08-analysis-new-plan.png) | 从 MU 分析页进入、已预填代码的新建计划表单 | 分析到执行计划的跨模块闭环 |

更新截图时应继续使用相同命名，先确认页面无系统确认框遮挡，并检查画面中没有任何密钥或账户敏感信息。
