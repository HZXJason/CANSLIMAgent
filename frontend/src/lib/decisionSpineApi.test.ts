import { afterEach, describe, expect, it, vi } from 'vitest';

import { API_BASE_URL } from './api';
import { createCandidateSnapshot, reviseUserThesis } from './decisionSpineApi';

describe('decision spine API contract', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('creates candidate snapshots through the shared API envelope', async () => {
    const input = {
      symbol: 'NVDA',
      market: 'NASDAQ100' as const,
      screen_name: 'CANSLIM',
      screen_definition_id: null,
      inclusion_reasons: ['RS 达标'],
      exclusion_risks: ['财报临近'],
      context_snapshot: { data_as_of: '2026-08-04' },
      data_as_of: '2026-08-04T00:00:00Z'
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        success: true,
        data: { id: 'candidate-1', created_at: input.data_as_of, ...input },
        error: null
      })
    });
    vi.stubGlobal('fetch', fetchMock);

    await createCandidateSnapshot(input);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith(API_BASE_URL + '/decision-spine/candidates', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(input)
    }));
  });

  it('revises an immutable thesis through its dedicated route', async () => {
    const input = {
      stance: 'watch' as const,
      summary: '等待量价确认',
      accepted_arguments: [],
      rejected_arguments: [],
      unresolved_questions: ['成交量能否持续'],
      falsifiers: ['跌破关键支撑']
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ success: true, data: {}, error: null })
    });
    vi.stubGlobal('fetch', fetchMock);

    await reviseUserThesis('thesis/1', input);

    expect(fetchMock).toHaveBeenCalledWith(API_BASE_URL + '/decision-spine/theses/thesis%2F1/revise', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(input)
    }));
  });
});
