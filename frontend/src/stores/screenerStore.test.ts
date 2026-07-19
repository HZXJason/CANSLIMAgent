import { beforeEach, describe, expect, it } from 'vitest';
import { DEFAULT_SCREENER_MARKET } from '../lib/markets';
import type { Market, StockWithMetrics } from '../types';
import { useScreenerStore } from './screenerStore';

function stock(market: Market, symbol: string): StockWithMetrics {
  return {
    market,
    symbol,
    name: symbol,
    metrics: {
      c: { value: 30, sufficient: true },
      a: { value: [30, 30, 30], sufficient: true },
      s: { value: 160, sufficient: true },
      l: { value: 90, sufficient: true },
      i: { value: 2, sufficient: true },
      n: null
    },
    ai_analyzed: false,
    data_as_of: '2026-07-19'
  };
}

describe('screener store market state', () => {
  beforeEach(() => {
    useScreenerStore.setState({
      selectedMarkets: { ...DEFAULT_SCREENER_MARKET },
      rawStocks: [],
      activeTagFilters: [],
      scanProgress: { analyzed: 0, total: 0 },
      dataAsOf: null
    });
  });

  it('remembers one selected screener market per asset market', () => {
    const store = useScreenerStore.getState();
    store.setMarket('US', 'NASDAQ100');
    store.setMarket('CN', 'CHINEXT');

    expect(useScreenerStore.getState().selectedMarkets).toEqual({ US: 'NASDAQ100', CN: 'CHINEXT' });
  });

  it('retains results and CANSLIM filters while switching to an A-share board', () => {
    const store = useScreenerStore.getState();
    store.setMarketStocks('SP500', [stock('SP500', 'AAPL')]);
    store.setMarketStocks('STAR', [stock('STAR', '688981.SS')]);
    store.toggleTagFilter('C');
    store.setMarket('CN', 'STAR');

    const state = useScreenerStore.getState();
    expect(state.rawStocks.map((item) => item.market + ':' + item.symbol)).toEqual([
      'SP500:AAPL',
      'STAR:688981.SH'
    ]);
    expect(state.activeTagFilters).toEqual(['C']);
  });

  it('drops cross-market or invalid symbols instead of creating empty identities', () => {
    const store = useScreenerStore.getState();
    store.setMarketStocks('STAR', [stock('STAR', 'AAPL'), stock('STAR', '900901.SH')]);
    store.addStockResult(stock('SP500', '600519.SH'));

    expect(useScreenerStore.getState().rawStocks).toEqual([]);
  });
});
