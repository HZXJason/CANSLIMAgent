export type ProfileRange = '7d' | '30d' | '90d' | 'all';
export type PrincipleCategory = 'no_trade' | 'pretrade_check' | 'risk_exit';
export type PlaybookStatus = 'draft' | 'active' | 'archived';

export interface TradingProfile {
  configured: boolean;
  user_id?: string;
  primary_markets: string[];
  asset_classes?: string[];
  trading_horizons: string[];
  strategy_preferences?: string[];
  investment_objectives?: string[];
  risk_tolerance?: 'low' | 'medium' | 'high' | null;
  max_risk_per_trade_percent: number | null;
  max_daily_loss_percent: number | null;
  max_open_positions: number | null;
  max_portfolio_concentration_percent?: number | null;
  leverage_allowed?: boolean | null;
  account_equity?: number | null;
  account_currency?: string;
  standard_risk_percent?: number | null;
  max_portfolio_heat_percent?: number | null;
  created_at: string;
  updated_at: string;
}

export interface TradingPrinciple {
  id: string; category: PrincipleCategory; content: string; enabled: boolean; ai_enabled: boolean; sort_order: number; created_at: string; updated_at: string;
}

export interface TradingPlaybook {
  id: string; name: string; setup_type?: string; markets: string[]; horizons: string[]; market_context: string | null; entry_rules: string | null; invalidation_rules: string | null; stop_rules: string | null; target_rules: string | null; position_rules: string | null; no_trade_rules: string | null; notes: string | null; default_risk_multiple?: number | null; default_stop_atr_multiple?: number | null; status: PlaybookStatus; source: string; evidence_review_ids: string[]; evidence_count: number; ai_enabled: boolean; created_at: string; updated_at: string;
}

export interface AIMemoryPreference {
  principles_enabled: boolean; observations_enabled: boolean; playbooks_enabled: boolean; created_at: string; updated_at: string;
}

export type TradingPolicyKind = 'rule' | 'discipline';
export type TradingPolicyOrigin = 'manual' | 'ai_suggested' | 'system';
export type TradingPolicyStatus = 'draft' | 'active' | 'archived';

export interface TradingPolicy {
  id: string; user_id: string; kind: TradingPolicyKind; category: string; title: string; statement: string;
  condition: Record<string, unknown>; action: Record<string, unknown>; scope: Record<string, unknown>;
  strength: 'hard' | 'soft' | 'reminder'; origin: TradingPolicyOrigin; status: TradingPolicyStatus;
  prompt_enabled: boolean; version: number; effective_from: string | null; effective_to: string | null;
  created_at: string; updated_at: string;
}

export interface UserTradingContext {
  version: string; generated_at: string; purpose: string;
  user: { id: string; display_name: string; timezone: string; base_currency: string; status: string };
  declared_profile: TradingProfile; policies: TradingPolicy[]; principles: TradingPrinciple[];
  playbooks: TradingPlaybook[];
  activity: { trade_plans: number; trades: number; execution_fills: number; completed_reviews: number };
}

export interface ProfileDashboard {
  range: ProfileRange; from: string | null; to: string; as_of: string; sample_size: number;
  process: { executions: number; planned_executions: number; planned_execution_rate: number; pretrade_reviewed: number; pretrade_review_rate: number; completed_reviews: number; review_completion_rate: number; followed_reviews: number; plan_follow_rate: number; deviated_reviews: number; off_plan_executions: number };
  trends: Array<{ date: string; executions: number; planned_execution_rate: number; pretrade_review_rate: number; review_completion_rate: number; plan_follow_rate: number; off_plan_executions: number; deviated_reviews: number }>;
  outcomes: Array<{ market: string; currency: string; realized_pnl: number; matched_trades: number; winning_trades: number; win_rate: number | null; open_quantity: number; coverage_rate: number }>;
}

export interface PlaybookSuggestion extends Omit<TradingPlaybook, 'id' | 'created_at' | 'updated_at' | 'status' | 'source' | 'ai_enabled'> { generated_at: string }

export type TradingProfileInput = Pick<TradingProfile, 'primary_markets' | 'trading_horizons' | 'max_risk_per_trade_percent' | 'max_daily_loss_percent' | 'max_open_positions'> & Partial<Pick<TradingProfile, 'asset_classes' | 'strategy_preferences' | 'investment_objectives' | 'risk_tolerance' | 'max_portfolio_concentration_percent' | 'leverage_allowed' | 'account_equity' | 'account_currency' | 'standard_risk_percent' | 'max_portfolio_heat_percent'>>;
export type PrincipleInput = { category: PrincipleCategory; content: string; enabled?: boolean; ai_enabled?: boolean; sort_order?: number };
export type PlaybookInput = Partial<Omit<TradingPlaybook, 'id' | 'created_at' | 'updated_at'>> & { name: string; markets: string[]; horizons: string[] };
