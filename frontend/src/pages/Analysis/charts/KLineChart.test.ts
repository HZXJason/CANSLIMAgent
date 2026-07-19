import { describe, expect, it } from 'vitest';
import { KLINE_MARKET_COLORS } from './KLineChart';

describe('KLineChart market colors', () => {
  it('uses US green-up/red-down and A-share red-up/green-down independently of the global theme', () => {
    expect(KLINE_MARKET_COLORS.US).toMatchObject({ up: '#26a69a', down: '#ef5350' });
    expect(KLINE_MARKET_COLORS.CN).toMatchObject({ up: '#ef5350', down: '#26a69a' });
  });
});
