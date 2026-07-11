import type { JournalEntry, NewJournalEntryInput, TradeSide } from './journal';

export type TradePlanDirection = TradeSide | 'watch';
export type TradePlanStatus = 'draft' | 'waiting' | 'executed' | 'cancelled' | 'expired';

export interface TradePlan {
  id: string;
  ticker: string;
  direction: TradePlanDirection;
  status: TradePlanStatus;
  plan_date: string;
  valid_until: string | null;
  entry_trigger: string | null;
  entry_price: number | null;
  stop_loss: number | null;
  take_profit: number | null;
  position_size: number | null;
  rationale: string | null;
  ai_summary: string | null;
  notes: string | null;
  executed_journal_id: string | null;
  executed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface TradePlanFilters {
  ticker?: string;
  status?: TradePlanStatus;
}

export interface NewTradePlanInput {
  ticker: string;
  direction: TradePlanDirection;
  status?: Exclude<TradePlanStatus, 'executed'>;
  plan_date: string;
  valid_until?: string | null;
  entry_trigger?: string | null;
  entry_price?: number | null;
  stop_loss?: number | null;
  take_profit?: number | null;
  position_size?: number | null;
  rationale?: string | null;
  ai_summary?: string | null;
  notes?: string | null;
}

export type TradePlanUpdateInput = Partial<NewTradePlanInput> & {
  status?: TradePlanStatus;
};

export type ExecuteTradePlanInput = Pick<NewJournalEntryInput, 'datetime' | 'quantity' | 'price' | 'market'> &
  Partial<Pick<NewJournalEntryInput, 'name' | 'fee' | 'currency' | 'order_id' | 'notes'>>;

export interface ExecuteTradePlanResponse {
  plan: TradePlan;
  journal: JournalEntry;
}
