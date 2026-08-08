import { ClipboardList, Clock3, Plus } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { fetchTradePlans } from '../../lib/api';
import { displaySymbol } from '../../lib/markets';
import { cn, formatDateCN } from '../../lib/utils';
import type { TradePlan, TradePlanStatus } from '../../types/tradePlan';

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

function formatMoney(value: number | null) {
  if (value === null) return '-';
  return value.toLocaleString('en-US', { maximumFractionDigits: 4 });
}

function StatusBadge({ status }: { status: TradePlanStatus }) {
  return <span className={cn('inline-flex h-7 items-center rounded-full border px-2.5 text-xs font-bold', statusClass[status])}>{statusLabel[status]}</span>;
}

export default function TradePlanPanel({ symbol, aiSummary, userThesisId }: { symbol: string; aiSummary?: string; userThesisId?: string }) {
  const navigate = useNavigate();
  const visibleSymbol = displaySymbol(symbol);
  const plansQuery = useQuery({
    queryKey: ['trade-plans', { ticker: symbol }],
    queryFn: () => fetchTradePlans({ ticker: symbol })
  });

  const activePlans = (plansQuery.data ?? []).filter((plan) => plan.status === 'draft' || plan.status === 'waiting').slice(0, 3);
  const createPlan = () => {
    navigate('/trade-plans/new?symbol=' + encodeURIComponent(symbol), {
      state: aiSummary || userThesisId ? { aiSummary, userThesisId } : undefined
    });
  };

  return (
    <Card className="p-4 sm:p-5">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h2 className="text-lg font-black text-white">交易计划</h2>
        {activePlans.length > 0 ? (
          <Button className="h-10 rounded-xl" onClick={createPlan}>
            <Plus className="h-4 w-4" />
            新建计划
          </Button>
        ) : null}
      </div>

      {plansQuery.isLoading ? (
        <div className="grid gap-3 md:grid-cols-3">
          {Array.from({ length: 3 }).map((_, index) => (
            <div key={index} className="h-24 animate-pulse rounded-2xl bg-slate-800/60" />
          ))}
        </div>
      ) : null}

      {plansQuery.isError ? <div className="rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">交易计划读取失败</div> : null}

      {!plansQuery.isLoading && !plansQuery.isError && activePlans.length === 0 ? (
        <button className="flex min-h-24 w-full items-center justify-center gap-3 rounded-xl border border-dashed border-slate-700 bg-slate-950/25 text-sm font-semibold text-slate-400 transition hover:border-brand-400/50 hover:text-brand-200" onClick={createPlan}>
          <ClipboardList className="h-5 w-5" />
          新建 {visibleSymbol} 交易计划
        </button>
      ) : null}

      {!plansQuery.isLoading && !plansQuery.isError && activePlans.length > 0 ? (
        <div className="grid gap-3 lg:grid-cols-3">
          {activePlans.map((plan) => (
            <button
              key={plan.id}
              className="min-h-28 rounded-xl border border-slate-800 bg-slate-950/35 p-4 text-left transition hover:border-brand-400/40 hover:bg-slate-900"
              onClick={() => navigate('/trade-plans/' + plan.id)}
            >
              <div className="flex items-start justify-between gap-3">
                <p className="text-base font-black text-white">{directionLabel[plan.direction]}</p>
                <StatusBadge status={plan.status} />
              </div>
              <p className="mt-3 line-clamp-2 text-sm leading-6 text-slate-300">{plan.entry_trigger || '未填写触发条件'}</p>
              <div className="mt-3 flex flex-wrap items-center gap-3 text-xs font-semibold text-slate-500">
                <span className="inline-flex items-center gap-1">
                  <Clock3 className="h-3.5 w-3.5 text-brand-300" />
                  {formatDateCN(plan.plan_date)}
                </span>
                <span>入场 {formatMoney(plan.entry_price)}</span>
                <span>止损 {formatMoney(plan.stop_loss)}</span>
              </div>
            </button>
          ))}
        </div>
      ) : null}
    </Card>
  );
}
