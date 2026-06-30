# AlphaLens · 模块一：CANSLIM 选股器
## Codex 实现提示词

---

## 项目概览

你正在构建 **AlphaLens** 的第一个核心模块——CANSLIM 选股器。这是一款面向个人投资者的股票研究工具，帮助用户基于奥尼尔 CANSLIM 法则筛选候选股票。

**整体架构：前后端分离**
- 前端：React + TypeScript + shadcn/ui + Zustand
- 后端：Go（API Gateway）+ PostgreSQL
- 数据源：Financial Modeling Prep（FMP）+ Polygon.io

**界面语言：所有 UI 文字、标签、提示、错误信息均使用中文。**

---

## 技术栈要求

### 前端
```
React 18 + TypeScript
shadcn/ui 组件库
Zustand 状态管理
TanStack Query（数据请求与缓存）
Recharts（图表，本模块暂不使用，为后续模块预留）
Tailwind CSS
```

### 后端
```
Go 1.22+
Gin Web Framework
PostgreSQL 15+
GORM（ORM）
```

---

## 数据库 Schema

```sql
-- 股票基础信息
CREATE TABLE stocks (
  id          SERIAL PRIMARY KEY,
  symbol      VARCHAR(10) NOT NULL,
  name        VARCHAR(100) NOT NULL,
  market      VARCHAR(20) NOT NULL, -- 'SP500' | 'NASDAQ100' | 'RUSSELL2000'
  created_at  TIMESTAMP DEFAULT NOW(),
  updated_at  TIMESTAMP DEFAULT NOW(),
  UNIQUE(symbol, market)
);

-- 各维度原始数值（每次刷新覆盖写入）
CREATE TABLE stock_metrics (
  id                    SERIAL PRIMARY KEY,
  stock_id              INTEGER REFERENCES stocks(id),
  -- C: 当季EPS同比增长（百分点，绝对值）
  c_eps_growth_pct      DECIMAL(10,2),
  c_data_sufficient     BOOLEAN DEFAULT TRUE,
  -- A: 年度EPS增长（最近3年各年同比增长率数组，JSON）
  a_annual_eps_growth   JSONB,   -- e.g. [25.3, 31.2, 28.7]
  a_years_available     INTEGER, -- 实际可用年数
  a_data_sufficient     BOOLEAN DEFAULT TRUE,
  -- S: 突破成交量倍数（相对50日均量）
  s_volume_ratio        DECIMAL(10,2),
  s_data_sufficient     BOOLEAN DEFAULT TRUE,
  -- L: RS评级（0-99）
  l_rs_rating           INTEGER,
  l_data_sufficient     BOOLEAN DEFAULT TRUE,
  -- I: 机构持仓季度净变化（机构数量）
  i_institution_delta   INTEGER,
  i_data_sufficient     BOOLEAN DEFAULT TRUE,
  -- N标签（由个股分析页AI写回，初始为NULL）
  n_label               BOOLEAN,
  n_analysis_at         TIMESTAMP,
  -- 数据时间戳
  data_as_of            DATE NOT NULL,
  fetched_at            TIMESTAMP DEFAULT NOW(),
  UNIQUE(stock_id)
);

-- 用户阈值方案
CREATE TABLE threshold_presets (
  id              SERIAL PRIMARY KEY,
  name            VARCHAR(50) NOT NULL,
  is_default      BOOLEAN DEFAULT FALSE,
  c_min_growth    DECIMAL(5,2) DEFAULT 25.0,
  a_min_growth    DECIMAL(5,2) DEFAULT 25.0,
  a_min_years     INTEGER DEFAULT 3,
  s_volume_ratio  DECIMAL(5,2) DEFAULT 150.0,
  s_ma_period     INTEGER DEFAULT 50,
  l_min_rs        INTEGER DEFAULT 80,
  i_min_delta     INTEGER DEFAULT 1,
  created_at      TIMESTAMP DEFAULT NOW()
);

-- 插入奥尼尔默认模版
INSERT INTO threshold_presets (name, is_default) VALUES ('奥尼尔默认', TRUE);
```

---

## 后端 API 规范

