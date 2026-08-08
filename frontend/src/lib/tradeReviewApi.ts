import type { TradePlan } from '../types/tradePlan';
import type { BehaviorObservation, BehaviorObservationFeedback, TradeReview, UpdateTradeReviewInput } from '../types/tradeReview';
import { request } from './api';

function reviewPath(journalId: string) {
  return '/journal/' + encodeURIComponent(journalId) + '/review';
}

export function createTradeReview(journalId: string) {
  return request<TradeReview>(reviewPath(journalId), { method: 'POST' });
}

export function fetchTradeReview(journalId: string) {
  return request<TradeReview>(reviewPath(journalId));
}

export function updateTradeReview(journalId: string, input: UpdateTradeReviewInput) {
  return request<TradeReview>(reviewPath(journalId), { method: 'PATCH', body: JSON.stringify(input) });
}

export function fetchJournalPlanCandidates(journalId: string) {
  return request<TradePlan[]>('/journal/' + encodeURIComponent(journalId) + '/plan-candidates');
}

export function linkJournalToPlan(journalId: string, tradePlanId: string) {
  return request<TradeReview>('/journal/' + encodeURIComponent(journalId) + '/link-plan', {
    method: 'POST',
    body: JSON.stringify({ trade_plan_id: tradePlanId })
  });
}

export function fetchBehaviorObservations() {
  return request<BehaviorObservation[]>('/behavior-observations');
}

export function updateBehaviorObservationFeedback(id: string, feedback: BehaviorObservationFeedback) {
  return request<BehaviorObservation>('/behavior-observations/' + encodeURIComponent(id) + '/feedback', {
    method: 'POST',
    body: JSON.stringify({ feedback })
  });
}
