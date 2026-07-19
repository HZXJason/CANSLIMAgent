import { BookOpenCheck, BrainCircuit, ChevronRight, Circle, CircleCheck, Plus, RefreshCw } from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import MarkdownContent from '../../components/analysis/MarkdownContent';
import BrokerJournalImport from '../../components/journal/BrokerJournalImport';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { fetchBrokerConnections, fetchJournalEntries, fetchLatestJournalReflection, generateJournalReflection } from '../../lib/api';
import { cn } from '../../lib/utils';
import type { JournalEntry, StoredJournalReflection } from '../../types/journal';

function SideBadge({ side }: { side: JournalEntry['side'] }) {
  return (
    <span className={cn('inline-flex h-8 items-center rounded-full border px-3 text-xs font-bold', side === 'buy' ? 'border-emerald-400 bg-emerald-400 text-slate-950' : 'border-red-400 bg-red-500 text-white')}>
      {side === 'buy' ? '买入' : '卖出'}
    </span>
  );
}

function ReflectionStatus({ entry }: { entry: JournalEntry }) {
  const reflected = Boolean(entry.reflection?.trim());
  return (
    <span className={cn('inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold', reflected ? 'border-cyan-400/50 bg-cyan-400/10 text-cyan-200' : 'border-slate-700 bg-slate-950/30 text-slate-500')}>
      {reflected ? <CircleCheck className="h-3.5 w-3.5" /> : <Circle className="h-3.5 w-3.5" />}
      {reflected ? '已复盘' : '待复盘'}
    </span>
  );
}

function formatNumber(value: number) {
  return value.toLocaleString('en-US', { maximumFractionDigits: 8 });
}

function formatPercent(value: number | undefined) {
  return value === undefined ? '-' : (value * 100).toFixed(1) + '%';
}

