import type { JournalEntry } from './journal';
import type { TradePlan } from './tradePlan';
import type { TradeReview } from './tradeReview';

export type TradeQuadrant = 'open' | 'model_win' | 'good_loss' | 'unforced_error' | 'toxic_win';

export interface Trade {
  id: string;
  user_id: string;
  created_at: string;
  updated_at: string;
  trade_plan_id: string | null;
  playbook_id: string | null;
  symbol: string;
  name: string;
  market: string;
  currency: string;
  direction: 'long' | 'short';
  status: 'open' | 'closed' | 'cancelled';
  opened_at: string;
  closed_at: string | null;
  open_quantity: number;
  total_buy_quantity: number;
  total_sell_quantity: number;
  avg_entry_price: number;
  avg_exit_price: number | null;
  current_stop_loss: number | null;
  total_fees: number;
  planned_entry_price: number | null;
  planned_stop_loss: number | null;
  initial_risk_amount: number | null;
  open_risk_amount: number;
  realized_pnl: number | null;
  realized_pnl_percent: number | null;
  realized_r: number | null;
  entry_slippage_r: number | null;
  holding_days: number | null;
  quadrant: TradeQuadrant;
  auto_violations: string[];
}

export interface EnrichedTrade extends Trade {
  fills: JournalEntry[];
  trade_plan?: TradePlan | null;
  trade_review?: TradeReview | null;
}

export interface TradesSummary {
  total_trades: number;
  open_trades: number;
  closed_trades: number;
  model_wins: number;
  good_losses: number;
  unforced_errors: number;
  toxic_wins: number;
  expectancy_r: number;
  avg_win_r: number;
  avg_loss_r: number;
  win_rate: number;
  cost_of_indiscipline_r: number;
  disciplined_trades: number;
  disciplined_r_loop_rate: number;
  open_portfolio_risk: number;
  open_portfolio_risk_percent: number;
  account_equity: number;
  one_r_amount: number;
  max_portfolio_heat_percent: number;
  unlinked_fills_count: number;
  pending_review_trades: number;
}

export interface PositionSizeInput {
  symbol: string;
  direction?: 'buy' | 'sell';
  account_equity?: number;
  account_currency?: string;
  standard_risk_percent?: number;
  max_position_percent?: number;
  max_portfolio_heat_percent?: number;
  risk_multiple?: number;
  entry_price: number;
  stop_loss: number;
  take_profit?: number | null;
  atr_14?: number | null;
  market_regime?: string;
}

export interface PositionSizeWarning {
  code: string;
  severity: 'low' | 'medium' | 'high';
  message: string;
}

export interface PositionSizeResult {
  symbol: string;
  lot_size: number;
  min_lot_size: number;
  one_r_amount: number;
  target_risk_amount: number;
  per_share_risk: number;
  suggested_quantity: number;
  position_value: number;
  position_percent: number;
  actual_risk_amount: number;
  actual_risk_percent: number;
  actual_risk_r: number;
  stop_distance_percent: number;
  stop_atr_multiple: number | null;
  reward_risk_ratio: number | null;
  current_open_heat_percent: number;
  projected_heat_amount: number;
  projected_heat_percent: number;
  warnings: PositionSizeWarning[];
}
