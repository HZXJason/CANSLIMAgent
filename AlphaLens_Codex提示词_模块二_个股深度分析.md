# AlphaLens · 模块二：个股深度分析
## Codex 实现提示词

---

## 前提说明

本模块建立在模块一（CANSLIM 选股器）已完成的基础上。复用模块一已有的：
- 数据库中的 `stocks` 和 `stock_metrics` 表
- 多标签页容器组件（`TabStore`、`TabBar`）
- 全局布局与路由结构

**界面语言：所有 UI 文字、标签、提示、错误信息均使用中文。**

---

## 本模块新增任务

在模块一已预留的个股分析标签页内，实现以下内容：
1. 股票头部信息区
2. 五张数据图表（平铺，自动加载）
3. 多智能体 AI 分析区（手动触发，流式输出）
4. N 标签写回入口

---

## 页面路由

```
/analysis          → 多标签页容器（模块一已实现）
/analysis/:symbol  → 个股分析内容页（本模块实现）
```

---

## 数据库新增

无需新增表结构。复用模块一的 `stock_metrics` 表，其中 `n_label` 和 `n_analysis_at` 字段即为本模块写回使用。

---

## 后端新增 API

### 1. K 线数据
```
GET /api/v1/stocks/:symbol/kline?period=daily|weekly|monthly
```
响应：
```json
{
  "success": true,
  "data": {
    "symbol": "ABCD",
    "period": "daily",
    "data_as_of": "2025-06-01",
    "candles": [
      {
        "date": "2025-05-30",
        "open": 123.4,
        "high": 128.9,
        "low": 121.0,
        "close": 127.5,
        "volume": 3820000
      }
    ]
  }
}
```
数据源：Polygon.io，返回最近 365 条（日线）/ 104 条（周线）/ 60 条（月线）

---

### 2. RS 强度线
```
GET /api/v1/stocks/:symbol/rs
```
响应：
```json
{
  "success": true,
  "data": {
    "symbol": "ABCD",
    "data_as_of": "2025-06-01",
    "rs_line": [
      { "date": "2025-05-30", "value": 88.3 }
    ]
  }
}
```
计算方式：股票 12 个月相对涨幅 / 标普 500 同期涨幅，映射至 1-99 百分位，与 K 线时间轴对齐。

---

### 3. 季度 EPS 趋势
```
GET /api/v1/stocks/:symbol/eps/quarterly
```
响应：
```json
{
  "success": true,
  "data": {
    "symbol": "ABCD",
    "source": "Financial Modeling Prep",
    "data_as_of": "2025-03-31",
    "quarters": [
      {
        "period": "2025Q1",
        "eps": 2.34,
        "eps_yoy_growth": 47.2,
        "data_sufficient": true
      }
    ]
  }
}
```
返回最近 12 个季度。`data_sufficient: false` 时 `eps_yoy_growth` 为绝对值，前端需标注。

---

### 4. 年度营收趋势
```
GET /api/v1/stocks/:symbol/revenue/annual
```
响应：
```json
{
  "success": true,
  "data": {
    "symbol": "ABCD",
    "source": "Financial Modeling Prep",
    "data_as_of": "2024-12-31",
    "years": [
      {
        "year": 2024,
        "revenue": 4820000000,
        "revenue_yoy_growth": 28.4
      }
    ]
  }
}
```
返回最近 5 年。

---

### 5. 毛利率走势
```
GET /api/v1/stocks/:symbol/margin/gross/quarterly
```
响应：
```json
{
  "success": true,
  "data": {
    "symbol": "ABCD",
    "source": "Financial Modeling Prep",
    "data_as_of": "2025-03-31",
    "quarters": [
      {
        "period": "2025Q1",
        "gross_margin": 68.3
      }
    ]
  }
}
```
返回最近 12 个季度。

---

### 6. AI 分析（SSE 流式）
```
GET /api/v1/analysis/:symbol/stream
```
后端作为代理，将多智能体系统的流式输出透传至前端。

SSE 事件格式（与多智能体系统约定）：
```
event: thought
data: {"agent": "基本面分析师", "content": "正在分析季度EPS趋势..."}

event: thought
data: {"agent": "技术面分析师", "content": "关注到近期成交量放大..."}

event: report
data: {"content": "## 综合研判\n\n该股票在基本面..."}

event: done
data: {"status": "complete"}

event: error
data: {"message": "分析中断，请检查网络连接后重新生成"}
```

前端根据事件类型分区渲染：`thought` → 思维链区，`report` → 报告区，`done` → 结束状态，`error` → 错误提示。

---

### 7. N 标签写回
```
PATCH /api/v1/stocks/:symbol/n-label
Body: { "n_label": true }
```
响应：
```json
{ "success": true, "data": { "symbol": "ABCD", "n_label": true, "n_analysis_at": "2025-06-01T10:30:00Z" } }
```
写回 `stock_metrics` 表的 `n_label` 和 `n_analysis_at` 字段。

