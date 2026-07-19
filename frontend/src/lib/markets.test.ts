import { describe, expect, it } from 'vitest';
import {
  SCREENER_MARKETS,
  assetMarketForScreenerMarket,
  assetMarketForSymbol,
  displaySymbol,
  normalizeSymbolForAsset,
  screenerMarketLabel
} from './markets';

describe('market helpers', () => {
  it('keeps US symbols uppercase without adding an exchange suffix', () => {
    expect(normalizeSymbolForAsset('US', ' brk.b ')).toBe('BRK.B');
    expect(normalizeSymbolForAsset('US', 'aapl')).toBe('AAPL');
    expect(normalizeSymbolForAsset('US', 'bf-b')).toBe('BF-B');
  });

  it('rejects A-share identities while the US market is selected', () => {
    expect(normalizeSymbolForAsset('US', '600519')).toBe('');
    expect(normalizeSymbolForAsset('US', '600519.SH')).toBe('');
    expect(normalizeSymbolForAsset('US', '600519.SS')).toBe('');
    expect(normalizeSymbolForAsset('US', 'SH600519')).toBe('');
  });

  it('canonicalizes six-digit A-share symbols by exchange prefix', () => {
    expect(normalizeSymbolForAsset('CN', '600519')).toBe('600519.SH');
    expect(normalizeSymbolForAsset('CN', '688981')).toBe('688981.SH');
    expect(normalizeSymbolForAsset('CN', '000001')).toBe('000001.SZ');
    expect(normalizeSymbolForAsset('CN', '300750')).toBe('300750.SZ');
    expect(normalizeSymbolForAsset('CN', '830799')).toBe('830799.BJ');
    expect(normalizeSymbolForAsset('CN', '920000')).toBe('920000.BJ');
  });

  it('normalizes Yahoo Shanghai suffixes and preserves existing canonical suffixes', () => {
    expect(normalizeSymbolForAsset('CN', '600519.ss')).toBe('600519.SH');
    expect(normalizeSymbolForAsset('CN', '000001.sz')).toBe('000001.SZ');
    expect(normalizeSymbolForAsset('CN', '830799.bj')).toBe('830799.BJ');
    expect(normalizeSymbolForAsset('CN', 'sh600519')).toBe('600519.SH');
    expect(normalizeSymbolForAsset('CN', 'SZ:000001')).toBe('000001.SZ');
    expect(normalizeSymbolForAsset('CN', 'BJ.830799')).toBe('830799.BJ');
  });

  it('rejects US and malformed identities while the A-share market is selected', () => {
    expect(normalizeSymbolForAsset('CN', 'AAPL')).toBe('');
    expect(normalizeSymbolForAsset('CN', 'BRK.B')).toBe('');
    expect(normalizeSymbolForAsset('CN', '60051')).toBe('');
    expect(normalizeSymbolForAsset('CN', '600519.HK')).toBe('');
    expect(normalizeSymbolForAsset('CN', '123456')).toBe('');
    expect(normalizeSymbolForAsset('CN', '900901.SH')).toBe('');
    expect(normalizeSymbolForAsset('CN', '200002.SZ')).toBe('');
    expect(normalizeSymbolForAsset('CN', '000001.SH')).toBe('');
    expect(normalizeSymbolForAsset('CN', 'SH000001')).toBe('');
    expect(normalizeSymbolForAsset('CN', 'BJ600519')).toBe('');
  });

  it('detects the asset market from a valid symbol without using global UI state', () => {
    expect(assetMarketForSymbol('600519.SH')).toBe('CN');
    expect(assetMarketForSymbol('SZ000001')).toBe('CN');
    expect(assetMarketForSymbol('AAPL')).toBe('US');
    expect(assetMarketForSymbol('BRK.B')).toBe('US');
    expect(assetMarketForSymbol('not valid!')).toBeNull();
  });

  it('keeps data identities canonical while presenting A-share codes without suffixes', () => {
    expect(displaySymbol('600519.SH')).toBe('600519');
    expect(displaySymbol('AAPL')).toBe('AAPL');
    expect(displaySymbol('AAPL.SH')).toBe('AAPL.SH');
  });

  it('keeps a centralized mapping between asset markets and screener markets', () => {
    expect(SCREENER_MARKETS.US).toHaveLength(3);
    expect(SCREENER_MARKETS.CN.map((market) => market.label)).toEqual(['沪市主板', '深市主板', '科创板', '创业板', '北交所']);
    expect(assetMarketForScreenerMarket('STAR')).toBe('CN');
    expect(assetMarketForScreenerMarket('NASDAQ100')).toBe('US');
    expect(screenerMarketLabel('CHINEXT')).toBe('创业板');
  });
});
