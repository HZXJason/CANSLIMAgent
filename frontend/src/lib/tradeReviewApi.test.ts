import { afterEach, describe, expect, it, vi } from 'vitest';
import { API_BASE_URL } from './api';
import { createTradeReview, fetchBehaviorObservations, fetchTradeReview, linkJournalToPlan, updateBehaviorObservationFeedback, updateTradeReview } from './tradeReviewApi';

describe('trade review API contract', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('creates, loads, and updates one journal review', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: {}, error: null }) });
    vi.stubGlobal('fetch', fetchMock);
    await createTradeReview('journal/1');
    await fetchTradeReview('journal/1');
    await updateTradeReview('journal/1', { process_quality: 'followed', complete: true });
    expect(fetchMock).toHaveBeenNthCalledWith(1, API_BASE_URL + '/journal/journal%2F1/review', expect.objectContaining({ method: 'POST' }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, API_BASE_URL + '/journal/journal%2F1/review', expect.any(Object));
    expect(fetchMock).toHaveBeenNthCalledWith(3, API_BASE_URL + '/journal/journal%2F1/review', expect.objectContaining({ method: 'PATCH' }));
  });

  it('links only after an explicit plan choice and records behavior feedback', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: {}, error: null }) });
    vi.stubGlobal('fetch', fetchMock);
    await linkJournalToPlan('journal-1', 'plan-1');
    await fetchBehaviorObservations();
    await updateBehaviorObservationFeedback('observation-1', 'not_applicable');
    expect(fetchMock).toHaveBeenNthCalledWith(1, API_BASE_URL + '/journal/journal-1/link-plan', expect.objectContaining({ method: 'POST', body: JSON.stringify({ trade_plan_id: 'plan-1' }) }));
    expect(fetchMock).toHaveBeenNthCalledWith(3, API_BASE_URL + '/behavior-observations/observation-1/feedback', expect.objectContaining({ method: 'POST' }));
  });
});
