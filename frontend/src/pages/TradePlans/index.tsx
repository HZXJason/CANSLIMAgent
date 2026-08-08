import { ChevronRight, ClipboardList, Clock3, FilePenLine, Plus, RefreshCw, XCircle } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Select } from '../../components/ui/select';
import { fetchTradePlans } from '../../lib/api';
import { displaySymbol } from '../../lib/markets';
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
  waiting: 'border-brand-400/50 bg-brand-400/10 text-brand-200',
  executed: 'border-emerald-400/60 bg-emerald-400/15 text-emerald-200',
  cancelled: 'border-red-400/50 bg-red-950/50 text-red-100',
  expired: 'border-amber-400/50 bg-amber-950/40 text-amber-100'
};

const directionClass: Record<TradePlan['direction'], string> = {
  buy: 'border-emerald-400/70 bg-emerald-400 text-emerald-950',
  sell: 'border-red-400/70 bg-red-500 text-white',
  watch: 'border-slate-600 bg-slate-800 text-slate-100'
};

function StatusBadge({ status }: { status: TradePlanStatus }) {
  return <span className={cn('font-data inline-flex h-7 items-center rounded-sm border px-2.5 text-xs font-bold', statusClass[status])}>{statusLabel[status]}</span>;
}

function DirectionBadge({ direction }: { direction: TradePlan['direction'] }) {
  return <span className={cn('font-data inline-flex h-7 items-center rounded-sm border px-2.5 text-xs font-bold', directionClass[direction])}>{directionLabel[direction]}</span>;
}

function formatMoney(value: number | null) {
  if (value === null) return '-';
  return value.toLocaleString('en-US', { maximumFractionDigits: 4 });
}

function EmptyState() {
  return (
    <Card className="flex min-h-[280px] flex-col items-center justify-center gap-4 text-center">
      <ClipboardList className="h-10 w-10 text-slate-600" />
      <p className="text-lg font-semibold text-slate-200">暂无交易计划</p>
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
    <section className="page-ledger mx-auto flex w-full max-w-[1180px] flex-col gap-6 px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader
        title="交易计划"
        meta={plansQuery.isLoading ? '正在同步计划' : `${plans.length} 个计划`}
        actions={
          <>
            <div className="w-full sm:w-40">
              <Select aria-label="按状态筛选" value={statusFilter} onValueChange={(value) => setStatusFilter(value as StatusFilter)}>
                <option value="all">全部状态</option>
                {Object.entries(statusLabel).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </Select>
            </div>
            <Button className="w-full sm:w-auto" onClick={() => navigate('/trade-plans/new')}>
              <Plus className="h-4 w-4" />
              新建计划
            </Button>
          </>
        }
      />

      {plansQuery.isLoading ? (
        <Card className="space-y-3 p-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-14 animate-pulse rounded-xl bg-slate-800/60" />
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

      {!plansQuery.isLoading && !plansQuery.isError && plans.length === 0 ? <EmptyState /> : null}

      {!plansQuery.isLoading && !plansQuery.isError && plans.length > 0 ? (
        <Card className="overflow-hidden p-0">
          <div className="hidden grid-cols-[1.15fr_0.7fr_0.85fr_1fr_1.1fr] gap-4 border-b border-slate-800 px-5 py-3 text-sm font-semibold text-slate-400 md:grid">
            <span>代码</span>
            <span>方向</span>
            <span>状态</span>
            <span>计划日期</span>
            <span>入场 / 止损</span>
          </div>
          <div className="divide-y divide-slate-900">
            {plans.map((plan) => (
              <button
                key={plan.id}
                className="group grid w-full gap-3 px-4 py-4 text-left transition-[background-color,box-shadow] duration-150 hover:bg-slate-800/55 active:bg-slate-800/75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-400/60 sm:px-5 md:grid-cols-[1.15fr_0.7fr_0.85fr_1fr_1.1fr] md:items-center md:gap-4"
                onClick={() => navigate('/trade-plans/' + plan.id)}
              >
                <span className="flex min-w-0 items-start justify-between gap-3 md:block">
                  <span>
                    <span className="flex items-center gap-2 text-lg font-black text-white">
                    <PlanStatusIcon status={plan.status} />
                    {displaySymbol(plan.ticker)}
                    </span>
                    {plan.entry_trigger ? <span className="mt-1 block line-clamp-2 text-sm leading-5 text-slate-400 md:truncate">{plan.entry_trigger}</span> : null}
                  </span>
                  <ChevronRight className="mt-1 h-5 w-5 shrink-0 text-slate-600 transition-transform duration-150 group-hover:translate-x-0.5 group-hover:text-slate-400 md:hidden" />
                </span>
                <span className="flex items-center gap-3">
                  <span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">方向</span>
                  <DirectionBadge direction={plan.direction} />
                </span>
                <span className="flex items-center gap-3">
                  <span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">状态</span>
                  <StatusBadge status={plan.status} />
                </span>
                <span className="flex items-center gap-3 text-sm font-semibold text-slate-300">
                  <span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">日期</span>
                  {formatDateCN(plan.plan_date)}
                </span>
                <span className="flex items-center gap-3 text-sm font-medium text-slate-300 md:block">
                  <span className="w-12 shrink-0 text-xs font-semibold text-slate-500 md:hidden">价格</span>
                  <span className="md:hidden">入场 {formatMoney(plan.entry_price)} / 止损 {formatMoney(plan.stop_loss)}</span>
                  <span className="font-data hidden md:inline">{formatMoney(plan.entry_price)} / {formatMoney(plan.stop_loss)}</span>
                </span>
              </button>
            ))}
          </div>
        </Card>
      ) : null}
    </section>
  );
}
