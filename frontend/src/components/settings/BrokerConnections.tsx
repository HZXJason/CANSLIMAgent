import { Building2, CheckCircle2, Eye, KeyRound, Link2, Loader2, ShieldCheck, Trash2, Upload } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { deleteBrokerConnection, fetchBrokerConnections, importBrokerExecutions, startBrokerConnection, syncBrokerConnection } from '../../lib/api';
import type { BrokerConnection, BrokerExecution, BrokerID, BrokerSyncResult } from '../../types/broker';

type Field = { key: string; label: string; placeholder: string; secret?: boolean; defaultValue?: string };
type BrokerDefinition = { id: BrokerID; name: string; summary: string; instruction: string; fields: Field[] };

const brokers: BrokerDefinition[] = [
  {
    id: 'longport', name: '长桥 LongPort', summary: 'OAuth / OpenAPI，只读拉取持仓和历史成交。',
    instruction: '填写 OAuth access token；连接后只会调用持仓与成交接口。',
    fields: [{ key: 'access_token', label: 'OAuth Access Token', placeholder: '仅保存到后端加密存储', secret: true }, { key: 'base_url', label: 'OpenAPI 地址', placeholder: 'https://openapi.longportapp.com', defaultValue: 'https://openapi.longportapp.com' }]
  },
  {
    id: 'tiger', name: '老虎证券 Tiger', summary: '通过 Tiger OpenAPI 读取持仓与已成交订单。',
    instruction: '在老虎开放平台生成开发者凭据；不会调用下单接口。',
    fields: [{ key: 'tiger_id', label: 'Tiger ID', placeholder: '开放平台 Tiger ID' }, { key: 'account', label: '证券账户', placeholder: '账户号' }, { key: 'private_key', label: 'Private Key', placeholder: '私钥内容', secret: true }]
  },
  {
    id: 'futu', name: '富途证券 Futu', summary: '连接本机 OpenD，仅同步已经登录的账户。',
    instruction: '先在本机启动并登录 OpenD；本应用只连接本地网关，不上传登录密码。',
    fields: [{ key: 'host', label: 'OpenD 主机', placeholder: '127.0.0.1', defaultValue: '127.0.0.1' }, { key: 'port', label: 'OpenD 端口', placeholder: '11111', defaultValue: '11111' }, { key: 'account', label: '账户号（可选）', placeholder: '留空则使用 OpenD 默认账户' }]
  },
  {
    id: 'ibkr', name: '盈透证券 IBKR', summary: '连接本机 TWS 或 IB Gateway 的只读 API 会话。',
    instruction: '先在 TWS/IB Gateway 登录并开启 API；请使用只读 API 设置。',
    fields: [{ key: 'host', label: 'TWS / Gateway 主机', placeholder: '127.0.0.1', defaultValue: '127.0.0.1' }, { key: 'port', label: 'API 端口', placeholder: '7497', defaultValue: '7497' }, { key: 'client_id', label: 'Client ID', placeholder: '9901', defaultValue: '9901' }]
  }
];

function defaultValues(definition: BrokerDefinition) {
  return Object.fromEntries(definition.fields.map((field) => [field.key, field.defaultValue ?? '']));
}

