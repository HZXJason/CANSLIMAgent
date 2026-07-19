import { Download, Loader2, Upload, X } from 'lucide-react';
import { useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { Input } from '../ui/input';
import { importBrokerExecutions, syncBrokerConnection } from '../../lib/api';
import type { BrokerConnection, BrokerExecution, BrokerSyncResult } from '../../types/broker';

type BrokerJournalImportProps = { connections: BrokerConnection[]; onImported: () => void };

const dateValue = (date: Date) => date.toISOString().slice(0, 10);
const defaultEndDate = dateValue(new Date());
const defaultStartDate = (() => {
  const date = new Date();
  date.setUTCFullYear(date.getUTCFullYear() - 1);
  return dateValue(date);
})();

export default function BrokerJournalImport({ connections, onImported }: BrokerJournalImportProps) {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [preview, setPreview] = useState<{ connectionID: string; result: BrokerSyncResult } | null>(null);
  const [startDate, setStartDate] = useState(defaultStartDate);
  const [endDate, setEndDate] = useState(defaultEndDate);
  const connected = connections.filter((connection) => connection.connected && connection.id);

  const sync = async (connection: BrokerConnection) => {
    if (startDate > endDate) {
      setError('开始日期不能晚于结束日期');
      return;
    }
    setBusy(`sync:${connection.id}`); setError(null); setMessage(null);
    try {
      const result = await syncBrokerConnection(connection.id, {
        start_at: `${startDate}T00:00:00.000Z`,
        end_at: `${endDate}T23:59:59.999Z`,
      });
      setPreview({ connectionID: connection.id, result });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '券商交易日志读取失败');
    } finally {
      setBusy(null);
    }
  };

  const importPreview = async () => {
    if (!preview) return;
    setBusy(`import:${preview.connectionID}`); setError(null);
    try {
      const result = await importBrokerExecutions(preview.connectionID, preview.result.executions);
      setPreview(null);
      setMessage(`已导入 ${result.created} 笔成交，跳过 ${result.skipped} 笔重复记录`);
      onImported();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '交易日志导入失败');
    } finally {
      setBusy(null);
    }
  };

  return (
    <Card className="overflow-hidden p-0">
      <div className="border-b border-slate-800 px-5 py-4 sm:px-6">
        <h2 className="text-lg font-bold text-white">从券商导入</h2>
      </div>

      <div className="space-y-4 p-5 sm:p-6">
        {connected.length === 0 ? (
          <div className="flex flex-col gap-3 rounded-xl border border-slate-800 bg-slate-950/35 p-4 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-slate-300">尚未连接券商账户</p>
            <Button type="button" variant="outline" onClick={() => window.location.assign('/settings')}>前往设置</Button>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-sm font-semibold text-slate-300">
                开始日期
                <Input className="mt-2" aria-label="开始日期" type="date" value={startDate} max={endDate} disabled={busy !== null} onChange={(event) => setStartDate(event.target.value)} />
              </label>
              <label className="text-sm font-semibold text-slate-300">
                结束日期
                <Input className="mt-2" aria-label="结束日期" type="date" value={endDate} min={startDate} max={defaultEndDate} disabled={busy !== null} onChange={(event) => setEndDate(event.target.value)} />
              </label>
            </div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {connected.map((connection) => (
                <Button key={connection.id} className="min-w-0 w-full" type="button" variant="outline" disabled={busy !== null} onClick={() => void sync(connection)}>
                  {busy === `sync:${connection.id}` ? <Loader2 className="h-4 w-4 shrink-0 animate-spin" /> : <Download className="h-4 w-4 shrink-0" />}
                  <span className="truncate">{busy === `sync:${connection.id}` ? '读取中' : `获取 ${connection.label || connection.account_name || connection.broker} 成交`}</span>
                </Button>
              ))}
            </div>
          </div>
        )}

        {connected.length > 0 ? <p className="text-sm text-slate-400">最多读取一年；确认预览后才会写入日志。</p> : null}
        {error ? <p className="rounded-xl border border-red-800 bg-red-950/30 p-3 text-sm text-red-200">{error}</p> : null}
        {message ? <p className="rounded-xl border border-brand-900 bg-brand-950/20 p-3 text-sm text-brand-100">{message}</p> : null}

        {preview ? (
          <div className="rounded-2xl border border-brand-900/70 bg-brand-950/20 p-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h3 className="font-bold text-brand-100">导入预览</h3>
                <p className="mt-1 text-sm text-slate-300">{preview.result.executions.length} 笔可导入成交</p>
              </div>
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button type="button" variant="outline" disabled={busy !== null} onClick={() => setPreview(null)}>
                  <X className="h-4 w-4" />
                  取消
                </Button>
                <Button type="button" disabled={busy !== null || preview.result.executions.length === 0} onClick={() => void importPreview()}>
                  {busy === `import:${preview.connectionID}` ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
                  {busy === `import:${preview.connectionID}` ? '导入中' : '确认导入成交'}
                </Button>
              </div>
            </div>
            {preview.result.warnings.length > 0 ? <ul className="mt-3 list-disc space-y-1 pl-5 text-sm text-amber-200">{preview.result.warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul> : null}
            <div className="mt-4 overflow-x-auto">
              <table className="w-full min-w-[620px] text-left text-sm">
                <thead className="text-slate-400"><tr><th className="pb-2">时间</th><th className="pb-2">标的</th><th className="pb-2">方向</th><th className="pb-2">数量</th><th className="pb-2">价格</th><th className="pb-2">费用</th></tr></thead>
                <tbody>{preview.result.executions.slice(0, 10).map((item: BrokerExecution) => <tr className="border-t border-slate-800 text-slate-200" key={item.external_id}><td className="py-2">{new Date(item.executed_at).toLocaleString()}</td><td>{item.symbol}</td><td>{item.side === 'buy' ? '买入' : '卖出'}</td><td>{item.quantity}</td><td>{item.price}</td><td>{item.fee}</td></tr>)}</tbody>
              </table>
            </div>
            {preview.result.executions.length > 10 ? <p className="mt-2 text-sm text-slate-400">仅展示前 10 笔，确认后将导入全部预览成交。</p> : null}
          </div>
        ) : null}
      </div>
    </Card>
  );
}
