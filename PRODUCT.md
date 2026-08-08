# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

主要使用者是产品作者本人:作为日常投资研究工具使用,同时作为对外作品集的核心展示项目(GitHub、README、演示),面向审阅者、同行与潜在合作者。界面需要同时经得起"真实使用"和"第一眼观感"两种检验。

## Product Purpose

AlphaLens 是一个覆盖美股与 A 股的本地优先投资研究工作台。它把分散在行情、财报、新闻、AI 对话和交易记录中的工作收进一条可追溯的研究链路:发现候选 → 核查证据 → 多视角 AI 讨论 → 人工决策 → 记录执行 → 交易复盘。成功标准是:使用者能判断 AI 结论背后的证据是否可信,并且最终决策始终留在人手里。产品不自动下单,不把模型输出包装成交易信号。

## Positioning

"让 AI 展示证据,而不是只给结论"。与常见 AI 选股工具的区别在于机制本身:分析前先审计来源、报告期间与数据冲突,缺失信息明确显示为"未知"而不是零;多角色(基本面、技术、新闻、情绪、看多、看空、风险)分别研究、辩论和审阅;券商连接只读且导入需人工确认。这些约束是产品身份的一部分,不是功能缺失。

## Operating Context

自托管全栈:Go 后端(API + SSE)→ PostgreSQL(Docker)→ Python AI sidecar;前端为 React + Vite 开发服务器。另有原生 iOS 客户端共享同一套 REST/SSE 契约(`ai-analysis.v1` 流式事件)。数据源包括 Yahoo Finance、Financial Modeling Prep、SEC EDGAR;美股覆盖标普 500、纳指 100、罗素 2000,A 股覆盖沪深主板、科创板、创业板与北交所。市场切换与股票池隔离是核心交互之一。

## Capabilities and Constraints

- 五个核心页面:筛选器(CANSLIM 扫描与阈值预设)、个股分析(证据、图表、多智能体 AI 研究)、交易计划、交易日志与行为复盘、设置(LLM 凭据、券商连接)。
- 技术栈:React 18 + Vite + Tailwind CSS 3 + recharts + TanStack Query + react-markdown;SSE 实时接收扫描与 AI 分析事件。
- 界面语言为简体中文;CANSLIM 六字母(C/A/N/S/L/I)术语贯穿全站。
- 硬约束:不提供自动下单;加载、空数据、错误、断流显示真实状态,不用伪造数据或占位文案填充;来源与数据日期是内容的一部分,不可隐藏。
- 主题:以深色为主(2026-08 重设计确认),需配合系统偏好。

## Brand Commitments

- 产品名 AlphaLens(仓库名 CANSLIMAgent);标语方向:"让 AI 展示证据,而不是只给结论"。
- 品牌 voice:克制、有据、不夸大;不用感叹式营销语言,不承诺收益。
- 现有素材:docs/assets/alphalens-title.svg、iOS 截图(docs/images/ios/ 12 张)、docs/images/demo-screener.jpg。

## Evidence on Hand

- 真实选股数据:标普 500 503 只候选、约 160 只达标,DELL/EME/FIX/GEV 等真实标签与数值(数据日期 2026-07-10)。
- 真实 AI 分析报告存在于数据库(FIX 等标的已生成)。
- 不得虚构:收益承诺、用户证言、媒体引用、"回测通过率"类指标。

## Product Principles

1. 证据先于结论:任何数值都附带来源与数据日期,未知就显示未知。
2. 暴露分歧:看多与看空的冲突是内容,不是噪音。
3. 人保留最终判断:界面引导核查,不引导冲动。
4. 真实状态优先:空、慢、错都是真实状态,值得被认真设计。
5. 研究工具的气质:长时间使用的密度与扫读性,胜过一次性演示的华丽。