所有接口返回 JSON，统一响应格式：
```json
{
  "success": true,
  "data": {},
  "error": null
}
```

### 接口列表

#### 1. 触发筛选（SSE 流式响应）
```
GET /api/v1/screener/scan?market=SP500
```
- 使用 Server-Sent Events（SSE）流式推送结果
- 每支股票计算完成后立即推送一条事件
- 事件格式：
```json
{
  "type": "stock_result",
  "data": {
    "symbol": "ABCD",
    "name": "某公司",
    "market": "SP500",
    "metrics": {
      "c": { "value": 47.3, "sufficient": true },
      "a": { "value": [32.1, 28.4, 25.9], "sufficient": true },
      "s": { "value": 183.2, "sufficient": true },
      "l": { "value": 88, "sufficient": true },
      "i": { "value": 12, "sufficient": true },
      "n": null
    },
    "data_as_of": "2025-03-31"
  }
}
```
- 结束事件：`{ "type": "scan_complete", "data": { "total": 500, "analyzed": 500 } }`

#### 2. 获取缓存结果
```
GET /api/v1/screener/results?market=SP500
```
- 返回最近一次筛选的缓存数据
- 包含 `data_as_of` 字段表示数据截止日期

#### 3. 刷新数据
```
POST /api/v1/screener/refresh
Body: { "market": "SP500" }
```
- 触发后台重新从 FMP / Polygon 拉取数据
- 返回任务 ID，前端可轮询进度

#### 4. 获取阈值方案列表
```
GET /api/v1/presets
```

#### 5. 新建/保存阈值方案
```
POST /api/v1/presets
Body: {
  "name": "激进版",
  "c_min_growth": 40,
  "a_min_growth": 30,
  "a_min_years": 3,
  "s_volume_ratio": 200,
  "s_ma_period": 50,
  "l_min_rs": 85,
  "i_min_delta": 3
}
```

#### 6. 删除阈值方案
```
DELETE /api/v1/presets/:id
```
- 不允许删除 is_default=true 的默认方案

#### 7. N 标签写回（供模块二调用）
```
PATCH /api/v1/stocks/:symbol/n-label
Body: { "n_label": true, "analysis_summary": "..." }
```

---

## 前端页面结构

### 路由
```
/screener          → 选股器主页面
/analysis/:symbol  → 个股分析页（多标签页容器）
```

### 页面布局（选股器主页面）

```
┌─────────────────────────────────────────────────────┐
│  顶部导航栏（应用名称 + 模块切换）                      │
├─────────────────────────────────────────────────────┤
│  ① 市场选择区                                         │
│  [标普500]  [纳指100]  [罗素2000]                     │
├─────────────────────────────────────────────────────┤
│  ② 阈值配置面板（默认收起，点击展开）                   │
│  ▶ 筛选参数配置                     [奥尼尔默认] [保存] │
│  ┌──────────────────────────────────────────────┐   │
│  │ C: 当季EPS增长 ≥ [25]%                        │   │
│  │ A: 年度EPS增长 ≥ [25]% · 连续 [3] 年          │   │
│  │ S: 突破成交量 ≥ 均量 [150]% · 均量周期 [50] 日  │   │
│  │ L: RS评级 ≥ [80]                             │   │
│  │ I: 机构持仓净增加 ≥ [1] 家（季度环比）           │   │
│  └──────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│  ③ 操作栏                                            │
│  [开始筛选]  已分析 347/500 ████████░░              │
│  数据截止：2025年03月31日（基于最新财报）  [刷新数据]   │
├─────────────────────────────────────────────────────┤
│  ④ 标签过滤栏                                         │
│  筛选条件：[C] [A] [S] [L] [I]（可多选，AND逻辑）     │
├─────────────────────────────────────────────────────┤
│  ⑤ 股票列表                                          │
│  代码    名称           标签              市场         │
│  ABCD   某某公司        ✓C ✓A ✓L ?S ·I   罗素2000    │
│  EFGH   另一公司        ✓C ✓A ✓S ✓L ✓I  标普500     │
│  ...                                                 │
└─────────────────────────────────────────────────────┘
```

