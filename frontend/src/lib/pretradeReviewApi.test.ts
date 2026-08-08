import { afterEach, describe, expect, it, vi } from 'vitest';
import { API_BASE_URL } from './api';
import { acknowledgePreTradeFinding, createPreTradeReview, fetchLatestPreTradeReview } from './pretradeReviewApi';

describe('pre-trade review API contract', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('creates an AI-assisted review for a saved plan', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: {}, error: null }) });
    vi.stubGlobal('fetch', fetchMock);
    const input = { run_ai: true, llm_provider: 'openai', model: 'gpt-test', backend_url: '', current_evidence: {} };

    await createPreTradeReview('plan/1', input);

    expect(fetchMock).toHaveBeenCalledWith(API_BASE_URL + '/trade-plans/plan%2F1/risk-reviews', expect.objectContaining({ method: 'POST', body: JSON.stringify(input) }));
  });

  it('loads latest and records a high-risk acknowledgement', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: {}, error: null }) });
    vi.stubGlobal('fetch', fetchMock);

    await fetchLatestPreTradeReview('plan-1');
    await acknowledgePreTradeFinding('plan-1', 'review-2', { finding_code: 'event_risk', reason: '已了解跳空风险' });

    expect(fetchMock).toHaveBeenNthCalledWith(1, API_BASE_URL + '/trade-plans/plan-1/risk-reviews/latest', expect.any(Object));
    expect(fetchMock).toHaveBeenNthCalledWith(2, API_BASE_URL + '/trade-plans/plan-1/risk-reviews/review-2/acknowledgements', expect.objectContaining({ method: 'POST' }));
  });
});
