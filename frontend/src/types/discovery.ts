import type { AssetMarket, Market, ThresholdConfig } from '.';

export type ScreenField = 'quarterly_eps_growth' | 'annual_eps_growth' | 'volume_ratio' | 'relative_strength' | 'institution_delta' | 'avg_dollar_volume' | 'volatility';
export interface ScreenCondition { field: ScreenField; operator: 'gte' | 'lte'; value: number; unknown_policy: 'unknown' }
export interface ScreenDefinition {
  id: string; created_at: string; updated_at: string; name: string; description: string;
  asset_market: AssetMarket; stock_pool: Market; conditions: ScreenCondition[]; is_system: boolean;
}
export interface NewScreenDefinition { name: string; description: string; asset_market: AssetMarket; stock_pool: Market; conditions: ScreenCondition[] }

export function conditionsToThresholds(conditions: ScreenCondition[]): Partial<ThresholdConfig> {
  const result: Partial<ThresholdConfig> = {};
  for (const condition of conditions) {
    if (condition.operator !== 'gte') continue;
    if (condition.field === 'quarterly_eps_growth') result.c_min_growth = condition.value;
    if (condition.field === 'annual_eps_growth') result.a_min_growth = condition.value;
    if (condition.field === 'volume_ratio') result.s_volume_ratio = condition.value;
    if (condition.field === 'relative_strength') result.l_min_rs = condition.value;
    if (condition.field === 'institution_delta') result.i_min_delta = condition.value;
  }
  return result;
}
