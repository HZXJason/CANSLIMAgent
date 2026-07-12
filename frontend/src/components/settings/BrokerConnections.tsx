import { Building2, Eye, Link2, Loader2, ShieldCheck, Trash2, Upload } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { deleteBrokerConnection, fetchBrokerConnections, importBrokerExecutions, startBrokerConnection, syncBrokerConnection } from '../../lib/api';
import { brokerConnectionView } from '../../lib/brokerConnectionView';
import type { BrokerConnection, BrokerExecution, BrokerID, BrokerSyncResult } from '../../types/broker';

type BrokerDefinition = { id: BrokerID; name: string; summary: string };

const brokers: BrokerDefinition[] = [
  { id: 'longport', name: '长桥 LongPort', summary: 'OAuth 授权后只读拉取持仓和历史成交。' },
  { id: 'tiger', name: '老虎证券 Tiger', summary: 'OAuth 授权后只读拉取持仓和已成交订单。' },
  { id: 'futu', name: '富途证券 Futu', summary: '检测本机已登录的 OpenD，只同步已验证账户。' },
  { id: 'ibkr', name: '盈透证券 IBKR', summary: '检测本机已登录的 TWS 或 IB Gateway，只读同步。' }
];

const emptyConnection = (broker: BrokerID): BrokerConnection => ({
  id: broker,
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
  const [preview, setPreview] = useState<{ connectionID: string; result: BrokerSyncResult } | null>(null);
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

  const sync = async (connection: BrokerConnection) => {
    setBusy(`sync:${connection.id}`); setError(null); setMessage(null);
    try {
      const result = await syncBrokerConnection(connection.id);
      setPreview({ connectionID: connection.id, result });
      setMessage(`已读取 ${result.positions.length} 项持仓与 ${result.executions.length} 笔成交；请确认后导入。`);
    } catch (reason) { setError(reason instanceof Error ? reason.message : '券商同步失败'); } finally { setBusy(null); }
  };

  const importPreview = async () => {
    if (!preview) return;
    setBusy(`import:${preview.connectionID}`); setError(null);
    try {
      const result = await importBrokerExecutions(preview.connectionID, preview.result.executions);
      setPreview(null); await refresh();
      setMessage(`已导入 ${result.created} 笔成交；跳过 ${result.skipped} 笔已存在记录。`);
    } catch (reason) { setError(reason instanceof Error ? reason.message : '成交导入失败'); } finally { setBusy(null); }
  };

  const disconnect = async (connection: BrokerConnection) => {
    if (!window.confirm(`确定断开 ${connection.account_name || connection.label || connection.broker} 吗？已导入的交易日志不会删除。`)) return;
    setBusy(`delete:${connection.id}`); setError(null);
    try { await deleteBrokerConnection(connection.id); setPreview(null); await refresh(); setMessage('券商连接已断开；已导入的交易日志仍会保留。'); } catch (reason) { setError(reason instanceof Error ? reason.message : '断开失败'); } finally { setBusy(null); }
  };

  return (
    <Card className="mt-6 rounded-lg">
      <div className="flex items-start gap-3"><div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300"><Building2 className="h-5 w-5" /></div><div><h2 className="text-lg font-bold text-white">券商绑定与交易同步</h2><p className="mt-2 text-sm leading-6 text-slate-500">只读同步持仓与成交；同步结果必须确认后才会写入交易日志并用于 Vibe 复盘。</p></div></div>
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
          return <div key={definition.id} className="flex flex-col gap-4 rounded-lg border border-slate-800 bg-slate-950/25 p-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-bold text-slate-100">{definition.name}</p><span className={`rounded-full px-2 py-0.5 text-xs ${connection.connected ? 'bg-emerald-950/80 text-emerald-300' : 'bg-slate-800 text-slate-300'}`}>{view.statusLabel}</span></div><p className="mt-1 text-sm text-slate-300">{connection.account_name || definition.summary}</p><p className="mt-1 text-xs text-slate-500">{connection.last_synced_at ? `上次同步：${new Date(connection.last_synced_at).toLocaleString()}` : connection.message || '尚未同步交易记录'}</p></div>
            <div className="flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={!secureStorageReady || busy !== null || isPending} onClick={() => void connect(definition)}>{connecting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Link2 className="h-4 w-4" />}{connecting ? '连接中' : view.actionLabel}</Button>{view.canSync ? <Button type="button" variant="outline" disabled={busy !== null} onClick={() => void sync(connection)}>{busy === `sync:${connection.id}` ? <Loader2 className="h-4 w-4 animate-spin" /> : <Eye className="h-4 w-4" />}同步预览</Button> : null}{connection.connected ? <Button type="button" variant="danger" disabled={busy !== null} onClick={() => void disconnect(connection)}><Trash2 className="h-4 w-4" />断开</Button> : null}</div>
          </div>;
        })}
      </div>

      {preview ? <div className="mt-6 rounded-lg border border-cyan-900/70 bg-cyan-950/20 p-4"><div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h3 className="font-bold text-cyan-100">同步预览</h3><p className="mt-1 text-sm text-slate-400">{preview.result.positions.length} 项持仓，{preview.result.executions.length} 笔可导入成交</p></div><Button type="button" disabled={busy !== null || preview.result.executions.length === 0} onClick={() => void importPreview()}>{busy === `import:${preview.connectionID}` ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}{busy === `import:${preview.connectionID}` ? '导入中' : '确认导入成交'}</Button></div>{preview.result.warnings.length > 0 ? <ul className="mt-3 list-disc space-y-1 pl-5 text-xs text-amber-200">{preview.result.warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul> : null}<div className="mt-4 overflow-x-auto"><table className="w-full min-w-[620px] text-left text-xs"><thead className="text-slate-500"><tr><th className="pb-2">时间</th><th className="pb-2">标的</th><th className="pb-2">方向</th><th className="pb-2">数量</th><th className="pb-2">价格</th><th className="pb-2">费用</th></tr></thead><tbody>{preview.result.executions.slice(0, 10).map((item: BrokerExecution) => <tr className="border-t border-slate-800 text-slate-200" key={item.external_id}><td className="py-2">{new Date(item.executed_at).toLocaleString()}</td><td>{item.symbol}</td><td>{item.side === 'buy' ? '买入' : '卖出'}</td><td>{item.quantity}</td><td>{item.price}</td><td>{item.fee}</td></tr>)}</tbody></table></div>{preview.result.executions.length > 10 ? <p className="mt-2 text-xs text-slate-500">仅展示前 10 笔，确认后将导入全部预览成交。</p> : null}</div> : null}
    </Card>
  );
}