function ReflectionCard({ reflection }: { reflection: StoredJournalReflection }) {
  const profile = reflection.payload.profile;
  const warnings = (reflection.payload.validation_warnings ?? []).map((warning) =>
    typeof warning === 'string' ? warning : warning.message || warning.code || '部分日志数据不完整'
  );
  return (
    <Card>
      <div>
        <h2 className="text-xl font-bold text-white">交易表现</h2>
        <p className="mt-2 text-sm text-slate-400">{new Date(reflection.created_at).toLocaleString()} · {reflection.record_count} 笔成交</p>
      </div>
      {profile ? (
        <div className="mt-5 grid grid-cols-2 gap-3 md:grid-cols-4">
          <Metric label="完整来回" value={String(profile.total_roundtrips ?? 0)} />
          <Metric label="胜率" value={formatPercent(profile.win_rate)} />
          <Metric label="平均持仓" value={profile.avg_holding_days === undefined ? '-' : profile.avg_holding_days.toFixed(2) + ' 天'} />
          <Metric label="累计盈亏" value={profile.total_pnl === undefined ? '-' : formatNumber(profile.total_pnl)} />
        </div>
      ) : null}
      {reflection.payload.reflection_text ? <div className="mt-5 rounded-2xl border border-slate-800 bg-slate-950/35 p-4"><MarkdownContent content={reflection.payload.reflection_text} /></div> : null}
      {warnings.length > 0 ? <div className="mt-4 rounded-2xl border border-amber-400/20 bg-amber-950/20 px-4 py-3 text-sm text-amber-100">{warnings.join('；')}</div> : null}
    </Card>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div className="rounded-2xl border border-slate-800 bg-slate-950/35 px-4 py-3"><p className="text-sm font-semibold text-slate-400">{label}</p><p className="mt-1 text-lg font-black text-white">{value}</p></div>;
}

export default function JournalList() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ['journal-entries'], queryFn: fetchJournalEntries });
  const brokerQuery = useQuery({ queryKey: ['broker-connections'], queryFn: fetchBrokerConnections });
  const reflectionQuery = useQuery({ queryKey: ['journal-reflection-latest'], queryFn: fetchLatestJournalReflection });
  const reflectionMutation = useMutation({
    mutationFn: generateJournalReflection,
    onSuccess: (reflection) => queryClient.setQueryData(['journal-reflection-latest'], reflection)
  });
  const entries = query.data ?? [];
  const structuredEntries = entries.filter((entry) => entry.quantity > 0 && entry.price > 0);

  return (
    <section className="mx-auto flex w-full max-w-[1180px] flex-col gap-6 px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader
        eyebrow="决策复盘"
        title="交易日志"
        description="把成交、来源与复盘放在同一条记录链路中，持续校准自己的执行。"
        meta={query.isLoading ? '正在同步成交记录' : `${entries.length} 笔成交 · ${structuredEntries.length} 笔可纳入分析`}
        actions={
          <>
            <Button className="order-2 w-full sm:order-1 sm:w-auto" variant="outline" disabled={structuredEntries.length === 0 || reflectionMutation.isPending} onClick={() => reflectionMutation.mutate()}>
              <BrainCircuit className="h-4 w-4" />
              {reflectionMutation.isPending ? '分析中' : '分析交易表现'}
            </Button>
            <Button className="order-1 w-full sm:order-2 sm:w-auto" onClick={() => navigate('/journal/new')}><Plus className="h-4 w-4" />新建成交</Button>
          </>
        }
      />

      <BrokerJournalImport connections={brokerQuery.data ?? []} onImported={() => { void queryClient.invalidateQueries({ queryKey: ['journal-entries'] }); }} />

      {reflectionMutation.isError ? <Card className="border-red-800 bg-red-950/30 text-sm text-red-100">{reflectionMutation.error instanceof Error ? reflectionMutation.error.message : '行为反思生成失败'}</Card> : null}
      {reflectionQuery.data ? <ReflectionCard reflection={reflectionQuery.data} /> : null}
      {query.isLoading ? <Card className="h-64 animate-pulse bg-slate-900/60" /> : null}
      {query.isError ? <Card className="flex min-h-64 flex-col items-center justify-center gap-4"><p className="text-red-100">交易日志读取失败</p><Button variant="outline" onClick={() => void query.refetch()}><RefreshCw className="h-4 w-4" />重试</Button></Card> : null}
      {!query.isLoading && !query.isError && entries.length === 0 ? <Card className="flex min-h-[280px] flex-col items-center justify-center gap-4 text-center"><BookOpenCheck className="h-10 w-10 text-slate-600" /><p className="text-lg font-semibold text-slate-200">暂无成交记录</p></Card> : null}
      {entries.length > 0 ? (
        <Card className="overflow-hidden p-0">
          <div className="hidden grid-cols-[1.1fr_0.7fr_1.2fr_1fr_1fr_0.8fr] gap-4 border-b border-slate-800 px-5 py-3 text-sm font-semibold text-slate-400 md:grid"><span>标的</span><span>方向</span><span>成交时间</span><span>数量 × 价格</span><span>成交额 / 手续费</span><span className="text-right">复盘</span></div>
          <div className="divide-y divide-slate-900">
            {entries.map((entry) => (
              <button key={entry.id} className="group grid w-full gap-3 px-4 py-4 text-left transition-[background-color,box-shadow] duration-150 hover:bg-slate-800/55 active:bg-slate-800/75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-cyan-400/60 sm:px-5 md:grid-cols-[1.1fr_0.7fr_1.2fr_1fr_1fr_0.8fr] md:items-center md:gap-4" onClick={() => navigate('/journal/' + entry.id)}>
                <span className="flex items-start justify-between gap-3 md:block">
                  <span><span className="block text-lg font-black text-white">{entry.symbol}</span><span className="text-sm text-slate-400">{entry.name || entry.market}</span></span>
                  <ChevronRight className="mt-1 h-5 w-5 shrink-0 text-slate-600 transition-transform duration-150 group-hover:translate-x-0.5 group-hover:text-slate-400 md:hidden" />
                </span>
                <span className="flex items-center gap-3"><span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">方向</span><SideBadge side={entry.side} /></span>
                <span className="flex items-center gap-3 text-sm text-slate-300"><span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">时间</span>{new Date(entry.datetime).toLocaleString()}</span>
                <span className="flex items-center gap-3 text-sm text-slate-400"><span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">成交</span>{formatNumber(entry.quantity)} × {formatNumber(entry.price)}</span>
                <span className="flex items-center gap-3 text-sm text-slate-400"><span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">金额</span>{formatNumber(entry.amount)} / {formatNumber(entry.fee)}</span>
                <span className="flex items-center gap-3 md:justify-end"><span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">复盘</span><ReflectionStatus entry={entry} /></span>
              </button>
            ))}
          </div>
        </Card>
      ) : null}
    </section>
  );
}
