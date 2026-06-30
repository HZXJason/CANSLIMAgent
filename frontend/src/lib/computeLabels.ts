import type { ComputedStock, LabelKey, LabelResult, LabelStatus, Metric, StockWithMetrics, ThresholdConfig } from '../types';

export const WEIGHTS: Record<LabelKey, number> = { C: 3, A: 3, L: 2, S: 1, I: 1 };
export const ACTIVE_LABEL_KEYS: LabelKey[] = ['C', 'A', 'S', 'L'];

function scalarLabel(key: LabelKey, metric: Metric<number>, threshold: number): LabelResult {
  if (!metric.sufficient) {
    return { key, status: 'insufficient', value: metric.value, sufficient: false };
  }
  return {
    key,
    status: metric.value >= threshold ? 'pass' : 'fail',
    value: metric.value,
    sufficient: true
  };
}

function annualLabel(metric: Metric<number[]>, thresholds: ThresholdConfig): LabelResult {
  const values = Array.isArray(metric.value) ? metric.value : [];
  if (!metric.sufficient || values.length < thresholds.a_min_years) {
    return { key: 'A', status: 'insufficient', value: values, sufficient: false };
  }
  const selected = values.slice(0, thresholds.a_min_years);
  const pass = selected.every((value) => value >= thresholds.a_min_growth);
  return { key: 'A', status: pass ? 'pass' : 'fail', value: values, sufficient: true };
}

export function computeStockLabels(stock: StockWithMetrics, thresholds: ThresholdConfig): Record<LabelKey, LabelResult> {
  return {
    C: scalarLabel('C', stock.metrics.c, thresholds.c_min_growth),
    A: annualLabel(stock.metrics.a, thresholds),
    S: scalarLabel('S', stock.metrics.s, thresholds.s_volume_ratio),
    L: scalarLabel('L', stock.metrics.l, thresholds.l_min_rs),
    I: scalarLabel('I', stock.metrics.i, thresholds.i_min_delta)
  };
}

export function computeStock(stock: StockWithMetrics, thresholds: ThresholdConfig): ComputedStock {
  const labels = computeStockLabels(stock, thresholds);
  const entries = ACTIVE_LABEL_KEYS.map((key) => [key, labels[key]] as [LabelKey, LabelResult]);
  const passCount = entries.filter(([, label]) => label.status === 'pass').length;
  const weightSum = entries.reduce((sum, [key, label]) => (label.status === 'pass' ? sum + WEIGHTS[key] : sum), 0);
  return { ...stock, labels, passCount, weightSum };
}

export function labelStatusText(status: LabelStatus) {
  if (status === 'pass') return '达标';
  if (status === 'insufficient') return '数据不足';
  return '未达标';
}