---

## 前端页面结构

### 组件树
```
AnalysisPage (/:symbol)
├── StockHeader            # 头部信息区（固定顶部）
│   ├── StockBasicInfo     # 代码、名称、当前价、市场
│   └── CanslimBadges      # CANSLIM 标签徽章
├── ChartSection           # 图表区（自动加载）
│   ├── KLineChart         # K线图 + 成交量
│   ├── RSChart            # RS强度线（与K线时间轴联动）
│   ├── EPSChart           # 季度EPS趋势
│   ├── RevenueChart       # 年度营收趋势
│   └── GrossMarginChart   # 毛利率走势
└── AISection              # AI分析区（手动触发）
    ├── TriggerButton      # 未触发状态
    ├── ThoughtStream      # 思维链滚动区
    ├── ReportStream       # 最终报告流式输出
    └── AIActions          # 重新生成 + N标签写回
```

---

## 前端状态设计

```typescript
// 新增至现有 store 或单独创建 analysisStore.ts
interface AnalysisStore {
  // 按 symbol 分别存储，支持多标签页各自独立状态
  analysisStates: Record<string, SymbolAnalysisState>;
  getState: (symbol: string) => SymbolAnalysisState;
  updateState: (symbol: string, patch: Partial<SymbolAnalysisState>) => void;
}

interface SymbolAnalysisState {
  // 图表数据
  klineData: KLineData | null;
  klinePeriod: 'daily' | 'weekly' | 'monthly';
  rsData: RSData | null;
  epsData: EPSData | null;
  revenueData: RevenueData | null;
  grossMarginData: GrossMarginData | null;

  // 图表加载状态（每张独立）
  chartStatus: {
    kline: 'loading' | 'success' | 'error' | 'insufficient';
    rs: 'loading' | 'success' | 'error' | 'insufficient';
    eps: 'loading' | 'success' | 'error' | 'insufficient';
    revenue: 'loading' | 'success' | 'error' | 'insufficient';
    grossMargin: 'loading' | 'success' | 'error' | 'insufficient';
  };

  // AI 分析状态
  aiStatus: 'idle' | 'streaming' | 'done' | 'error';
  thoughtLines: ThoughtLine[];   // 思维链内容
  reportContent: string;         // 最终报告（累积 Markdown）
  aiError: string | null;

  // UI 状态
  thoughtExpanded: boolean;      // 思维链折叠/展开（完成后默认折叠）
  nLabelWritten: boolean;        // N 标签是否已写回
}

interface ThoughtLine {
  agent: string;
  content: string;
  timestamp: number;
}
```

---

## 图表实现要求

使用 **Recharts** 实现所有图表。

### K 线图 + 成交量
```typescript
// K线使用 ComposedChart，上方K线下方成交量
// K线用自定义 CandleStick shape 实现
// 涨跌颜色：上涨绿色（#26A69A），下跌红色（#EF5350）
// 成交量柱颜色跟随当日涨跌
// 日/周/月切换：切换后重新请求对应 period 数据
// 时间轴格式：日线显示 MM/DD，周线/月线显示 YYYY/MM
```

### RS 强度线
```typescript
// 使用 LineChart，折线颜色蓝色（#2E5FA3）
// 时间轴与 K 线图保持一致，切换 K 线 period 时联动切换
// Y 轴范围 0-99
```

### EPS / 营收 / 毛利率
```typescript
// EPS 增长率：BarChart，正值绿色，负值红色
// 营收：ComposedChart，柱状图显示营收绝对值，折线显示同比增长率（双 Y 轴）
// 毛利率：LineChart，折线颜色橙色（#FF9800）
// 所有图表 hover 时显示 Tooltip 展示具体数值
```

### 通用要求
```typescript
// 每张图表右下角文字标注数据来源和截止时间
// 加载中：显示骨架屏（高度与图表一致的灰色占位块）
// 错误状态：图表区域内居中显示错误图示 + 「数据加载失败，请刷新重试」
// 数据不足：图表区域内居中显示「历史数据不足，无法生成完整图表」
```

---

## AI 分析区实现要求

### SSE 连接管理
```typescript
// 使用原生 EventSource 或封装的 SSE hook
// 切换标签页后连接保持，不中断
// 组件卸载（标签页关闭）时断开连接
// 连接断开时自动重试最多 3 次，超过后触发 error 状态

function useAIStream(symbol: string) {
  // 建立 SSE 连接
  // 监听 thought / report / done / error 事件
  // 将内容追加至 analysisStore 对应 symbol 的状态
  // 返回 { start, abort } 控制方法
}
```

