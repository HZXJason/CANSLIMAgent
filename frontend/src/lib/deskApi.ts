import { request } from './api';
import type { DeskPayload, MarketRegimeReport } from '../types/desk';

export function fetchDesk(market: 'US' | 'CN') {
  return request<DeskPayload>('/desk?market=' + encodeURIComponent(market));
}

export function fetchMarketRegime(market: 'US' | 'CN') {
  return request<MarketRegimeReport>('/market-regime?market=' + encodeURIComponent(market));
}
