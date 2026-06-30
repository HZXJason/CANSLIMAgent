export type TradeDirection = 'buy' | 'sell';

export interface JournalEntry {
  id: string;
  ticker: string;
  direction: TradeDirection;
  trade_date: string;
  price: number;
  position_size: number;
  stop_loss: number;
  notes: string | null;
  exit_price: number | null;
  pnl_pct: number | null;
  reflection: string | null;
  phase1_locked: boolean;
  created_at: string;
  updated_at: string;
}

export interface NewJournalEntryInput {
  ticker: string;
  direction: TradeDirection;
  trade_date: string;
  price: number;
  position_size: number;
  stop_loss: number;
  notes?: string | null;
}

export interface JournalReflectionInput {
  exit_price: number | null;
  pnl_pct: number | null;
  reflection: string | null;
}
