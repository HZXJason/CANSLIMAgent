import type { ChartStatus, KLinePeriod } from '../../../types/analysis';

export const chartTextColor = '#8EA3B8';
export const chartGridColor = 'rgba(142, 163, 184, 0.18)';

export function formatAxisDate(value: string, period: KLinePeriod) {
  if (!value) return '';
  const [year, month, day] = value.split('-');
  if (period === 'daily') return month + '/' + day;
  return year + '/' + month;
}

export function formatCompactNumber(value: number) {
  if (!Number.isFinite(value)) return '暂无';
  const abs = Math.abs(value);
  if (abs >= 1_000_000_000_000) return (value / 1_000_000_000_000).toFixed(1) + '万亿';
  if (abs >= 100_000_000) return (value / 100_000_000).toFixed(1) + '亿';
  if (abs >= 10_000) return (value / 10_000).toFixed(1) + '万';
  return value.toFixed(0);
}

export function statusFromLength(status: ChartStatus, length: number) {
  if (status === 'success' && length === 0) return 'insufficient';
  return status;
}
