import type {
  AcknowledgePreTradeFindingInput,
  CreatePreTradeReviewInput,
  PreTradeRiskReview,
  RiskAcknowledgement
} from '../types/pretradeReview';
import { request } from './api';

function reviewRoot(planId: string) {
  return '/trade-plans/' + encodeURIComponent(planId) + '/risk-reviews';
}

export function createPreTradeReview(planId: string, input: CreatePreTradeReviewInput) {
  return request<PreTradeRiskReview>(reviewRoot(planId), {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function fetchPreTradeReviews(planId: string) {
  return request<PreTradeRiskReview[]>(reviewRoot(planId));
}

export function fetchLatestPreTradeReview(planId: string) {
  return request<PreTradeRiskReview>(reviewRoot(planId) + '/latest');
}

export function acknowledgePreTradeFinding(planId: string, reviewId: string, input: AcknowledgePreTradeFindingInput) {
  return request<RiskAcknowledgement>(reviewRoot(planId) + '/' + encodeURIComponent(reviewId) + '/acknowledgements', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}
