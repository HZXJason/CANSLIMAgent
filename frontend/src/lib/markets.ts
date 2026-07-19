import type { AssetMarket, Market } from '../types';

export interface AssetMarketOption {
  value: AssetMarket;
  label: string;
  compactLabel: string;
}

export interface ScreenerMarketOption {
  value: Market;
  label: string;
  compactLabel: string;
}

export const ASSET_MARKET_OPTIONS: readonly AssetMarketOption[] = [
  { value: 'US', label: '美股市场', compactLabel: '美股' },
  { value: 'CN', label: 'A股市场', compactLabel: 'A股' }
];

export const SCREENER_MARKETS: Record<AssetMarket, readonly ScreenerMarketOption[]> = {
  US: [
    { value: 'SP500', label: '标普 500', compactLabel: '标普500' },
    { value: 'NASDAQ100', label: '纳指 100', compactLabel: '纳指100' },
    { value: 'RUSSELL2000', label: '罗素 2000', compactLabel: '罗素2000' }
  ],
  CN: [
    { value: 'SSE_MAIN', label: '沪市主板', compactLabel: '沪市主板' },
    { value: 'SZSE_MAIN', label: '深市主板', compactLabel: '深市主板' },
    { value: 'STAR', label: '科创板', compactLabel: '科创板' },
    { value: 'CHINEXT', label: '创业板', compactLabel: '创业板' },
    { value: 'BSE', label: '北交所', compactLabel: '北交所' }
  ]
};

export const DEFAULT_SCREENER_MARKET: Record<AssetMarket, Market> = {
  US: 'SP500',
  CN: 'SSE_MAIN'
};

const screenerMarketLabels = new Map(
  Object.values(SCREENER_MARKETS)
    .flat()
    .map((market) => [market.value, market] as const)
);

const CN_SYMBOL_WITH_SUFFIX = /^(\d{6})\.(SH|SZ|BJ|SS)$/;
const CN_SYMBOL_WITH_PREFIX = /^(SH|SZ|BJ)[.:-]?(\d{6})$/;
const US_SYMBOL = /^[A-Z][A-Z0-9]{0,9}(?:[.-][A-Z0-9]{1,5})?$/;

export function assetMarketForScreenerMarket(market: Market): AssetMarket {
  return SCREENER_MARKETS.CN.some((option) => option.value === market) ? 'CN' : 'US';
}

export function screenerMarketLabel(market: Market, compact = false) {
  const option = screenerMarketLabels.get(market);
  return compact ? option?.compactLabel ?? market : option?.label ?? market;
}

function cnExchangeForCode(symbol: string): 'SH' | 'SZ' | 'BJ' | null {
  const prefix = symbol.slice(0, 3);
  if (['600', '601', '603', '605', '688', '689'].includes(prefix)) return 'SH';
  if (['000', '001', '002', '003', '300', '301'].includes(prefix)) return 'SZ';
  if (prefix === '920' || /^[48]/.test(symbol)) return 'BJ';
  return null;
}

export function normalizeSymbolForAsset(assetMarket: AssetMarket, value: string) {
  const symbol = value.trim().toUpperCase();
  if (!symbol) return '';
  const suffixMatch = symbol.match(CN_SYMBOL_WITH_SUFFIX);
  const prefixMatch = symbol.match(CN_SYMBOL_WITH_PREFIX);

  if (assetMarket === 'CN') {
    if (/^\d{6}$/.test(symbol)) {
      const exchange = cnExchangeForCode(symbol);
      return exchange ? symbol + '.' + exchange : '';
    }
    if (suffixMatch) {
      const [, code, suffix] = suffixMatch;
      const exchange = suffix === 'SS' ? 'SH' : suffix;
      return cnExchangeForCode(code) === exchange ? code + '.' + exchange : '';
    }
    if (prefixMatch) {
      const [, prefix, code] = prefixMatch;
      return cnExchangeForCode(code) === prefix ? code + '.' + prefix : '';
    }
    return '';
  }

  if (/^\d{6}$/.test(symbol) || suffixMatch || prefixMatch) return '';
  return US_SYMBOL.test(symbol) ? symbol : '';
}

export function assetMarketForSymbol(value: string): AssetMarket | null {
  if (normalizeSymbolForAsset('CN', value)) return 'CN';
  if (normalizeSymbolForAsset('US', value)) return 'US';
  return null;
}

export function displaySymbol(value: string) {
  const symbol = value.trim().toUpperCase();
  const canonicalAshare = normalizeSymbolForAsset('CN', symbol);
  return canonicalAshare ? canonicalAshare.slice(0, 6) : symbol;
}