### 个股分析页（多标签页容器）

```
┌─────────────────────────────────────────────────────┐
│  顶部导航栏                                           │
├─────────────────────────────────────────────────────┤
│  [← 返回选股器]                                       │
│  [ABCD · 某某公司 ×] [EFGH · 另一公司 ×] [+]         │
├─────────────────────────────────────────────────────┤
│                                                     │
│   个股分析内容区（模块二实现，本期预留空白占位）          │
│                                                     │
└─────────────────────────────────────────────────────┘
```

---

## 前端核心状态设计（Zustand）

```typescript
interface ScreenerStore {
  // 市场选择
  selectedMarket: 'SP500' | 'NASDAQ100' | 'RUSSELL2000' | null;
  setMarket: (market: string) => void;

  // 阈值配置
  thresholds: ThresholdConfig;
  setThresholds: (t: Partial<ThresholdConfig>) => void;
  activePresetId: number | null;
  presets: ThresholdPreset[];

  // 阈值面板开关
  thresholdPanelOpen: boolean;
  toggleThresholdPanel: () => void;

  // 原始数据（从后端获取，存储原始数值）
  rawStocks: StockWithMetrics[];
  scanProgress: { analyzed: number; total: number };
  dataAsOf: string | null;
  isScanning: boolean;

  // 标签过滤（纯前端计算）
  activeTagFilters: ('C' | 'A' | 'S' | 'L' | 'I')[];
  toggleTagFilter: (tag: string) => void;

  // 计算属性：带标签的股票列表（由原始数据 + 阈值实时计算）
  // 使用 useMemo 或 selector 实现，不存储在 store 中
}

interface ThresholdConfig {
  c_min_growth: number;      // 默认 25
  a_min_growth: number;      // 默认 25
  a_min_years: number;       // 默认 3
  s_volume_ratio: number;    // 默认 150
  s_ma_period: number;       // 默认 50
  l_min_rs: number;          // 默认 80
  i_min_delta: number;       // 默认 1
}
```

---

## 标签计算逻辑（前端实现）

```typescript
// 根据原始数值和当前阈值计算标签
function computeLabel(
  metric: StockMetric,
  thresholds: ThresholdConfig
): 'pass' | 'fail' | 'insufficient' {
  if (!metric.sufficient) return 'insufficient';
  // 根据各维度阈值判断
  return metric.value >= threshold ? 'pass' : 'fail';
}

// 权重配置（不对用户展示）
const WEIGHTS = { C: 3, A: 3, L: 2, S: 1, I: 1 };

// 排序：先按达标标签数降序，再按权重和降序
function sortStocks(stocks: ComputedStock[]): ComputedStock[] {
  return stocks.sort((a, b) => {
    const aCount = a.passCount - b.passCount;
    if (aCount !== 0) return -aCount;
    return b.weightSum - a.weightSum;
  });
}
```

---

## 多标签页管理（个股分析页）

```typescript
interface TabStore {
  tabs: StockTab[];
  activeTabId: string | null;

  openTab: (symbol: string, name: string) => void;
  // 同一 symbol 已存在则激活，不新建
  closeTab: (symbol: string) => void;
  setActiveTab: (symbol: string) => void;
}

interface StockTab {
  symbol: string;
  name: string;
  openedAt: number;
}
```

---

## 异常处理要求

| 场景 | 前端行为 | 提示文字 |
|------|---------|---------|
| 数据源超时/报错 | 展示缓存数据（如有），否则展示空状态 | 「数据获取失败，请检查网络连接」 |
| 数据不足（某维度） | 标签显示「?」徽章，hover 展示 tooltip | 「数据不足，谨慎参考，基准期以 0 计算」 |
| 缓存数据过期 | 静默展示，不拦截 | 顶部显示数据截止时间戳，用户自行判断 |
| 筛选结果为零 | 空状态图示 | 「当前市场暂无符合条件的股票」 |
| 标签过滤后结果为零 | 空状态图示 | 「没有同时满足所选条件的股票，可尝试减少选中标签」 |
| 阈值输入超出合理范围 | 输入框红色边框，不阻止 | 「超出建议范围（X~Y），请确认是否继续」 |
| 切换市场时已有结果 | 弹出确认对话框 | 「切换市场将清空当前结果，是否继续？」 |

