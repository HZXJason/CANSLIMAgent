import { BookOpenCheck, Circle, CircleCheck, Plus, RefreshCw } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { fetchJournalEntries } from '../../lib/api';
import { cn, formatDateCN } from '../../lib/utils';
import type { JournalEntry } from '../../types/journal';

function directionLabel(direction: JournalEntry['direction']) {
  return direction === 'buy' ? '买入' : '卖出';
}

function DirectionBadge({ direction }: { direction: JournalEntry['direction'] }) {
  return (
    <span
      className={cn(
        'inline-flex h-8 items-center justify-center rounded-full border px-3 text-xs font-bold',
        direction === 'buy'
          ? 'border-emerald-400/70 bg-emerald-400 text-slate-950'
          : 'border-red-400/70 bg-red-500 text-white'
      )}
    >
      {directionLabel(direction)}
    </span>
  );
}

function hasReflection(entry: JournalEntry) {
  return entry.exit_price !== null || entry.pnl_pct !== null || Boolean(entry.reflection?.trim());
}

function formatMoney(value: number | null) {
  if (value === null) return '-';
  return value.toLocaleString('en-US', { maximumFractionDigits: 4 });
}

function ReflectionStatus({ entry }: { entry: JournalEntry }) {
  const reflected = hasReflection(entry);
  return (
    <span
      className={cn(
        'inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold',
        reflected
          ? 'border-cyan-400/50 bg-cyan-400/10 text-cyan-200'
          : 'border-slate-700 bg-slate-950/30 text-slate-500'
      )}
    >
      {reflected ? <CircleCheck className="h-3.5 w-3.5" /> : <Circle className="h-3.5 w-3.5" />}
      {reflected ? '已反思' : '待反思'}
    </span>
  );
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <Card className="flex min-h-[360px] flex-col items-center justify-center gap-4 text-center">
      <BookOpenCheck className="h-12 w-12 text-slate-600" />
      <p className="text-lg font-semibold text-slate-200">暂无交易记录。点击新建记录开始。</p>
      <Button onClick={onNew}>
        <Plus className="h-4 w-4" />
        新建记录
      </Button>
    </Card>
  );
}

export default function JournalList() {
  const navigate = useNavigate();
  const journalQuery = useQuery({
    queryKey: ['journal-entries'],
    queryFn: fetchJournalEntries
  });

  const entries = journalQuery.data ?? [];

  return (
    <section className="mx-auto flex w-full max-w-[1180px] flex-col gap-6 px-4 py-6 lg:px-8">
      <header className="flex flex-col gap-4 rounded-[1.25rem] border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-sm font-semibold text-cyan-300/80">AlphaLens · Trade Journal</p>
          <h1 className="mt-2 text-3xl font-black tracking-tight text-white">交易日志</h1>
        </div>
        <Button onClick={() => navigate('/journal/new')}>
          <Plus className="h-4 w-4" />
          新建记录
        </Button>
      </header>

      {journalQuery.isLoading ? (
        <Card className="space-y-3">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-16 animate-pulse rounded-2xl bg-slate-800/60" />
          ))}
        </Card>
      ) : null}

      {journalQuery.isError ? (
        <Card className="flex min-h-[280px] flex-col items-center justify-center gap-4 text-center">
          <p className="text-lg font-semibold text-red-100">交易日志读取失败</p>
          <p className="text-sm text-slate-500">
            {journalQuery.error instanceof Error ? journalQuery.error.message : '请稍后重试'}
          </p>
          <Button variant="outline" onClick={() => void journalQuery.refetch()}>
            <RefreshCw className="h-4 w-4" />
            重试
          </Button>
        </Card>
      ) : null}

      {!journalQuery.isLoading && !journalQuery.isError && entries.length === 0 ? (
        <EmptyState onNew={() => navigate('/journal/new')} />
      ) : null}

      {!journalQuery.isLoading && !journalQuery.isError && entries.length > 0 ? (
        <Card className="overflow-hidden p-0">
          <div className="hidden grid-cols-[1fr_0.8fr_1fr_1fr_1fr] gap-4 border-b border-slate-800 px-5 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 md:grid">
            <span>代码</span>
            <span>方向</span>
            <span>交易日期</span>
            <span>价格 / 仓位</span>
            <span className="text-right">反思</span>
          </div>
          <div className="divide-y divide-slate-900">
            {entries.map((entry, index) => (
              <button
                key={entry.id}
                className="grid w-full gap-3 px-5 py-4 text-left transition hover:bg-slate-800/70 md:grid-cols-[1fr_0.8fr_1fr_1fr_1fr] md:items-center md:gap-4"
                style={{ animationDelay: index * 30 + 'ms' }}
                onClick={() => navigate('/journal/' + entry.id)}
              >
                <span className="min-w-0">
                  <span className="block truncate text-lg font-black text-white">{entry.ticker}</span>
                  <span className="mt-1 block text-xs text-slate-500">止损 {formatMoney(entry.stop_loss)}</span>
                </span>
                <span>
                  <DirectionBadge direction={entry.direction} />
                </span>
                <span className="text-sm font-semibold text-slate-300">{formatDateCN(entry.trade_date)}</span>
                <span className="text-sm text-slate-400">
                  {formatMoney(entry.price)} · {entry.position_size.toFixed(2)}%
                </span>
                <span className="flex md:justify-end">
                  <ReflectionStatus entry={entry} />
                </span>
              </button>
            ))}
          </div>
        </Card>
      ) : null}
    </section>
  );
}
