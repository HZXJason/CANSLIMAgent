import type { StockMetrics } from './index';

export type KLinePeriod = 'daily' | 'weekly' | 'monthly';
export type ChartStatus = 'loading' | 'success' | 'error' | 'insufficient';
export type AIStatus = 'idle' | 'streaming' | 'paused' | 'done' | 'error';
export type AIProgressStatus = 'pending' | 'running' | 'paused' | 'done' | 'error';

export interface AnalysisConfig {
  llm_provider: string;
  deep_think_llm: string;
  quick_think_llm: string;
  backend_url: string;
  max_debate_rounds: number;
  max_risk_rounds: number;
  online_tools: boolean;
  analysts: string[];
  api_keys: Record<string, string>;
}

export interface AIKLineContext {
  symbol: string;
  period: KLinePeriod;
  source: string;
  data_as_of: string;
  recent_candles: Candle[];
}

export interface AIAnalysisContext {
  stock_summary: StockSummary | null;
  company_profile: CompanyProfile | null;
  second_layer_data: SecondLayerData | null;
  kline: AIKLineContext | null;
  rs: RSData | null;
  eps: EPSData | null;
  revenue: RevenueData | null;
  gross_margin: GrossMarginData | null;
  chart_status: Record<string, ChartStatus>;
  derived: Record<string, unknown>;
}

export interface SecondLayerData {
  symbol?: string;
  name?: string;
  source?: string;
  data_as_of?: string | null;
  company?: Record<string, unknown>;
  supply?: Record<string, unknown>;
  valuation?: Record<string, unknown>;
  analyst_expectations?: Record<string, unknown>;
  financial_quality?: Record<string, unknown>;
  events?: Record<string, unknown>;
  earnings_history?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export interface AIProviderConfig {
  id: string;
  name: string;
  models: {
    deep: string[];
    quick: string[];
  };
  custom_models?: boolean;
  default_backend_url?: string;
}

export interface AIAnalystConfig {
  id: string;
  name: string;
  description: string;
  default_enabled?: boolean;
}

export interface AIAnalysisConfigResponse {
  providers: AIProviderConfig[];
  analysts: AIAnalystConfig[];
  debate_rounds: number[];
  risk_rounds: number[];
}

export interface StockSummary {
  symbol: string;
  name: string;
  market: string;
  metrics: StockMetrics;
  data_as_of: string;
  n_analysis_at: string | null;
  n_analysis_summary: string;
}

export interface CompanyProfile {
  symbol: string;
  name: string;
  description: string;
  sector: string;
  industry: string;
  website: string;
  source: string;
  updated_at: string;
}

export interface Candle {
  date: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface KLineData {
  symbol: string;
  period: KLinePeriod;
  source: string;
  data_as_of: string;
  candles: Candle[];
}

export interface RSPoint {
  date: string;
  value: number;
}

export interface RSData {
  symbol: string;
  period: KLinePeriod;
  source: string;
  data_as_of: string;
  rs_line: RSPoint[];
}

export interface EPSQuarter {
  period: string;
  eps: number;
  eps_yoy_growth: number;
  data_sufficient: boolean;
  report_date: string;
}

export interface EPSData {
  symbol: string;
  source: string;
  data_as_of: string;
  quarters: EPSQuarter[];
}

export interface RevenueYear {
  year: number;
  revenue: number;
  revenue_yoy_growth: number;
  data_sufficient: boolean;
}

export interface RevenueData {
  symbol: string;
  source: string;
  data_as_of: string;
  years: RevenueYear[];
}

export interface GrossMarginQuarter {
  period: string;
  gross_margin: number;
  report_date: string;
}

export interface GrossMarginData {
  symbol: string;
  source: string;
  data_as_of: string;
  quarters: GrossMarginQuarter[];
}

export interface ThoughtLine {
  agent: string;
  content: string;
  timestamp: number;
}

export interface AIAgentProgress {
  agent: string;
  status: AIProgressStatus;
  message: string;
  timestamp: number;
}

export interface AIAgentResult {
  agent: string;
  content: string;
  timestamp: number;
}

export interface AIDecision {
  action: 'BUY' | 'SELL' | 'HOLD' | string;
  confidence: 'HIGH' | 'MEDIUM' | 'LOW' | string;
  summary: string;
}

export interface SymbolAnalysisState {
  stockSummary: StockSummary | null;
  companyProfile: CompanyProfile | null;
  klineData: KLineData | null;
  klinePeriod: KLinePeriod;
  rsData: RSData | null;
  epsData: EPSData | null;
  revenueData: RevenueData | null;
  grossMarginData: GrossMarginData | null;
  secondLayerData: SecondLayerData | null;
  chartStatus: {
    summary: ChartStatus;
    profile: ChartStatus;
    kline: ChartStatus;
    rs: ChartStatus;
    eps: ChartStatus;
    revenue: ChartStatus;
    grossMargin: ChartStatus;
    secondLayer: ChartStatus;
  };
  aiStatus: AIStatus;
  thoughtLines: ThoughtLine[];
  liveNarration: string;
  liveNarrationByAgent: Record<string, string>;
  reportContent: string;
  aiProgress: AIAgentProgress[];
  aiResults: AIAgentResult[];
  aiDecision: AIDecision | null;
  aiError: string | null;
  thoughtExpanded: boolean;
  nLabelWritten: boolean;
}

export interface NLabelResponse {
  symbol: string;
  n_label: boolean;
  n_analysis_at: string;
}
