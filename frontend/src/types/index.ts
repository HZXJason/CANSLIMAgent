export type Market = 'SP500' | 'NASDAQ100' | 'RUSSELL2000';
export type LabelKey = 'C' | 'A' | 'S' | 'L' | 'I';
export type LabelStatus = 'pass' | 'fail' | 'insufficient';

export interface Metric<T> {
  value: T;
  sufficient: boolean;
}

export interface StockMetrics {
  c: Metric<number>;
  a: Metric<number[]>;
  s: Metric<number>;
  l: Metric<number>;
  i: Metric<number>;
  n: boolean | null;
}

export interface StockWithMetrics {
  symbol: string;
  name: string;
  market: Market;
  metrics: StockMetrics;
  data_as_of: string;
}

export interface ThresholdConfig {
  c_min_growth: number;
  a_min_growth: number;
  a_min_years: number;
  s_volume_ratio: number;
  s_ma_period: number;
  l_min_rs: number;
  i_min_delta: number;
}

export interface ThresholdPreset extends ThresholdConfig {
  id: number;
  name: string;
  is_default: boolean;
  created_at: string;
}

export interface LabelResult {
  key: LabelKey;
  status: LabelStatus;
  value: number | number[];
  sufficient: boolean;
}

export interface ComputedStock extends StockWithMetrics {
  labels: Record<LabelKey, LabelResult>;
  passCount: number;
  weightSum: number;
}

export interface ScanProgress {
  analyzed: number;
  total: number;
}

export type ScanEvent =
  | { type: 'stock_result'; data: StockWithMetrics }
  | { type: 'scan_progress'; data: ScanProgress }
  | { type: 'scan_complete'; data: ScanProgress }
  | { type: 'scan_error'; data: { message: string } };

export interface APIResponse<T> {
  success: boolean;
  data: T;
  error: string | null;
}

export interface RefreshJobResponse {
  job_id: string;
  status: string;
}

export interface RefreshJob {
  id: string;
  market: Market;
  status: 'running' | 'completed' | 'failed';
  error?: string;
  created_at: string;
  finished_at?: string;
}