### 思维链展示
```typescript
// 滚动文本区域，max-height: 300px，overflow-y: auto
// 新内容追加时自动滚动到底部
// 每行格式：[Agent名称] 内容文字
// 字体：monospace，字号小一号，颜色浅灰
// 分析完成后默认折叠，标题行：「思维链（已完成）▶ 点击展开」
```

### 最终报告展示
```typescript
// 使用 react-markdown 渲染 Markdown
// 流式输出：每收到一段 report 事件内容，追加至 reportContent 字符串
// 光标效果：在末尾显示闪烁光标（CSS animation），done 事件后消失
// 报告完成后底部出现：「重新生成」按钮 + 「将 N 标签写回选股器」按钮
```

### 重新生成确认弹窗
```typescript
// 使用 shadcn/ui 的 AlertDialog 组件
// 标题：「确认重新生成」
// 内容：「重新生成将覆盖当前分析结果，是否继续？」
// 按钮：「取消」「确认重新生成」
// 确认后：清空 thoughtLines、reportContent、aiError，重置 nLabelWritten，重新建立 SSE 连接
```

---

## UI/UX 细节要求

1. **头部固定**：`StockHeader` 使用 `position: sticky; top: 0` 固定在页面顶部，滚动时始终可见
2. **图表间距**：每张图表之间留 24px 间距，图表内部上下各 16px padding
3. **图表高度**：K线图 400px，RS线 120px，其余三张各 250px
4. **AI 分析区背景**：使用略深于页面背景的卡片样式，与图表区明显区分
5. **N 标签按钮**：写回前为普通按钮样式；写回成功后变为绿色禁用状态，文字改为「已标记 N ✓」
6. **流式光标**：使用 CSS `@keyframes blink` 实现，done 后移除
7. **骨架屏**：使用 shadcn/ui 的 `Skeleton` 组件，高度与对应图表一致

---

## 文件结构（新增部分）

```
frontend/src/
├── pages/
│   └── Analysis/
│       ├── index.tsx                  # 多标签页容器（模块一已有，本期扩展内容区）
│       ├── AnalysisPage.tsx           # 个股分析主页面
│       ├── StockHeader.tsx            # 头部信息区
│       ├── charts/
│       │   ├── KLineChart.tsx         # K线 + 成交量
│       │   ├── RSChart.tsx            # RS强度线
│       │   ├── EPSChart.tsx           # 季度EPS趋势
│       │   ├── RevenueChart.tsx       # 年度营收趋势
│       │   └── GrossMarginChart.tsx   # 毛利率走势
│       └── ai/
│           ├── AISection.tsx          # AI分析区容器
│           ├── ThoughtStream.tsx      # 思维链滚动区
│           ├── ReportStream.tsx       # 报告流式输出
│           └── AIActions.tsx          # 重新生成 + N标签按钮
├── stores/
│   └── analysisStore.ts              # 个股分析状态（按symbol分组）
├── hooks/
│   └── useAIStream.ts                # SSE 连接管理 hook
└── types/
    └── analysis.ts                   # 本模块类型定义

backend/internal/
├── handlers/
│   ├── analysis.go                   # AI分析流式代理接口
│   └── charts.go                     # 图表数据接口
└── services/
    ├── kline.go                      # K线数据服务
    ├── rs.go                         # RS强度计算服务
    └── fundamentals.go               # EPS/营收/毛利率服务
```

---

## 实现优先级

### Phase 1（核心可用）
1. 后端：五个图表数据接口（复用模块一已有的 FMP/Polygon 服务层）
2. 前端：头部信息区 + 五张图表平铺展示 + 骨架屏 + 错误状态
3. K线图日/周/月切换

### Phase 2（AI 接入）
4. 后端：SSE 代理接口（透传多智能体系统输出）
5. 前端：AI 分析区完整实现（思维链 + 报告流式输出）
6. 重新生成确认弹窗

### Phase 3（体验完善）
7. N 标签写回功能
8. 思维链折叠/展开
9. RS 线与 K 线时间轴联动
10. 所有异常处理与空状态完善

---

## 注意事项

- **状态按 symbol 隔离**：多标签页各自维护独立的图表数据和 AI 状态，切换标签页不互相干扰
- **SSE 连接生命周期**：与标签页生命周期绑定，标签页关闭时断开连接，切换时保持后台运行
- **react-markdown 安全**：AI 输出内容经过 `rehype-sanitize` 处理，防止 XSS
- **K线自定义 shape**：Recharts 原生不支持 K 线，需用 `customizedBar` 或 `Cell` 自定义渲染蜡烛图形状
- **双 Y 轴**：营收图表使用 Recharts 的 `<YAxis yAxisId="left">` 和 `<YAxis yAxisId="right" orientation="right">` 实现
- **所有 UI 文字使用中文**，包括图表 Tooltip、错误提示、按钮文字、加载状态文字
