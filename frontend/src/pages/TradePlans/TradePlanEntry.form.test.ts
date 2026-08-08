import { describe, expect, it } from 'vitest';
import { buildPlanInput } from './TradePlanEntry';

describe('trade plan risk fields', () => {
  it('includes invalidation and explicit risk budgets in the saved plan', () => {
    const result = buildPlanInput({
      ticker: 'NVDA', direction: 'buy', status: 'draft', plan_date: '2026-08-04', valid_until: '2026-08-08',
      entry_trigger: '放量突破', entry_price: '120', stop_loss: '115', take_profit: '132', position_size: '10',
      invalidation: '跌破 115 且无法收回', max_risk_amount: '800', max_risk_percent: '0.8',
      rationale: '基本面与技术面共振', ai_summary: '', notes: ''
    }, false, 'US', 'thesis-1');

    expect(result.errors).toEqual({});
    expect(result.input).toEqual(expect.objectContaining({
      user_thesis_id: 'thesis-1',
      invalidation: '跌破 115 且无法收回',
      max_risk_amount: 800,
      max_risk_percent: 0.8
    }));
  });
});
