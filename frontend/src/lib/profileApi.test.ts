import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPrinciple, createPlaybook, exportJournalRecords, fetchProfileDashboard, patchAIMemory, saveTradingProfile, suggestPlaybooks } from './profileApi';

describe('profile API', () => {
  beforeEach(() => vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: {} }) })));

  it('uses the profile contracts without sending secrets from the page', async () => {
    await saveTradingProfile({ primary_markets: ['US'], trading_horizons: ['swing'], max_risk_per_trade_percent: 1, max_daily_loss_percent: 2, max_open_positions: 4 });
    await createPrinciple({ category: 'no_trade', content: '财报前不追价' });
    await createPlaybook({ name: '放量突破', markets: ['US'], horizons: ['swing'] });
    await fetchProfileDashboard('90d');
    await patchAIMemory({ principles_enabled: false });
    await suggestPlaybooks({ llm_provider: 'openai', model: 'gpt-test', backend_url: '' });
    await exportJournalRecords();
    const paths = vi.mocked(fetch).mock.calls.map(([url]) => new URL(String(url), 'http://localhost').pathname + new URL(String(url), 'http://localhost').search);
    expect(paths).toEqual(expect.arrayContaining(['/api/v1/profile', '/api/v1/profile/principles', '/api/v1/profile/playbooks', '/api/v1/profile/dashboard?range=90d', '/api/v1/profile/ai-memory', '/api/v1/profile/playbook-suggestions', '/api/v1/journal/export']));
  });
});
