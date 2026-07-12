export type BrokerID = 'longport' | 'tiger' | 'futu' | 'ibkr';

export type BrokerConnectionMethod = 'oauth' | 'local_gateway' | '';

export type BrokerConnectionStatus =
  | 'unconnected'
  | 'pending_authorization'
  | 'connected'
  | 'reconnect_required'
  | 'gateway_unavailable';

export interface BrokerConnection {
  id: string;
  broker: BrokerID;
  label: string;
  account_name: string;
  connection_method: BrokerConnectionMethod;
  connection_status: BrokerConnectionStatus;
  message: string;
  connected: boolean;
  secure_storage_ready: boolean;
  last_synced_at: string | null;
}

export type BrokerConnectionStart =
  | { action: 'redirect'; authorization_url: string }
  | BrokerConnection;

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
