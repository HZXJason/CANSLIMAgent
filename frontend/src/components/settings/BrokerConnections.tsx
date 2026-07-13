import { Building2, Link2, Loader2, ShieldCheck, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { deleteBrokerConnection, fetchBrokerConnections, startBrokerConnection } from '../../lib/api';
import { brokerConnectionView } from '../../lib/brokerConnectionView';
import type { BrokerConnection, BrokerID } from '../../types/broker';

type BrokerDefinition = { id: BrokerID; name: string; summary: string };

const brokers: BrokerDefinition[] = [
  { id: 'longport', name: '长桥 LongPort', summary: 'OAuth 授权后只读拉取持仓和历史成交。' },
  { id: 'futu', name: '富途证券 Futu', summary: '检测本机已登录的 OpenD，只同步已验证账户。' },
  { id: 'ibkr', name: '盈透证券 IBKR', summary: '检测本机已登录的 TWS 或 IB Gateway，只读同步。' }
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
    setBusy(`connect:${definition.id}`); setError(null); setMessage(null);
    try {
      const result = await startBrokerConnection(definition.id);
      if ('action' in result && result.action === 'redirect') {
        window.location.assign(result.authorization_url);
        return;
      }
      await refresh();
      setMessage(`${definition.name} 检测完成，已更新账户状态。`);
    } catch (reason) { setError(reason instanceof Error ? reason.message : '券商连接失败'); } finally { setBusy(null); }
  };

  const disconnect = async (connection: BrokerConnection) => {
    if (!window.confirm(`确定断开 ${connection.account_name || connection.label || connection.broker} 吗？已导入的交易日志不会删除。`)) return;
    setBusy(`delete:${connection.id}`); setError(null);
    try { await deleteBrokerConnection(connection.id); await refresh(); setMessage('券商连接已断开；已导入的交易日志仍会保留。'); } catch (reason) { setError(reason instanceof Error ? reason.message : '断开失败'); } finally { setBusy(null); }
  };

  return (
    <Card className="mt-6 rounded-lg">
      <div className="flex items-start gap-3"><div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300"><Building2 className="h-5 w-5" /></div><div><h2 className="text-lg font-bold text-white">券商账户绑定</h2><p className="mt-2 text-sm leading-6 text-slate-500">连接账户后，请前往“交易日志”获取成交预览并确认导入。</p></div></div>
      <div className="mt-5 flex gap-2 rounded-lg border border-emerald-900/70 bg-emerald-950/20 p-3 text-xs leading-5 text-emerald-100"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" /><span>浏览器不会采集、存储或显示券商密码、令牌、私钥或本机网关配置；连接只会跳转到服务端提供的授权地址，或检测已登录的本机网关。</span></div>
      {!secureStorageReady ? <p className="mt-4 rounded-lg border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-200">尚未配置 <code>BROKER_CREDENTIALS_ENCRYPTION_KEY</code>，为保护账户凭据，绑定功能已锁定。</p> : null}
      {error ? <p className="mt-4 rounded-lg border border-red-800 bg-red-950/30 p-3 text-sm text-red-200">{error}</p> : null}
      {message ? <p className="mt-4 rounded-lg border border-cyan-900 bg-cyan-950/30 p-3 text-sm text-cyan-100">{message}</p> : null}

      <div className="mt-6 space-y-3">
        {brokers.map((definition) => {
          const connection = connectionByBroker.get(definition.id) ?? emptyConnection(definition.id);
          const view = brokerConnectionView(connection);
          const connecting = busy === `connect:${definition.id}`;
          const isPending = connection.connection_status === 'pending_authorization';
          const hasPersistedConnection = connection.id !== '';
          return <div key={definition.id} className="flex flex-col gap-4 rounded-lg border border-slate-800 bg-slate-950/25 p-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-bold text-slate-100">{definition.name}</p><span className={`rounded-full px-2 py-0.5 text-xs ${connection.connected ? 'bg-emerald-950/80 text-emerald-300' : 'bg-slate-800 text-slate-300'}`}>{view.statusLabel}</span></div><p className="mt-1 text-sm text-slate-300">{connection.account_name || definition.summary}</p><p className="mt-1 text-xs text-slate-500">{connection.last_synced_at ? `上次同步：${new Date(connection.last_synced_at).toLocaleString()}` : connection.message || '尚未同步交易记录'}</p></div>
            <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={!secureStorageReady || busy !== null || isPending} onClick={() => void connect(definition)}>{connecting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Link2 className="h-4 w-4" />}{connecting ? '连接中' : view.actionLabel}</Button>{hasPersistedConnection ? <Button type="button" variant="danger" disabled={busy !== null} onClick={() => void disconnect(connection)}><Trash2 className="h-4 w-4" />断开</Button> : null}</div>
          </div>;
        })}
      </div>
    </Card>
  );
}
