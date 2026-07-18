import { Building2, Link2, Loader2, ShieldCheck, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { ConfirmDialog } from '../ui/confirm-dialog';
import { deleteBrokerConnection, fetchBrokerConnections, startBrokerConnection } from '../../lib/api';
import { brokerConnectionView } from '../../lib/brokerConnectionView';
import type { BrokerConnection, BrokerID } from '../../types/broker';

type BrokerDefinition = { id: BrokerID; name: string };

const brokers: BrokerDefinition[] = [
  { id: 'longport', name: '长桥 LongPort' },
  { id: 'futu', name: '富途证券 Futu' },
  { id: 'ibkr', name: '盈透证券 IBKR' }
];

const emptyConnection = (broker: BrokerID): BrokerConnection => ({
  id: '',
  broker,
  label: '',
  account_name: '',
  connection_method: '',
  connection_status: 'unconnected',
  message: '',
  connected: false,
  secure_storage_ready: true,
  last_synced_at: null
});

export default function BrokerConnections() {
  const [connections, setConnections] = useState<BrokerConnection[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [disconnectTarget, setDisconnectTarget] = useState<BrokerConnection | null>(null);

  const refresh = async () => {
    const values = await fetchBrokerConnections();
    setConnections(values);
  };

  useEffect(() => {
    void refresh().catch((reason) => setError(reason instanceof Error ? reason.message : '券商配置读取失败'));
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const broker = params.get('broker');
    const connectionError = params.get('connection_error');
    const definition = brokers.find((item) => item.id === broker);
    if (!definition) return;
    if (connectionError) setError(`${definition.name.replace('长桥 ', '')} 连接失败：${connectionError}`);
    void refresh().catch((reason) => setError(reason instanceof Error ? reason.message : '券商配置读取失败'));
  }, []);

  const connectionByBroker = useMemo(
    () => new Map(connections.map((connection) => [connection.broker, connection])),
    [connections]
  );
  const secureStorageReady = connections.length === 0 || connections.every((connection) => connection.secure_storage_ready);

  const connect = async (definition: BrokerDefinition) => {
    setBusy(`connect:${definition.id}`);
    setError(null);
    setMessage(null);
    try {
      const result = await startBrokerConnection(definition.id);
      if ('action' in result && result.action === 'redirect') {
        window.location.assign(result.authorization_url);
        return;
      }
      await refresh();
      setMessage(`${definition.name} 账户状态已更新`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '券商连接失败');
    } finally {
      setBusy(null);
    }
  };

  const disconnect = async (connection: BrokerConnection) => {
    setBusy(`delete:${connection.id}`);
    setError(null);
    setMessage(null);
    try {
      await deleteBrokerConnection(connection.id);
      await refresh();
      setMessage('连接已断开，已导入的交易日志仍会保留');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '断开失败');
    } finally {
      setBusy(null);
    }
  };

  return (
    <Card className="mt-5 overflow-hidden p-0">
      <div className="flex flex-col gap-3 border-b border-slate-800 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-cyan-400/10 text-cyan-300">
            <Building2 className="h-5 w-5" />
          </div>
          <h2 className="text-lg font-bold text-white">券商账户</h2>
        </div>
        <span className="inline-flex self-start items-center gap-2 rounded-full bg-emerald-400/10 px-3 py-1.5 text-xs font-semibold text-emerald-200 sm:self-auto">
          <ShieldCheck className="h-4 w-4" />
          只读连接
        </span>
      </div>

      {!secureStorageReady || error || message ? (
        <div className="space-y-3 px-5 pt-4 sm:px-6">
          {!secureStorageReady ? <p className="rounded-xl border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-200">安全存储未配置，券商绑定已锁定。</p> : null}
          {error ? <p className="rounded-xl border border-red-800 bg-red-950/30 p-3 text-sm text-red-200">{error}</p> : null}
          {message ? <p className="rounded-xl border border-cyan-900 bg-cyan-950/30 p-3 text-sm text-cyan-100">{message}</p> : null}
        </div>
      ) : null}

      <div className="mt-4 divide-y divide-slate-800 border-t border-slate-800">
        {brokers.map((definition) => {
          const connection = connectionByBroker.get(definition.id) ?? emptyConnection(definition.id);
          const view = brokerConnectionView(connection);
          const connecting = busy === `connect:${definition.id}`;
          const isPending = connection.connection_status === 'pending_authorization';
          const hasPersistedConnection = connection.id !== '';
          return (
            <div key={definition.id} className="flex flex-col gap-4 px-5 py-4 sm:px-6 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="font-bold text-slate-100">{definition.name}</p>
                  <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${connection.connected ? 'bg-emerald-400/10 text-emerald-300' : connection.connection_status === 'reconnect_required' || connection.connection_status === 'gateway_unavailable' ? 'bg-amber-400/10 text-amber-200' : 'bg-slate-800 text-slate-300'}`}>{view.statusLabel}</span>
                </div>
                {connection.account_name ? <p className="mt-2 text-sm font-medium text-slate-300">{connection.account_name}</p> : null}
                {connection.message ? <p className="mt-2 text-sm text-amber-200">{connection.message}</p> : null}
                {connection.last_synced_at ? <p className="mt-2 text-sm text-slate-400">上次同步 {new Date(connection.last_synced_at).toLocaleString()}</p> : null}
              </div>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                {hasPersistedConnection ? (
                  <Button type="button" variant="danger" disabled={busy !== null} onClick={() => setDisconnectTarget(connection)}>
                    <Trash2 className="h-4 w-4" />
                    断开
                  </Button>
                ) : null}
                <Button type="button" variant={connection.connected ? 'outline' : 'primary'} disabled={!secureStorageReady || busy !== null || isPending} onClick={() => void connect(definition)}>
                  {connecting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Link2 className="h-4 w-4" />}
                  {connecting ? '连接中' : view.actionLabel}
                </Button>
              </div>
            </div>
          );
        })}
      </div>
      <ConfirmDialog
        open={disconnectTarget !== null}
        title="断开券商账户"
        description={`断开 ${disconnectTarget?.account_name || disconnectTarget?.label || disconnectTarget?.broker || '此账户'} 后，已导入的交易日志仍会保留。`}
        confirmText="确认断开"
        onConfirm={() => {
          if (disconnectTarget) void disconnect(disconnectTarget);
        }}
        onOpenChange={(open) => {
          if (!open) setDisconnectTarget(null);
        }}
      />
    </Card>
  );
}
