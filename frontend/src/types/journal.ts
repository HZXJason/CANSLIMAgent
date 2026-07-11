export type TradeSide = 'buy' | 'sell';
export type TradeMarket = 'us' | 'hk' | 'china_a' | 'crypto' | 'other';

export interface JournalEntry {
  id: string;
  datetime: string;
  symbol: string;
  name: string;
  side: TradeSide;
  quantity: number;
  price: number;
  amount: number;
  fee: number;
  market: TradeMarket;
  currency: string;
  order_id: string | null;
  source: string;
  notes: string | null;
  reflection: string | null;
  created_at: string;
  updated_at: string;
}

export interface NewJournalEntryInput {
  datetime: string;
  symbol: string;
  name?: string;
  side: TradeSide;
  quantity: number;
  price: number;
  amount?: number;
  fee?: number;
  market: TradeMarket;
  currency?: string;
  order_id?: string | null;
  notes?: string | null;
}

export interface JournalReflectionInput {
  notes?: string | null;
  reflection?: string | null;
}

export interface JournalBehaviorDiagnostic {
  severity?: 'low' | 'medium' | 'high' | string;
  evidence?: string;
  [key: string]: unknown;
}

export interface JournalReflectionPayload {
  engine?: string;
  schema_version?: string;
  record_count?: number;
  validation_warnings?: Array<string | { code?: string; message?: string; [key: string]: unknown }>;
  profile?: {
    total_trades?: number;
    total_roundtrips?: number;
    avg_holding_days?: number;
    trade_frequency_per_week?: number;
    win_rate?: number;
    profit_loss_ratio?: number;
    total_pnl?: number;
    max_drawdown?: number;
    [key: string]: unknown;
  };
  behavior?: Record<string, JournalBehaviorDiagnostic>;
  insights?: string[];
  reflection_text?: string;
  shadow_strategy_ready?: boolean;
  [key: string]: unknown;
}

export interface StoredJournalReflection {
  id: string;
  created_at: string;
  scope: string;
  record_count: number;
  engine: string;
  payload: JournalReflectionPayload;
}
