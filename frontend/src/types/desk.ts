import type { TradePlan } from './tradePlan';
import type { EnrichedTrade, TradesSummary } from './trades';

export type MarketRegimeStatus =
  | 'confirmed_uptrend'
  | 'uptrend_under_pressure'
  | 'rally_attempt'
  | 'correction'
  | 'unavailable'
  | 'insufficient_data';

export interface IndexRegime {
  symbol: string;
  name: string;
  status: string;
  regime: MarketRegimeStatus;
  regime_label: string;
  max_exposure_percent: number;
  close: number | null;
  ema_21: number | null;
  sma_50: number | null;
  sma_200: number | null;
  atr_14_percent: number | null;
  distribution_days: number;
  above_21ema: boolean | null;
  above_50sma: boolean | null;
  above_200sma: boolean | null;
  data_as_of: string;
  source: string;
  error?: string;
}

export interface MarketRegimeReport {
  market: 'US' | 'CN';
  status: string;
  regime: MarketRegimeStatus;
  regime_label: string;
  max_exposure_percent: number;
  distribution_days: number;
  guidance: string;
  data_as_of: string;
  indices: IndexRegime[];
}

export interface DeskPayload {
  market: 'US' | 'CN';
  timezone: string;
  local_date: string;
  session: 'premarket' | 'intraday' | 'postmarket' | 'non_trading';
  market_regime: MarketRegimeReport;
  summary: TradesSummary;
  open_trades: EnrichedTrade[];
  waiting_plans: TradePlan[];
  pending_reviews: EnrichedTrade[];
}
