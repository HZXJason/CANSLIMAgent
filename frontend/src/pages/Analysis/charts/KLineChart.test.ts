import { describe, expect, it } from 'vitest';
import * as KLineChartModule from './KLineChart';
import { KLINE_MARKET_COLORS } from './KLineChart';

describe('KLineChart market colors', () => {
  it('uses US green-up/red-down and A-share red-up/green-down independently of the global theme', () => {
    expect(KLINE_MARKET_COLORS.US).toMatchObject({ up: '#26a69a', down: '#ef5350' });
    expect(KLINE_MARKET_COLORS.CN).toMatchObject({ up: '#ef5350', down: '#26a69a' });
  });
});

describe('KLineChart dense daily candles', () => {
  it('keeps the candle body and wick within a narrow category slot', () => {
    const getCandleVisualMetrics = (KLineChartModule as Record<string, unknown>).getCandleVisualMetrics;

    expect(getCandleVisualMetrics).toBeTypeOf('function');
    expect((getCandleVisualMetrics as (slotWidth: number) => { bodyWidth: number; wickWidth: number })(1.5)).toEqual(
      expect.objectContaining({
        bodyWidth: expect.any(Number),
        wickWidth: expect.any(Number)
      })
    );

    const metrics = (getCandleVisualMetrics as (slotWidth: number) => { bodyWidth: number; wickWidth: number })(1.5);
    expect(metrics.bodyWidth).toBeLessThan(1.5);
    expect(metrics.wickWidth).toBeLessThanOrEqual(1.5);
  });

  it('overlays price and volume bars in one period-appropriate slot', () => {
    const getKLineBarLayout = (KLineChartModule as Record<string, unknown>).getKLineBarLayout;

    expect(getKLineBarLayout).toBeTypeOf('function');
    expect((getKLineBarLayout as (period: string) => { barSize?: number; barGap?: number })('daily')).toEqual({
      barSize: 1,
      barGap: -1
    });
    expect((getKLineBarLayout as (period: string) => { barSize?: number; barGap?: number })('weekly')).toEqual({
      barSize: 4,
      barGap: -4
    });
    expect((getKLineBarLayout as (period: string) => { barSize?: number; barGap?: number })('monthly')).toEqual({
      barSize: 8,
      barGap: -8
    });
  });
});
