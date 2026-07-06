import type {
  AIAnalysisContext,
  Candle,
  CompanyProfile,
  EPSData,
  GrossMarginData,
  KLineData,
  RevenueData,
  RSData,
  SecondLayerData,
  SymbolAnalysisState
} from '../types/analysis';

const RECENT_CANDLE_LIMIT = 45;
const RECENT_RS_POINT_LIMIT = 80;
const PROFILE_DESCRIPTION_LIMIT = 1200;
const QUARTERLY_FINANCIAL_LIMIT = 8;
const EARNINGS_HISTORY_LIMIT = 8;
const SEC_EVENT_LIMIT = 12;
const SEC_INSIDER_LIMIT = 20;

function round(value: number | null | undefined, digits = 2) {
  if (value === null || value === undefined || !Number.isFinite(value)) return null;
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

function percentDiff(value: number | null, base: number | null) {
  if (value === null || base === null || base === 0) return null;
  return round(((value - base) / base) * 100);
}

function ratioPercent(value: number | null, base: number | null) {
  if (value === null || base === null || base === 0) return null;
  return round((value / base) * 100);
}

function average(values: number[]) {
  const clean = values.filter((value) => Number.isFinite(value));
  if (clean.length === 0) return null;
  return clean.reduce((sum, value) => sum + value, 0) / clean.length;
}

function latest<T>(items: T[]) {
  return items.length > 0 ? items[items.length - 1] : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function compactRecord(value: unknown, arrayLimits: Record<string, number>) {
  if (!isRecord(value)) return undefined;
  const next: Record<string, unknown> = { ...value };
  for (const [key, limit] of Object.entries(arrayLimits)) {
    if (Array.isArray(next[key])) next[key] = next[key].slice(0, limit);
  }
  return next;
}

function compactProfile(profile: CompanyProfile | null) {
  if (!profile) return null;
  return {
    ...profile,
    description:
      profile.description.length > PROFILE_DESCRIPTION_LIMIT
        ? profile.description.slice(0, PROFILE_DESCRIPTION_LIMIT) + '...'
        : profile.description
  };
}

function compactRSData(rs: RSData | null) {
  if (!rs) return null;
  return {
    ...rs,
    rs_line: rs.rs_line.slice(-RECENT_RS_POINT_LIMIT)
  };
}

function compactEPSData(data: EPSData | null) {
  if (!data) return null;
  return {
    ...data,
    quarters: data.quarters.slice(0, 8)
  };
}

function compactRevenueData(data: RevenueData | null) {
  if (!data) return null;
  return {
    ...data,
    years: data.years.slice(0, 6)
  };
}

function compactGrossMarginData(data: GrossMarginData | null) {
  if (!data) return null;
  return {
    ...data,
    quarters: data.quarters.slice(0, 8)
  };
}

function compactSecondLayerData(data: SecondLayerData | null): SecondLayerData | null {
  if (!data) return null;
  const sec = compactRecord(data.sec, {
    recent_event_filings: SEC_EVENT_LIMIT,
    insider_transactions: SEC_INSIDER_LIMIT
  });
  return {
    symbol: data.symbol,
    name: data.name,
    source: data.source,
    data_as_of: data.data_as_of,
    company: data.company,
    quote: data.quote,
    supply: data.supply,
    valuation: data.valuation,
    analyst_expectations: data.analyst_expectations,
    financial_quality: compactRecord(data.financial_quality, {
      quarterly: QUARTERLY_FINANCIAL_LIMIT
    }),
    events: data.events,
    earnings_history: Array.isArray(data.earnings_history)
      ? data.earnings_history.slice(0, EARNINGS_HISTORY_LIMIT)
      : data.earnings_history,
    sec
  };
}

function recentCandles(data: KLineData | null, count = RECENT_CANDLE_LIMIT) {
  return (data?.candles ?? []).slice(-count);
}

function windowCandles(candles: Candle[], count: number, includeLatest = true) {
  const source = includeLatest ? candles : candles.slice(0, -1);
  return source.slice(-count);
}

function movingAverage(candles: Candle[], count: number) {
  const window = windowCandles(candles, count);
  if (window.length < count) return null;
  return average(window.map((candle) => candle.close));
}

function averageVolume(candles: Candle[], count: number, includeLatest = false) {
  const window = windowCandles(candles, count, includeLatest);
  if (window.length === 0) return null;
  return average(window.map((candle) => candle.volume));
}

function averageDollarVolume(candles: Candle[], count: number) {
  const window = windowCandles(candles, count, false);
  if (window.length === 0) return null;
  return average(window.map((candle) => candle.close * candle.volume));
}

function high(candles: Candle[]) {
  if (candles.length === 0) return null;
  return Math.max(...candles.map((candle) => candle.high));
}

function low(candles: Candle[]) {
  if (candles.length === 0) return null;
  return Math.min(...candles.map((candle) => candle.low));
}

function rsSummary(rs: RSData | null) {
  const line = rs?.rs_line ?? [];
  const last = latest(line);
  const rsHigh = line.length > 0 ? Math.max(...line.map((point) => point.value)) : null;
  const rs20 = line.length > 20 ? line[line.length - 21] : null;
  const distanceFromHigh = last && rsHigh ? round(last.value - rsHigh) : null;

  return {
    latest_rs: round(last?.value),
    latest_rs_date: last?.date ?? null,
    rs_high_in_series: round(rsHigh),
    rs_distance_from_series_high_points: distanceFromHigh,
    rs_near_series_high: distanceFromHigh !== null ? distanceFromHigh >= -3 : null,
    rs_20_period_change_points: last && rs20 ? round(last.value - rs20.value) : null,
    rs_source: rs?.source ?? null,
    rs_data_as_of: rs?.data_as_of ?? null
  };
}

function technicalSummary(kline: KLineData | null, rs: RSData | null) {
  const candles = kline?.candles ?? [];
  const last = latest(candles);
  const previous = candles.length > 1 ? candles[candles.length - 2] : null;
  const sma50 = round(movingAverage(candles, 50));
  const sma200 = round(movingAverage(candles, 200));
  const avgVolume50 = round(averageVolume(candles, 50));
  const avgDollarVolume20 = round(averageDollarVolume(candles, 20), 0);
  const avgDollarVolume50 = round(averageDollarVolume(candles, 50), 0);
  const high20 = round(high(windowCandles(candles, 20)));
  const high50 = round(high(windowCandles(candles, 50)));
  const high252 = round(high(windowCandles(candles, 252)));
  const low20 = round(low(windowCandles(candles, 20)));
  const low50 = round(low(windowCandles(candles, 50)));
  const low252 = round(low(windowCandles(candles, 252)));
  const prior50High = round(high(windowCandles(candles, 50, false)));
  const latestVolume = last ? last.volume : null;
  const latestDollarVolume = last ? round(last.close * last.volume, 0) : null;
  const volumeVs50 = ratioPercent(latestVolume, avgVolume50);
  const breakoutVolumeRatio = last && prior50High !== null && last.close > prior50High ? volumeVs50 : null;
  const highest20 = high(windowCandles(candles, 20));
  const failedBreakout =
    last && highest20 !== null && prior50High !== null
      ? highest20 > prior50High && last.close < prior50High
      : null;

  return {
    kline_source: kline?.source ?? null,
    kline_period: kline?.period ?? null,
    kline_data_as_of: kline?.data_as_of ?? null,
    candle_count: candles.length,
    current_price: round(last?.close),
    previous_close: round(previous?.close),
    latest_volume: latestVolume,
    latest_dollar_volume: latestDollarVolume,
    avg_dollar_volume_20d: avgDollarVolume20,
    avg_dollar_volume_50d: avgDollarVolume50,
    avg_volume_50d: avgVolume50,
    latest_volume_vs_50d_avg_pct: volumeVs50,
    sma_50: sma50,
    sma_200: sma200,
    price_vs_sma_50_pct: percentDiff(last?.close ?? null, sma50),
    price_vs_sma_200_pct: percentDiff(last?.close ?? null, sma200),
    high_20d: high20,
    low_20d: low20,
    high_50d: high50,
    low_50d: low50,
    high_52w: high252,
    low_52w: low252,
    distance_from_52w_high_pct: percentDiff(last?.close ?? null, high252),
    near_52w_high_within_5pct: last && high252 !== null ? percentDiff(last.close, high252)! >= -5 : null,
    breakout_point_proxy_50d_high: prior50High,
    breakout_volume_ratio_pct: breakoutVolumeRatio,
    failed_breakout_proxy: failedBreakout,
    support_levels: {
      twenty_day_low: low20,
      fifty_day_low: low50,
      sma_50: sma50,
      sma_200: sma200,
      fifty_two_week_low: low252
    },
    resistance_levels: {
      twenty_day_high: high20,
      fifty_day_high: high50,
      fifty_two_week_high: high252
    },
    ...rsSummary(rs)
  };
}

function firstLayerSummary(state: SymbolAnalysisState) {
  const metrics = state.stockSummary?.metrics;
  return {
    c_eps_growth_pct: metrics?.c ?? null,
    a_annual_growth_pct: metrics?.a ?? null,
    s_volume_ratio_pct: metrics?.s ?? null,
    l_rs_rating: metrics?.l ?? null,
    i_institution_delta: metrics?.i ?? null,
    n_label: metrics?.n ?? null,
    n_analysis_at: state.stockSummary?.n_analysis_at ?? null,
    n_analysis_summary: state.stockSummary?.n_analysis_summary ?? null,
    market: state.stockSummary?.market ?? null
  };
}

function financialSummary(state: SymbolAnalysisState) {
  const secondLayerFinancial = state.secondLayerData?.financial_quality;
  const quarterly = secondLayerFinancial && typeof secondLayerFinancial === 'object' && 'quarterly' in secondLayerFinancial
    ? secondLayerFinancial.quarterly
    : null;

  return {
    recent_eps_quarters: state.epsData?.quarters.slice(0, 4) ?? [],
    recent_revenue_years: state.revenueData?.years.slice(0, 4) ?? [],
    recent_gross_margin_quarters: state.grossMarginData?.quarters.slice(0, 4) ?? [],
    revenue_periodicity_available: state.revenueData ? 'annual' : null,
    quarterly_revenue_available: Array.isArray(quarterly) && quarterly.some((row) => Boolean(row?.revenue)),
    operating_margin_available: Array.isArray(quarterly) && quarterly.some((row) => Boolean(row?.operating_margin)),
    net_margin_available: Array.isArray(quarterly) && quarterly.some((row) => Boolean(row?.net_margin)),
    operating_cash_flow_available: Array.isArray(quarterly) && quarterly.some((row) => Boolean(row?.operating_cash_flow)),
    receivables_inventory_share_count_available: Array.isArray(quarterly) && quarterly.some((row) => Boolean(row?.receivables) || Boolean(row?.inventory) || Boolean(row?.shares_outstanding)),
    second_layer_quarterly_financials: Array.isArray(quarterly) ? quarterly.slice(0, 6) : []
  };
}

function dataQuality(state: SymbolAnalysisState) {
  return Object.fromEntries(
    Object.entries(state.chartStatus).map(([key, status]) => [key, status])
  );
}

export function buildAIAnalysisContext(state: SymbolAnalysisState): AIAnalysisContext {
  const klineCandles = recentCandles(state.klineData);

  return {
    stock_summary: state.stockSummary,
    company_profile: compactProfile(state.companyProfile),
    second_layer_data: compactSecondLayerData(state.secondLayerData),
    kline: state.klineData
      ? {
          symbol: state.klineData.symbol,
          period: state.klineData.period,
          source: state.klineData.source,
          data_as_of: state.klineData.data_as_of,
          recent_candles: klineCandles
        }
      : null,
    rs: compactRSData(state.rsData),
    eps: compactEPSData(state.epsData),
    revenue: compactRevenueData(state.revenueData),
    gross_margin: compactGrossMarginData(state.grossMarginData),
    chart_status: state.chartStatus,
    derived: {
      data_quality: dataQuality(state),
      first_layer: firstLayerSummary(state),
      financial: financialSummary(state),
      technical: technicalSummary(state.klineData, state.rsData)
    }
  };
}