---

## UI/UX 细节要求

1. **配色**：深色专业金融风格，主色调深蓝（#1E3A5F），辅色蓝（#2E5FA3），背景深灰
2. **标签徽章**：
   - 达标（pass）：绿色实心徽章，显示字母（如「C」）
   - 未达标（fail）：灰色空心徽章
   - 数据不足（insufficient）：黄色「?」徽章
3. **阈值面板**：展开/收起使用平滑动画（200ms ease）
4. **流式加载**：列表条目逐条出现，使用 fade-in 动画（100ms）
5. **进度条**：显示「已分析 X / Y 支」，使用 shadcn/ui 的 Progress 组件
6. **标签过滤按钮**：点亮状态使用主色蓝背景 + 白字，熄灭状态使用灰色边框 + 灰字
7. **空状态**：使用图示 + 说明文字，不展示空表格
8. **Tooltip**：数据不足的「?」徽章 hover 时展示原因说明

---

## 文件结构建议

```
frontend/
├── src/
│   ├── pages/
│   │   ├── Screener/
│   │   │   ├── index.tsx              # 选股器主页面
│   │   │   ├── MarketSelector.tsx     # 市场选择组件
│   │   │   ├── ThresholdPanel.tsx     # 阈值配置面板
│   │   │   ├── StockList.tsx          # 股票列表
│   │   │   ├── TagFilter.tsx          # 标签过滤栏
│   │   │   └── ScanProgress.tsx       # 进度条组件
│   │   └── Analysis/
│   │       ├── index.tsx              # 个股分析页（多标签页容器）
│   │       └── TabBar.tsx             # 标签页导航栏
│   ├── stores/
│   │   ├── screenerStore.ts           # 选股器状态
│   │   └── tabStore.ts                # 标签页状态
│   ├── lib/
│   │   ├── computeLabels.ts           # 标签计算逻辑
│   │   ├── sortStocks.ts              # 排序逻辑
│   │   └── api.ts                     # API 请求封装
│   └── types/
│       └── index.ts                   # 类型定义

backend/
├── cmd/
│   └── server/main.go
├── internal/
│   ├── handlers/
│   │   ├── screener.go                # 筛选接口
│   │   └── presets.go                 # 阈值方案接口
│   ├── services/
│   │   ├── scanner.go                 # 筛选核心逻辑
│   │   ├── fmp.go                     # FMP 数据源
│   │   └── polygon.go                 # Polygon 数据源
│   ├── models/
│   │   └── stock.go                   # 数据模型
│   └── db/
│       └── postgres.go                # 数据库连接
└── config/
    └── config.go
```

---

## 实现优先级

### Phase 1（核心可用）
1. 后端：数据库 Schema + FMP/Polygon 数据拉取 + SSE 流式推送接口
2. 前端：市场选择 + 触发筛选 + 流式列表展示 + 标签过滤
3. 使用奥尼尔默认阈值，阈值面板暂不实现

### Phase 2（完整功能）
4. 前端：阈值配置面板 + 自定义方案保存
5. 后端：阈值方案 CRUD 接口
6. 前端：多标签页容器 + 导航联动

### Phase 3（体验完善）
7. 数据缓存与时间戳标注
8. 刷新数据功能
9. 所有异常处理与空状态
10. N 标签写回接口（供模块二调用）

---

## 注意事项

- **所有 UI 文字使用中文**，包括错误提示、按钮文字、标签说明、tooltip 内容
- 标签计算必须在前端进行，后端只存原始数值
- SSE 连接断开时前端应自动重连（最多 3 次），超过后提示用户
- 阈值「奥尼尔默认」方案不可删除（is_default=true 保护）
- 同一 symbol 在分析页不重复开 Tab，而是激活已有 Tab
- RS 评级计算口径需与团队对齐后再实现，暂可用价格相对标普 500 的 12 个月相对涨幅百分位近似替代
