import { describe, expect, it } from 'vitest';
import { conditionsToThresholds } from './discovery';

describe('discovery rule mapping', () => {
  it('applies only visible supported rules and does not convert unavailable fields into passes', () => {
    expect(conditionsToThresholds([
      { field: 'relative_strength', operator: 'gte', value: 85, unknown_policy: 'unknown' },
      { field: 'avg_dollar_volume', operator: 'gte', value: 20_000_000, unknown_policy: 'unknown' }
    ])).toEqual({ l_min_rs: 85 });
  });
});