export default function BrokerConnections() {
  const [connections, setConnections] = useState<BrokerConnection[]>([]);
  const [selected, setSelected] = useState<BrokerID>('longport');
  const [credentials, setCredentials] = useState<Record<string, string>>(() => defaultValues(brokers[0]));
  const [label, setLabel] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<{ connectionID: string; result: BrokerSyncResult } | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const definition = useMemo(() => brokers.find((item) => item.id === selected) ?? brokers[0], [selected]);
  const secureStorageReady = connections.length > 0 ? connections[0].secure_storage_ready : true;

  const refresh = async () => {
    const values = await fetchBrokerConnections();
    setConnections(values);
  };

  useEffect(() => { void refresh().catch((reason) => setError(reason instanceof Error ? reason.message : '券商配置读取失败')); }, []);

  const chooseBroker = (id: BrokerID) => {
    const next = brokers.find((item) => item.id === id) ?? brokers[0];
    setSelected(id); setCredentials(defaultValues(next)); setLabel(''); setError(null); setMessage(null);
  };

  const save = async () => {
    setBusy('connect'); setError(null); setMessage(null);
    try {
      await startBrokerConnection(definition.id);
      await refresh();
      setCredentials(defaultValues(definition));
      setLabel('');
      setMessage(`${definition.name} 已安全保存，可先同步预览。`);
    } catch (reason) { setError(reason instanceof Error ? reason.message : '券商连接保存失败'); } finally { setBusy(null); }
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
    if (!window.confirm(`确定断开 ${connection.label || connection.broker} 吗？已导入的交易日志不会删除。`)) return;
    setBusy(`delete:${connection.id}`); setError(null);
    try { await deleteBrokerConnection(connection.id); setPreview(null); await refresh(); setMessage('券商凭据已从加密存储删除。'); } catch (reason) { setError(reason instanceof Error ? reason.message : '断开失败'); } finally { setBusy(null); }
  };

  return (
    <Card className="mt-6 rounded-lg">
      <div className="flex items-start gap-3"><div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300"><Building2 className="h-5 w-5" /></div><div><h2 className="text-lg font-bold text-white">券商绑定与交易同步</h2><p className="mt-2 text-sm leading-6 text-slate-500">只读同步持仓与成交；同步结果必须确认后才会写入交易日志并用于 Vibe 复盘。</p></div></div>
      <div className="mt-5 flex gap-2 rounded-lg border border-emerald-900/70 bg-emerald-950/20 p-3 text-xs leading-5 text-emerald-100"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" /><span>凭据不会存入浏览器或返回给前端；后端仅在配置了 AES-256-GCM 主密钥后才允许加密保存，并且适配器不提供下单、改单或撤单能力。</span></div>
      {!secureStorageReady ? <p className="mt-4 rounded-lg border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-200">尚未配置 <code>BROKER_CREDENTIALS_ENCRYPTION_KEY</code>，为保护账户凭据，绑定功能已锁定。</p> : null}
      {error ? <p className="mt-4 rounded-lg border border-red-800 bg-red-950/30 p-3 text-sm text-red-200">{error}</p> : null}
      {message ? <p className="mt-4 rounded-lg border border-cyan-900 bg-cyan-950/30 p-3 text-sm text-cyan-100">{message}</p> : null}

      <div className="mt-5 grid gap-3 md:grid-cols-2">{brokers.map((item) => <button key={item.id} type="button" onClick={() => chooseBroker(item.id)} className={`rounded-lg border p-4 text-left transition ${selected === item.id ? 'border-cyan-400/70 bg-cyan-950/25' : 'border-slate-800 bg-slate-950/25 hover:border-slate-600'}`}><p className="font-bold text-slate-100">{item.name}</p><p className="mt-1 text-xs leading-5 text-slate-500">{item.summary}</p></button>)}</div>

      <div className="mt-5 rounded-lg border border-slate-800 bg-slate-950/25 p-4"><div className="flex items-start gap-2"><KeyRound className="mt-0.5 h-4 w-4 text-cyan-300" /><div><p className="font-semibold text-slate-100">连接 {definition.name}</p><p className="mt-1 text-xs leading-5 text-slate-500">{definition.instruction}</p></div></div><div className="mt-4 grid gap-4 md:grid-cols-2"><label className="space-y-2"><Label>连接名称</Label><Input value={label} placeholder={definition.name} onChange={(event) => setLabel(event.target.value)} /></label>{definition.fields.map((field) => <label className="space-y-2" key={field.key}><Label>{field.label}</Label><Input type={field.secret ? 'password' : 'text'} autoComplete="off" value={credentials[field.key] ?? ''} placeholder={field.placeholder} onChange={(event) => setCredentials((current) => ({ ...current, [field.key]: event.target.value }))} /></label>)}</div><div className="mt-4 flex justify-end"><Button type="button" disabled={!secureStorageReady || busy !== null} onClick={() => void save()}>{busy === 'connect' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Link2 className="h-4 w-4" />}{busy === 'connect' ? '保存中' : '安全保存连接'}</Button></div></div>

      <div className="mt-6 space-y-3"><h3 className="font-bold text-slate-100">已绑定券商</h3>{connections.filter((item) => item.connected).length === 0 ? <p className="text-sm text-slate-500">尚未绑定券商。</p> : connections.filter((item) => item.connected).map((connection) => <div key={connection.id} className="flex flex-col gap-3 rounded-lg border border-slate-800 bg-slate-950/25 p-4 sm:flex-row sm:items-center sm:justify-between"><div><p className="flex items-center gap-2 font-semibold text-slate-100"><CheckCircle2 className="h-4 w-4 text-emerald-400" />{connection.label || connection.broker}</p><p className="mt-1 text-xs text-slate-500">{connection.last_synced_at ? `上次导入：${new Date(connection.last_synced_at).toLocaleString()}` : '尚未导入交易记录'}</p></div><div className="flex gap-2"><Button type="button" variant="outline" disabled={busy !== null} onClick={() => void sync(connection)}>{busy === `sync:${connection.id}` ? <Loader2 className="h-4 w-4 animate-spin" /> : <Eye className="h-4 w-4" />}同步预览</Button><Button type="button" variant="danger" disabled={busy !== null} onClick={() => void disconnect(connection)}><Trash2 className="h-4 w-4" />断开</Button></div></div>)}</div>

      {preview ? <div className="mt-6 rounded-lg border border-cyan-900/70 bg-cyan-950/20 p-4"><div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"><div><h3 className="font-bold text-cyan-100">同步预览</h3><p className="mt-1 text-sm text-slate-400">{preview.result.positions.length} 项持仓，{preview.result.executions.length} 笔可导入成交</p></div><Button type="button" disabled={busy !== null || preview.result.executions.length === 0} onClick={() => void importPreview()}>{busy === `import:${preview.connectionID}` ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}{busy === `import:${preview.connectionID}` ? '导入中' : '确认导入成交'}</Button></div>{preview.result.warnings.length > 0 ? <ul className="mt-3 list-disc space-y-1 pl-5 text-xs text-amber-200">{preview.result.warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul> : null}<div className="mt-4 overflow-x-auto"><table className="w-full min-w-[620px] text-left text-xs"><thead className="text-slate-500"><tr><th className="pb-2">时间</th><th className="pb-2">标的</th><th className="pb-2">方向</th><th className="pb-2">数量</th><th className="pb-2">价格</th><th className="pb-2">费用</th></tr></thead><tbody>{preview.result.executions.slice(0, 10).map((item: BrokerExecution) => <tr className="border-t border-slate-800 text-slate-200" key={item.external_id}><td className="py-2">{new Date(item.executed_at).toLocaleString()}</td><td>{item.symbol}</td><td>{item.side === 'buy' ? '买入' : '卖出'}</td><td>{item.quantity}</td><td>{item.price}</td><td>{item.fee}</td></tr>)}</tbody></table></div>{preview.result.executions.length > 10 ? <p className="mt-2 text-xs text-slate-500">仅展示前 10 笔，确认后将导入全部预览成交。</p> : null}</div> : null}
    </Card>
  );
}
