import { request } from './api';
import type { EnrichedTrade, Trade, TradesSummary } from '../types/trades';

export function fetchTrades(params: { status?: 'open' | 'closed' | 'all'; market?: 'US' | 'CN' } = {}) {
  const qs = new URLSearchParams();
  if (params.status) qs.set('status', params.status);
  if (params.market) qs.set('market', params.market);
  const query = qs.toString();
  return request<EnrichedTrade[]>('/trades' + (query ? '?' + query : ''));
}

export function fetchTradesSummary(market?: 'US' | 'CN') {
  return request<TradesSummary>('/trades/summary' + (market ? '?market=' + encodeURIComponent(market) : ''));
}

export function fetchTrade(id: string) {
  return request<EnrichedTrade>('/trades/' + encodeURIComponent(id));
}

export function updateTradeStopLoss(id: string, stopLoss: number) {
  return request<Trade>('/trades/' + encodeURIComponent(id) + '/stop-loss', {
    method: 'PATCH',
    body: JSON.stringify({ stop_loss: stopLoss })
  });
}

export function linkTradeToPlan(tradeId: string, tradePlanId: string) {
  return request<EnrichedTrade>('/trades/' + encodeURIComponent(tradeId) + '/link-plan', {
    method: 'POST',
    body: JSON.stringify({ trade_plan_id: tradePlanId })
  });
}

export function upsertTradeReview(
  tradeId: string,
  input: {
    process_quality: 'followed' | 'adjusted' | 'deviated';
    deviation_reason?: string | null;
    emotion_before?: string | null;
    emotion_after?: string | null;
    outcome_notes?: string | null;
    lesson?: string | null;
  }
) {
  return request<{ trade: EnrichedTrade }>('/trades/' + encodeURIComponent(tradeId) + '/review', {
    method: 'PUT',
    body: JSON.stringify(input)
  });
}
