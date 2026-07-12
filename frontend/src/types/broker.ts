export type BrokerID = 'longport' | 'tiger' | 'futu' | 'ibkr';

export interface BrokerConnection {
  id: string;
  broker: BrokerID;
  label: string;
  connected: boolean;
  secure_storage_ready: boolean;
  last_synced_at: string | null;
}

export interface BrokerExecution {
  external_id: string;
  symbol: string;
  name: string;
  side: 'buy' | 'sell' | string;
  quantity: number;
  price: number;
  fee: number;
  currency: string;
  market: string;
  executed_at: string;
}

export interface BrokerPosition {
  symbol: string;
  name: string;
  quantity: number;
  market: string;
  currency: string;
}

export interface BrokerSyncResult {
  positions: BrokerPosition[];
  executions: BrokerExecution[];
  warnings: string[];
}
