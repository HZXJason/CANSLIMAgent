import { ClipboardList, Clock3, FilePenLine, Plus, RefreshCw, XCircle } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Select } from '../../components/ui/select';
import { fetchTradePlans } from '../../lib/api';
import { cn, formatDateCN } from '../../lib/utils';
import type { TradePlan, TradePlanStatus } from '../../types/tradePlan';

type StatusFilter = TradePlanStatus | 'all';

const statusLabel: Record<TradePlanStatus, string> = {
  draft: '草稿',
  waiting: '等待触发',
  executed: '已执行',
  cancelled: '已取消',
  expired: '已过期'
};

const directionLabel: Record<TradePlan['direction'], string> = {
  buy: '买入',
  sell: '卖出',
  watch: '观察'
};

const statusClass: Record<TradePlanStatus, string> = {
  draft: 'border-slate-700 bg-slate-950/30 text-slate-300',
  waiting: 'border-cyan-400/50 bg-cyan-400/10 text-cyan-200',
  executed: 'border-emerald-400/60 bg-emerald-400/15 text-emerald-200',
  cancelled: 'border-red-400/50 bg-red-950/50 text-red-100',
  expired: 'border-amber-400/50 bg-amber-950/40 text-amber-100'
};

const directionClass: Record<TradePlan['direction'], string> = {
  buy: 'border-emerald-400/70 bg-emerald-400 text-slate-950',
  sell: 'border-red-400/70 bg-red-500 text-white',
  watch: 'border-slate-600 bg-slate-800 text-slate-100'
};

function StatusBadge({ status }: { status: TradePlanStatus }) {
  return <span className={cn('inline-flex h-8 items-center rounded-full border px-3 text-xs font-bold', statusClass[status])}>{statusLabel[status]}</span>;
}

function DirectionBadge({ direction }: { direction: TradePlan['direction'] }) {
  return <span className={cn('inline-flex h-8 items-center rounded-full border px-3 text-xs font-bold', directionClass[direction])}>{directionLabel[direction]}</span>;
}

function formatMoney(value: number | null) {
  if (value === null) return '-';
  return value.toLocaleString('en-US', { maximumFractionDigits: 4 });
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <Card className="flex min-h-[360px] flex-col items-center justify-center gap-4 text-center">
      <ClipboardList className="h-12 w-12 text-slate-600" />
      <p className="text-lg font-semibold text-slate-200">暂无交易计划</p>
      <Button onClick={onNew}>
        <Plus className="h-4 w-4" />
        新建计划
      </Button>
    </Card>
  );
}

function PlanStatusIcon({ status }: { status: TradePlanStatus }) {
  if (status === 'cancelled' || status === 'expired') return <XCircle className="h-4 w-4" />;
  if (status === 'draft') return <FilePenLine className="h-4 w-4" />;
  return <Clock3 className="h-4 w-4" />;
}

export default function TradePlanList() {
  const navigate = useNavigate();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const filters = useMemo(() => (statusFilter === 'all' ? {} : { status: statusFilter }), [statusFilter]);
  const plansQuery = useQuery({
    queryKey: ['trade-plans', filters],
    queryFn: () => fetchTradePlans(filters)
  });

  const plans = plansQuery.data ?? [];

  return (
    <section className="mx-auto flex w-full max-w-[1180px] flex-col gap-6 px-4 py-6 lg:px-8">
      <header className="flex flex-col gap-4 rounded-[1.25rem] border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-sm font-semibold text-cyan-300/80">AlphaLens · Trade Plans</p>
          <h1 className="mt-2 text-3xl font-black tracking-tight text-white">交易计划</h1>
        </div>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <Select value={statusFilter} onValueChange={(value) => setStatusFilter(value as StatusFilter)} className="min-w-36">
            <option value="all">全部状态</option>
            {Object.entries(statusLabel).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
          <Button onClick={() => navigate('/trade-plans/new')}>
            <Plus className="h-4 w-4" />
            新建计划
          </Button>
        </div>
      </header>

      {plansQuery.isLoading ? (
        <Card className="space-y-3">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-16 animate-pulse rounded-2xl bg-slate-800/60" />
          ))}
        </Card>
      ) : null}

      {plansQuery.isError ? (
        <Card className="flex min-h-[280px] flex-col items-center justify-center gap-4 text-center">
          <p className="text-lg font-semibold text-red-100">交易计划读取失败</p>
          <p className="text-sm text-slate-500">{plansQuery.error instanceof Error ? plansQuery.error.message : '请稍后重试'}</p>
          <Button variant="outline" onClick={() => void plansQuery.refetch()}>
            <RefreshCw className="h-4 w-4" />
            重试
          </Button>
        </Card>
      ) : null}

      {!plansQuery.isLoading && !plansQuery.isError && plans.length === 0 ? <EmptyState onNew={() => navigate('/trade-plans/new')} /> : null}

      {!plansQuery.isLoading && !plansQuery.isError && plans.length > 0 ? (
        <Card className="overflow-hidden p-0">
          <div className="hidden grid-cols-[1fr_0.8fr_0.9fr_1fr_1.2fr] gap-4 border-b border-slate-800 px-5 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 md:grid">
            <span>代码</span>
            <span>方向</span>
            <span>状态</span>
            <span>计划日期</span>
            <span>风险</span>
          </div>
          <div className="divide-y divide-slate-900">
            {plans.map((plan, index) => (
              <button
                key={plan.id}
                className="grid w-full gap-3 px-5 py-4 text-left transition hover:bg-slate-800/70 md:grid-cols-[1fr_0.8fr_0.9fr_1fr_1.2fr] md:items-center md:gap-4"
                style={{ animationDelay: index * 30 + 'ms' }}
                onClick={() => navigate('/trade-plans/' + plan.id)}
              >
                <span className="min-w-0">
                  <span className="flex items-center gap-2 text-lg font-black text-white">
                    <PlanStatusIcon status={plan.status} />
                    {plan.ticker}
                  </span>
                  <span className="mt-1 block truncate text-xs text-slate-500">{plan.entry_trigger || '未填写触发条件'}</span>
                </span>
                <span>
                  <DirectionBadge direction={plan.direction} />
                </span>
                <span>
                  <StatusBadge status={plan.status} />
                </span>
                <span className="text-sm font-semibold text-slate-300">{formatDateCN(plan.plan_date)}</span>
                <span className="text-sm text-slate-400">
                  入场 {formatMoney(plan.entry_price)} · 止损 {formatMoney(plan.stop_loss)}
                </span>
              </button>
            ))}
          </div>
        </Card>
      ) : null}
    </section>
  );
}
