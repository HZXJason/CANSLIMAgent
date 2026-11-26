import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, ArrowRight, CheckCircle2, Flame, ShieldAlert, TrendingUp } from 'lucide-react';
import { Link } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { fetchDesk } from '../../lib/deskApi';
import { displaySymbol } from '../../lib/markets';
import { updateTradeStopLoss } from '../../lib/tradesApi';
import { cn } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import type { MarketRegimeStatus } from '../../types/desk';
import { MarketClock } from './MarketClock';

const regimeStyles: Record<MarketRegimeStatus, string> = {
  confirmed_uptrend: 'border-emerald-400/50 bg-emerald-500/15 text-emerald-200',
  uptrend_under_pressure: 'border-amber-400/50 bg-amber-500/15 text-amber-100',
  rally_attempt: 'border-amber-400/40 bg-amber-500/10 text-amber-200',
  correction: 'border-rose-500/50 bg-rose-500/15 text-rose-100',
  unavailable: 'border-slate-700 bg-slate-900/60 text-slate-300',
  insufficient_data: 'border-slate-700 bg-slate-900/60 text-slate-300'
};

export default function DeskPage() {
  const market = useMarketStore((state) => state.assetMarket);
  const queryClient = useQueryClient();
  const [stopDrafts, setStopDrafts] = useState<Record<string, string>>({});
  const [notice, setNotice] = useState<string>('');

  const deskQuery = useQuery({
    queryKey: ['desk', market],
    queryFn: () => fetchDesk(market),
    refetchInterval: 60_000
  });

  const stopMutation = useMutation({
    mutationFn: ({ id, stopLoss }: { id: string; stopLoss: number }) => updateTradeStopLoss(id, stopLoss),
    onSuccess: (updated) => {
      setNotice(`已更新 ${displaySymbol(updated.symbol)} 止损至 ${updated.current_stop_loss}`);
      void queryClient.invalidateQueries({ queryKey: ['desk', market] });
      void queryClient.invalidateQueries({ queryKey: ['trades'] });
    }
  });

  const desk = deskQuery.data;
  const regime = desk?.market_regime;
  const summary = desk?.summary;
  const heatRatio = summary && summary.max_portfolio_heat_percent > 0
    ? Math.min(100, Math.round((summary.open_portfolio_risk_percent / summary.max_portfolio_heat_percent) * 100))
    : 0;

  return (
    <div className="mx-auto w-full max-w-[1380px] px-4 py-6 sm:px-6 lg:py-8">
      <PageHeader
        title="作战室"
        meta={desk ? <MarketClock timeZone={desk.timezone} session={desk.session} /> : null}
        actions={
          <div className="flex flex-wrap gap-2">
            <Link to="/research">
              <Button>去研究台立计划</Button>
            </Link>
            <Link to="/positions">
              <Button variant="outline">查看头寸簿</Button>
            </Link>
          </div>
        }
      />

      {notice ? (
        <div role="status" className="mt-4 flex items-center justify-between border border-emerald-500/35 bg-emerald-500/10 px-4 py-2.5 text-sm text-emerald-100">
          <span>{notice}</span>
          <button type="button" className="text-xs underline" onClick={() => setNotice('')}>关闭</button>
        </div>
      ) : null}

      {deskQuery.isLoading ? (
        <div className="mt-6 grid gap-4 lg:grid-cols-2">
          <Card className="h-52 animate-pulse bg-slate-900/60" />
          <Card className="h-52 animate-pulse bg-slate-900/60" />
        </div>
      ) : null}

      {deskQuery.isError ? (
        <Card className="mt-6 border-rose-500/40 p-5 text-sm text-rose-100">
          作战室数据读取失败，请确认后端服务已启动。
        </Card>
      ) : null}

      {desk && regime && summary ? (
        <div className="mt-6 space-y-6">
          {/* Top Gate Row: CANSLIM M + Portfolio Heat & North Star */}
          <div className="grid gap-4 lg:grid-cols-[1.15fr_1fr]">
            <Card className="p-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-data text-xs uppercase tracking-[0.16em] text-slate-400">
                    CANSLIM M · 大盘环境闸门 ({regime.market === 'CN' ? 'A股基准' : '美股基准'})
                  </p>
                  <div className="mt-2 flex flex-wrap items-center gap-3">
                    <span className={cn('inline-flex items-center rounded-sm border px-3 py-1 text-sm font-black', regimeStyles[regime.regime])}>
                      {regime.regime_label}
                    </span>
                    <span className="font-data text-sm font-bold text-slate-200">
                      建议总仓位上限：<span className="text-brand-200">{regime.max_exposure_percent}%</span>
                    </span>
                    <span className="font-data text-xs text-slate-400">
                      近25日派发日：{regime.distribution_days} 天
                    </span>
                  </div>
                </div>
                {regime.data_as_of ? (
                  <span className="font-data text-xs text-slate-500">截至 {regime.data_as_of}</span>
                ) : null}
              </div>

              <p className="mt-3 text-sm leading-6 text-slate-300">{regime.guidance}</p>

              <div className="mt-4 grid gap-2.5 sm:grid-cols-2">
                {regime.indices.map((idx) => (
                  <div key={idx.symbol} className="rounded-sm border border-rule bg-slate-950/55 px-3.5 py-2.5">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-bold text-white">{idx.name} <span className="font-data text-xs text-slate-400">{idx.symbol}</span></span>
                      <span className="font-data text-xs text-slate-300">{idx.regime_label}</span>
                    </div>
                    {idx.status === 'ok' ? (
                      <div className="font-data mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-400">
                        <span>收 {idx.close ?? '-'}</span>
                        <span className={idx.above_21ema ? 'text-emerald-300' : 'text-rose-300'}>21EMA {idx.ema_21 ?? '-'}</span>
                        <span className={idx.above_50sma ? 'text-emerald-300' : 'text-rose-300'}>50SMA {idx.sma_50 ?? '-'}</span>
                        <span>派发 {idx.distribution_days}d</span>
                      </div>
                    ) : (
                      <p className="mt-1.5 text-xs text-amber-200/80">{idx.error || '行情数据暂不可用'}</p>
                    )}
                  </div>
                ))}
              </div>
            </Card>

            <Card className="p-5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="font-data text-xs uppercase tracking-[0.16em] text-slate-400">
                    组合风控与北极星指标 (1R & Heat)
                  </p>
                  <div className="mt-2 flex items-baseline gap-3">
                    <span className="font-data text-3xl font-black text-white">
                      {summary.disciplined_r_loop_rate.toFixed(1)}%
                    </span>
                    <span className="text-xs font-semibold text-slate-400">
                      合规风险闭环率 ({summary.disciplined_trades}/{summary.closed_trades} 完结)
                    </span>
                  </div>
                </div>
                <Link to="/system" className="text-xs font-bold text-brand-300 hover:underline">
                  调整净值与 1R →
                </Link>
              </div>

              <div className="mt-4">
                <div className="flex items-center justify-between text-xs">
                  <span className="flex items-center gap-1.5 font-semibold text-slate-300">
                    <Flame className="h-3.5 w-3.5 text-amber-300" />
                    在途风险敞口 (Portfolio Heat)
                  </span>
                  <span className={cn('font-data font-bold', summary.open_portfolio_risk_percent > summary.max_portfolio_heat_percent ? 'text-rose-300' : 'text-slate-200')}>
                    {summary.open_portfolio_risk_percent.toFixed(2)}% / {summary.max_portfolio_heat_percent.toFixed(1)}%
                  </span>
                </div>
                <div className="mt-1.5 h-2 w-full overflow-hidden rounded-full bg-slate-900">
                  <div
                    className={cn('h-full transition-all', summary.open_portfolio_risk_percent > summary.max_portfolio_heat_percent ? 'bg-rose-500' : 'bg-brand-300')}
                    style={{ width: `${heatRatio}%` }}
                  />
                </div>
                <div className="font-data mt-3 grid grid-cols-3 gap-2 text-xs">
                  <div className="rounded-sm border border-rule bg-slate-950/50 px-2.5 py-2">
                    <span className="block text-slate-500">标准 1R</span>
                    <span className="font-bold text-white">{summary.one_r_amount > 0 ? summary.one_r_amount.toLocaleString() : '未设净值'}</span>
                  </div>
                  <div className="rounded-sm border border-rule bg-slate-950/50 px-2.5 py-2">
                    <span className="block text-slate-500">单笔期望值</span>
                    <span className={cn('font-bold', summary.expectancy_r >= 0 ? 'text-emerald-300' : 'text-rose-300')}>
                      {summary.expectancy_r >= 0 ? '+' : ''}{summary.expectancy_r.toFixed(2)}R
                    </span>
                  </div>
                  <div className="rounded-sm border border-rule bg-slate-950/50 px-2.5 py-2">
                    <span className="block text-slate-500">合规止损 (Good Loss)</span>
                    <span className="font-bold text-emerald-300">{summary.good_losses} 笔</span>
                  </div>
                </div>
              </div>

              {summary.toxic_wins > 0 ? (
                <div className="mt-3 flex items-center gap-2 rounded-sm border border-rose-500/45 bg-rose-950/35 px-3 py-2 text-xs text-rose-100">
                  <AlertTriangle className="h-4 w-4 shrink-0 text-rose-300" />
                  <span>检测到 <strong>{summary.toxic_wins}</strong> 笔“坏过程但盈利”（毒性盈利 Toxic Win），严禁把侥幸盈利当成策略有效。</span>
                </div>
              ) : null}
            </Card>
          </div>

          {/* Three Action Queues */}
          <div className="grid gap-4 lg:grid-cols-3">
            {/* Queue 1: Open Positions Risk */}
            <Card className="p-5">
              <div className="flex items-center justify-between border-b border-rule pb-3">
                <h2 className="flex items-center gap-2 text-base font-bold text-white">
                  <ShieldAlert className="h-4 w-4 text-brand-300" />
                  在途持仓风控 ({desk.open_trades.length})
                </h2>
                <Link to="/positions" className="text-xs text-slate-400 hover:text-white">全部</Link>
              </div>
              <div className="mt-3 space-y-3">
                {desk.open_trades.length === 0 ? (
                  <p className="py-6 text-center text-sm text-slate-500">当前无在途持仓。</p>
                ) : (
                  desk.open_trades.map((tr) => {
                    const draft = stopDrafts[tr.id] ?? (tr.current_stop_loss ? String(tr.current_stop_loss) : '');
                    return (
                      <div key={tr.id} className="rounded-sm border border-rule bg-slate-950/55 p-3">
                        <div className="flex items-center justify-between">
                          <Link to={`/research/${encodeURIComponent(tr.symbol)}`} className="font-data text-base font-black text-white hover:text-brand-200">
                            {displaySymbol(tr.symbol)}
                          </Link>
                          <span className="font-data text-xs text-slate-300">
                            在途风险 {tr.open_risk_amount.toLocaleString()} {tr.currency}
                          </span>
                        </div>
                        <p className="font-data mt-1 text-xs text-slate-400">
                          均价 {tr.avg_entry_price} · 持仓 {tr.open_quantity} 股 · 当前止损 {tr.current_stop_loss ?? '未设置'}
                        </p>
                        <div className="mt-2.5 flex items-center gap-2">
                          <Input
                            aria-label={`${displaySymbol(tr.symbol)} 新止损价`}
                            type="number"
                            step="any"
                            value={draft}
                            onChange={(e) => setStopDrafts((prev) => ({ ...prev, [tr.id]: e.target.value }))}
                            placeholder="新止损价"
                            className="h-8 text-xs"
                          />
                          <Button
                            variant="outline"
                            className="h-8 px-2.5 text-xs"
                            disabled={stopMutation.isPending || !Number(draft) || Number(draft) <= 0}
                            onClick={() => stopMutation.mutate({ id: tr.id, stopLoss: Number(draft) })}
                          >
                            移动止损
                          </Button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </Card>

            {/* Queue 2: Waiting Plans */}
            <Card className="p-5">
              <div className="flex items-center justify-between border-b border-rule pb-3">
                <h2 className="flex items-center gap-2 text-base font-bold text-white">
                  <TrendingUp className="h-4 w-4 text-emerald-300" />
                  待触发交易计划 ({desk.waiting_plans.length})
                </h2>
                <Link to="/positions?tab=plans" className="text-xs text-slate-400 hover:text-white">管理</Link>
              </div>
              <div className="mt-3 space-y-3">
                {desk.waiting_plans.length === 0 ? (
                  <div className="py-6 text-center">
                    <p className="text-sm text-slate-500">暂无等待触发的计划。</p>
                    <Link to="/research" className="mt-2 inline-flex items-center gap-1 text-xs font-bold text-brand-300 hover:underline">
                      去研究台挑选候选 <ArrowRight className="h-3.5 w-3.5" />
                    </Link>
                  </div>
                ) : (
                  desk.waiting_plans.map((plan) => (
                    <div key={plan.id} className="rounded-sm border border-rule bg-slate-950/55 p-3">
                      <div className="flex items-center justify-between">
                        <Link to={`/research/${encodeURIComponent(plan.ticker)}`} className="font-data text-base font-black text-white hover:text-brand-200">
                          {displaySymbol(plan.ticker)}
                        </Link>
                        <span className="font-data rounded-sm border border-brand-400/40 bg-brand-400/10 px-2 py-0.5 text-xs font-bold text-brand-200">
                          {plan.risk_multiple ? `${plan.risk_multiple}R` : plan.direction.toUpperCase()}
                        </span>
                      </div>
                      <p className="font-data mt-1 text-xs text-slate-400">
                        入场 {plan.entry_price ?? '-'} · 止损 {plan.stop_loss ?? '-'} · 股数 {plan.planned_quantity ?? '-'}
                      </p>
                      <div className="mt-2.5 flex justify-end">
                        <Link to="/positions?tab=plans">
                          <Button variant="outline" className="h-8 px-2.5 text-xs">记录成交 / 审查</Button>
                        </Link>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </Card>

            {/* Queue 3: Pending Reviews & Off-Plan Fills */}
            <Card className="p-5">
              <div className="flex items-center justify-between border-b border-rule pb-3">
                <h2 className="flex items-center gap-2 text-base font-bold text-white">
                  <CheckCircle2 className="h-4 w-4 text-amber-300" />
                  盘后待办与核查
                </h2>
                <Link to="/positions?tab=closed" className="text-xs text-slate-400 hover:text-white">结案簿</Link>
              </div>
              <div className="mt-3 space-y-3">
                {summary.unlinked_fills_count > 0 ? (
                  <div className="rounded-sm border border-amber-500/40 bg-amber-500/10 p-3 text-xs text-amber-100">
                    <p className="font-bold">发现 {summary.unlinked_fills_count} 笔计划外成交（裸奔单）</p>
                    <p className="mt-1 opacity-85">请前往头寸簿归档并关联原计划，否则系统将标记为未受控交易。</p>
                    <Link to="/positions?tab=fills" className="mt-2 inline-block font-bold underline">立即处理 →</Link>
                  </div>
                ) : null}

                {desk.pending_reviews.length === 0 && summary.unlinked_fills_count === 0 ? (
                  <p className="py-6 text-center text-sm text-slate-500">所有已结案交易均已完成复盘，纪律台账干净。</p>
                ) : (
                  desk.pending_reviews.map((tr) => (
                    <div key={tr.id} className="rounded-sm border border-rule bg-slate-950/55 p-3">
                      <div className="flex items-center justify-between">
                        <span className="font-data text-base font-black text-white">{displaySymbol(tr.symbol)}</span>
                        <span className={cn('font-data text-xs font-bold', (tr.realized_r ?? 0) >= 0 ? 'text-emerald-300' : 'text-rose-300')}>
                          {tr.realized_r !== null ? `${tr.realized_r >= 0 ? '+' : ''}${tr.realized_r.toFixed(2)}R` : `${tr.realized_pnl ?? 0}`}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-slate-400">
                        已平仓待结案复盘 · 当前象限：<span className="text-slate-200">{tr.quadrant}</span>
                      </p>
                      <div className="mt-2.5 flex justify-end">
                        <Link to="/positions?tab=closed">
                          <Button variant="outline" className="h-8 px-2.5 text-xs">15 秒快速复盘</Button>
                        </Link>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </Card>
          </div>
        </div>
      ) : null}
    </div>
  );
}
