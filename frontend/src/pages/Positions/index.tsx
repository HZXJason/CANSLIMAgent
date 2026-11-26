import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, CheckCircle2, ClipboardList, Download, Plus, ShieldAlert, TrendingUp, XCircle } from 'lucide-react';
import { Link, useSearchParams } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import BrokerJournalImport from '../../components/journal/BrokerJournalImport';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Select } from '../../components/ui/select';
import { createJournalEntry, executeTradePlan, fetchBrokerConnections, fetchJournalEntries, fetchTradePlans, updateTradePlan } from '../../lib/api';
import { displaySymbol } from '../../lib/markets';
import { exportJournalRecords } from '../../lib/profileApi';
import { linkTradeToPlan, fetchTrades, updateTradeStopLoss, upsertTradeReview } from '../../lib/tradesApi';
import { cn } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import type { TradePlan } from '../../types/tradePlan';
import type { EnrichedTrade, TradeQuadrant } from '../../types/trades';
import PreTradeRiskReviewCard from '../TradePlans/PreTradeRiskReviewCard';

type PositionsTab = 'open' | 'plans' | 'closed' | 'fills';

const TABS: Array<{ id: PositionsTab; label: string }> = [
  { id: 'open', label: '在途持仓' },
  { id: 'plans', label: '等待计划' },
  { id: 'closed', label: '已结案与四象限复盘' },
  { id: 'fills', label: '原始成交与券商导入' }
];

const QUADRANT_META: Record<TradeQuadrant, { label: string; style: string }> = {
  open: { label: '持仓中', style: 'border-slate-700 bg-slate-900 text-slate-200' },
  model_win: { label: '标准盈利 Model Win', style: 'border-emerald-400/50 bg-emerald-500/15 text-emerald-200' },
  good_loss: { label: '合规止损 Good Loss', style: 'border-emerald-500/40 bg-emerald-950/30 text-emerald-300' },
  unforced_error: { label: '违纪亏损 Unforced Error', style: 'border-rose-500/50 bg-rose-500/15 text-rose-200' },
  toxic_win: { label: '毒性盈利 Toxic Win', style: 'border-rose-500 bg-rose-600/25 text-rose-100' }
};

const VIOLATION_LABELS: Record<string, string> = {
  off_plan_unlinked: '计划外裸奔单',
  missing_stop_loss: '未设止损',
  entry_chase_over_0_25r: '追高 >0.25R',
  stop_loss_violated: '扛单超损 <-1.15R'
};

function isValidTab(value: string | null): value is PositionsTab {
  return value === 'open' || value === 'plans' || value === 'closed' || value === 'fills';
}

function matchesAssetMarket(tickerOrMarket: string, assetMarket: 'US' | 'CN', isTradeMarketField = false): boolean {
  if (isTradeMarketField) {
    return assetMarket === 'CN' ? tickerOrMarket === 'china_a' : tickerOrMarket === 'us';
  }
  const isCN = /\.(SH|SZ|BJ)$/i.test(tickerOrMarket);
  return assetMarket === 'CN' ? isCN : !isCN;
}

export default function PositionsPage() {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get('tab');
  const activeTab: PositionsTab = isValidTab(rawTab) ? rawTab : 'open';

  const [notice, setNotice] = useState('');
  const [errorMsg, setErrorMsg] = useState('');
  const [stopDrafts, setStopDrafts] = useState<Record<string, string>>({});
  const [closeTradeTarget, setCloseTradeTarget] = useState<EnrichedTrade | null>(null);
  const [closeQty, setCloseQty] = useState('');
  const [closePrice, setClosePrice] = useState('');
  const [closeFee, setCloseFee] = useState('0');

  const [executePlanTarget, setExecutePlanTarget] = useState<TradePlan | null>(null);
  const [execQty, setExecQty] = useState('');
  const [execPrice, setExecPrice] = useState('');
  const [execFee, setExecFee] = useState('0');
  const [reviewPlanId, setReviewPlanId] = useState<string | null>(null);

  const [reviewingTradeId, setReviewingTradeId] = useState<string | null>(null);
  const [reviewQuality, setReviewQuality] = useState<'followed' | 'adjusted' | 'deviated'>('followed');
  const [reviewLesson, setReviewLesson] = useState('');

  const [linkTradeId, setLinkTradeId] = useState<string | null>(null);
  const [selectedPlanToLink, setSelectedPlanToLink] = useState('');

  const openTradesQuery = useQuery({
    queryKey: ['trades', 'open', assetMarket],
    queryFn: () => fetchTrades({ status: 'open', market: assetMarket })
  });
  const closedTradesQuery = useQuery({
    queryKey: ['trades', 'closed', assetMarket],
    queryFn: () => fetchTrades({ status: 'closed', market: assetMarket })
  });
  const plansQuery = useQuery({
    queryKey: ['trade-plans'],
    queryFn: () => fetchTradePlans({})
  });
  const fillsQuery = useQuery({
    queryKey: ['journal-entries'],
    queryFn: fetchJournalEntries
  });
  const brokersQuery = useQuery({
    queryKey: ['broker-connections'],
    queryFn: fetchBrokerConnections
  });

  const invalidateAll = () => {
    void queryClient.invalidateQueries({ queryKey: ['trades'] });
    void queryClient.invalidateQueries({ queryKey: ['trade-plans'] });
    void queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
    void queryClient.invalidateQueries({ queryKey: ['desk'] });
  };

  const stopMutation = useMutation({
    mutationFn: ({ id, stopLoss }: { id: string; stopLoss: number }) => updateTradeStopLoss(id, stopLoss),
    onSuccess: (tr) => {
      setNotice(`已更新 ${displaySymbol(tr.symbol)} 止损至 ${tr.current_stop_loss}`);
      setErrorMsg('');
      invalidateAll();
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '止损更新失败')
  });

  const closePositionMutation = useMutation({
    mutationFn: async () => {
      if (!closeTradeTarget) return;
      const qty = Number(closeQty);
      const price = Number(closePrice);
      if (!Number.isFinite(qty) || qty <= 0 || !Number.isFinite(price) || price <= 0) {
        throw new Error('请输入有效的平仓股数和成交价');
      }
      return createJournalEntry({
        datetime: new Date().toISOString(),
        symbol: closeTradeTarget.symbol,
        name: closeTradeTarget.name || closeTradeTarget.symbol,
        side: closeTradeTarget.direction === 'short' ? 'buy' : 'sell',
        quantity: qty,
        price,
        fee: Number(closeFee) || 0,
        market: assetMarket === 'CN' ? 'china_a' : 'us',
        currency: closeTradeTarget.currency || (assetMarket === 'CN' ? 'CNY' : 'USD')
      });
    },
    onSuccess: () => {
      setNotice('已记录平仓/减仓成交并重算 Realized R');
      setCloseTradeTarget(null);
      setErrorMsg('');
      invalidateAll();
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '记录平仓失败')
  });

  const executePlanMutation = useMutation({
    mutationFn: async () => {
      if (!executePlanTarget) return;
      const qty = Number(execQty);
      const price = Number(execPrice);
      if (!Number.isFinite(qty) || qty <= 0 || !Number.isFinite(price) || price <= 0) {
        throw new Error('请输入有效的成交数量与价格');
      }
      return executeTradePlan(executePlanTarget.id, {
        datetime: new Date().toISOString(),
        quantity: qty,
        price,
        fee: Number(execFee) || 0,
        market: assetMarket === 'CN' ? 'china_a' : 'us',
        currency: assetMarket === 'CN' ? 'CNY' : 'USD'
      });
    },
    onSuccess: () => {
      setNotice('计划已执行并生成在途仓位');
      setExecutePlanTarget(null);
      setErrorMsg('');
      invalidateAll();
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '计划执行失败')
  });

  const cancelPlanMutation = useMutation({
    mutationFn: (id: string) => updateTradePlan(id, { status: 'cancelled' }),
    onSuccess: () => {
      setNotice('计划已取消');
      invalidateAll();
    }
  });

  const reviewMutation = useMutation({
    mutationFn: ({ tradeId }: { tradeId: string }) =>
      upsertTradeReview(tradeId, {
        process_quality: reviewQuality,
        lesson: reviewLesson.trim() || null
      }),
    onSuccess: () => {
      setNotice('结案复盘已保存，四象限指标已更新');
      setReviewingTradeId(null);
      setReviewLesson('');
      setErrorMsg('');
      invalidateAll();
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '复盘保存失败')
  });

  const linkPlanMutation = useMutation({
    mutationFn: ({ tradeId, planId }: { tradeId: string; planId: string }) => linkTradeToPlan(tradeId, planId),
    onSuccess: () => {
      setNotice('已补关联交易计划并重算仓位');
      setLinkTradeId(null);
      setSelectedPlanToLink('');
      invalidateAll();
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '关联计划失败')
  });

  const handleExportJournal = async () => {
    try {
      const payload = await exportJournalRecords();
      const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `alphalens-trade-journal-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
      setNotice('交易日志已导出（vibe.trade_record.v1）');
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : '导出失败');
    }
  };

  const openTrades = openTradesQuery.data ?? [];
  const closedTrades = closedTradesQuery.data ?? [];
  const waitingPlans = (plansQuery.data ?? []).filter(
    (p) => (p.status === 'waiting' || p.status === 'draft') && matchesAssetMarket(p.ticker, assetMarket)
  );
  const marketFills = (fillsQuery.data ?? []).filter((f) => matchesAssetMarket(f.market, assetMarket, true));
  const allPlansForMarket = (plansQuery.data ?? []).filter((p) => matchesAssetMarket(p.ticker, assetMarket));

  return (
    <div className="mx-auto w-full max-w-[1380px] px-4 py-6 sm:px-6 lg:py-8">
      <PageHeader
        title="头寸簿"
        meta={`${openTrades.length} 个在途 · ${waitingPlans.length} 个待触发计划 · ${closedTrades.length} 笔已结案`}
        actions={
          <div className="flex flex-wrap gap-2">
            <Link to="/research">
              <Button variant="outline"><Plus className="h-4 w-4" />去研究台开计划</Button>
            </Link>
            <Link to="/journal/new">
              <Button variant="outline">手工记一笔成交</Button>
            </Link>
          </div>
        }
      />

      <div role="tablist" aria-label="头寸簿视图" className="mt-5 grid grid-cols-2 border-y border-rule sm:grid-cols-4">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            role="tab"
            type="button"
            aria-selected={activeTab === tab.id}
            onClick={() => setSearchParams({ tab: tab.id })}
            className={cn(
              'min-h-11 border-b-2 px-3 text-xs font-bold transition-colors sm:text-sm',
              activeTab === tab.id ? 'border-brand-300 text-white' : 'border-transparent text-slate-500 hover:text-slate-200'
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {notice ? (
        <div role="status" className="mt-4 flex items-center justify-between border border-emerald-500/35 bg-emerald-500/10 px-4 py-2 text-sm text-emerald-100">
          <span>{notice}</span>
          <button type="button" className="text-xs underline" onClick={() => setNotice('')}>关闭</button>
        </div>
      ) : null}

      {errorMsg ? (
        <div role="alert" className="mt-4 flex items-center justify-between border border-rose-500/40 bg-rose-950/35 px-4 py-2 text-sm text-rose-100">
          <span>{errorMsg}</span>
          <button type="button" className="text-xs underline" onClick={() => setErrorMsg('')}>关闭</button>
        </div>
      ) : null}

      {/* Tab 1: Open Trades */}
      {activeTab === 'open' ? (
        <div className="mt-6 space-y-4">
          {openTrades.length === 0 ? (
            <Card className="p-8 text-center text-sm text-slate-400">当前市场无在途持仓。</Card>
          ) : (
            openTrades.map((tr) => {
              const draft = stopDrafts[tr.id] ?? (tr.current_stop_loss ? String(tr.current_stop_loss) : '');
              return (
                <Card key={tr.id} className="p-5">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <div className="flex flex-wrap items-center gap-2.5">
                        <Link to={`/research/${encodeURIComponent(tr.symbol)}`} className="font-data text-xl font-black text-white hover:text-brand-200">
                          {displaySymbol(tr.symbol)}
                        </Link>
                        <span className="rounded-sm border border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-xs font-bold text-emerald-200">
                          在途 {tr.open_quantity} 股
                        </span>
                        {tr.auto_violations.map((v) => (
                          <span key={v} className="rounded-sm border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-200">
                            {VIOLATION_LABELS[v] ?? v}
                          </span>
                        ))}
                      </div>
                      <p className="font-data mt-2 text-xs text-slate-400">
                        开仓均价 {tr.avg_entry_price} · 当前止损 {tr.current_stop_loss ?? '未设'} · 在途风险{' '}
                        <strong className="text-white">{tr.open_risk_amount.toLocaleString()} {tr.currency}</strong>
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <Input
                        aria-label={`${displaySymbol(tr.symbol)} 止损价格`}
                        type="number"
                        step="any"
                        value={draft}
                        onChange={(e) => setStopDrafts((prev) => ({ ...prev, [tr.id]: e.target.value }))}
                        className="h-9 w-28 text-xs"
                        placeholder="新止损"
                      />
                      <Button
                        variant="outline"
                        className="h-8 px-2.5 text-xs"
                        disabled={stopMutation.isPending || !Number(draft)}
                        onClick={() => stopMutation.mutate({ id: tr.id, stopLoss: Number(draft) })}
                      >
                        移动止损
                      </Button>
                      <Button
                        className="h-8 px-2.5 text-xs"
                        onClick={() => {
                          setCloseTradeTarget(tr);
                          setCloseQty(String(tr.open_quantity));
                          setClosePrice(String(tr.avg_entry_price));
                        }}
                      >
                        记录平仓/减仓
                      </Button>
                    </div>
                  </div>
                </Card>
              );
            })
          )}

          {closeTradeTarget ? (
            <Card className="border-brand-300/50 p-5">
              <h3 className="text-base font-bold text-white">记录 {displaySymbol(closeTradeTarget.symbol)} 卖出平仓</h3>
              <div className="mt-3 grid gap-3 sm:grid-cols-3">
                <Input aria-label="平仓股数" type="number" step="any" value={closeQty} onChange={(e) => setCloseQty(e.target.value)} placeholder="股数" />
                <Input aria-label="平仓成交价" type="number" step="any" value={closePrice} onChange={(e) => setClosePrice(e.target.value)} placeholder="成交价" />
                <Input aria-label="手续费" type="number" step="any" value={closeFee} onChange={(e) => setCloseFee(e.target.value)} placeholder="手续费" />
              </div>
              <div className="mt-3 flex justify-end gap-2">
                <Button variant="outline" onClick={() => setCloseTradeTarget(null)}>取消</Button>
                <Button disabled={closePositionMutation.isPending} onClick={() => closePositionMutation.mutate()}>确认记录平仓</Button>
              </div>
            </Card>
          ) : null}
        </div>
      ) : null}

      {/* Tab 2: Waiting Plans */}
      {activeTab === 'plans' ? (
        <div className="mt-6 space-y-4">
          {waitingPlans.length === 0 ? (
            <Card className="p-8 text-center text-sm text-slate-400">
              当前市场暂无待触发计划。请前往 <Link to="/research" className="font-bold text-brand-300 underline">研究台</Link> 使用 45 秒开单舱新建计划。
            </Card>
          ) : (
            waitingPlans.map((plan) => (
              <Card key={plan.id} className="p-5">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div>
                    <div className="flex items-center gap-2.5">
                      <Link to={`/research/${encodeURIComponent(plan.ticker)}`} className="font-data text-xl font-black text-white hover:text-brand-200">
                        {displaySymbol(plan.ticker)}
                      </Link>
                      <span className="font-data rounded-sm border border-brand-400/40 bg-brand-400/10 px-2 py-0.5 text-xs font-bold text-brand-200">
                        {plan.risk_multiple ? `${plan.risk_multiple}R` : plan.direction.toUpperCase()}
                      </span>
                      <span className="text-xs text-slate-400">状态：{plan.status}</span>
                    </div>
                    <p className="font-data mt-2 text-xs text-slate-300">
                      入场 {plan.entry_price ?? '-'} · 止损 {plan.stop_loss ?? '-'} · 计划股数 {plan.planned_quantity ?? '-'} · 失效条件：{plan.invalidation || '未写明'}
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      className="h-8 px-2.5 text-xs"
                      onClick={() => setReviewPlanId(reviewPlanId === plan.id ? null : plan.id)}
                    >
                      <ShieldAlert className="h-3.5 w-3.5" />红队审查
                    </Button>
                    <Button
                      className="h-8 px-2.5 text-xs"
                      onClick={() => {
                        setExecutePlanTarget(plan);
                        setExecQty(plan.planned_quantity ? String(plan.planned_quantity) : '');
                        setExecPrice(plan.entry_price ? String(plan.entry_price) : '');
                      }}
                    >
                      <TrendingUp className="h-3.5 w-3.5" />按计划记录成交
                    </Button>
                    <Button
                      variant="outline"
                      className="h-8 px-2.5 text-xs"
                      onClick={() => cancelPlanMutation.mutate(plan.id)}
                    >
                      <XCircle className="h-3.5 w-3.5" />取消
                    </Button>
                  </div>
                </div>
                {reviewPlanId === plan.id ? (
                  <div className="mt-4 border-t border-rule pt-4">
                    <PreTradeRiskReviewCard planId={plan.id} />
                  </div>
                ) : null}
              </Card>
            ))
          )}

          {executePlanTarget ? (
            <Card className="border-brand-300/50 p-5">
              <h3 className="text-base font-bold text-white">执行计划：{displaySymbol(executePlanTarget.ticker)}</h3>
              <div className="mt-3 grid gap-3 sm:grid-cols-3">
                <Input aria-label="实际成交股数" type="number" step="any" value={execQty} onChange={(e) => setExecQty(e.target.value)} placeholder="实际成交股数" />
                <Input aria-label="实际成交价格" type="number" step="any" value={execPrice} onChange={(e) => setExecPrice(e.target.value)} placeholder="实际成交价" />
                <Input aria-label="手续费" type="number" step="any" value={execFee} onChange={(e) => setExecFee(e.target.value)} placeholder="手续费" />
              </div>
              <div className="mt-3 flex justify-end gap-2">
                <Button variant="outline" onClick={() => setExecutePlanTarget(null)}>取消</Button>
                <Button disabled={executePlanMutation.isPending} onClick={() => executePlanMutation.mutate()}>确认成交并建仓</Button>
              </div>
            </Card>
          ) : null}
        </div>
      ) : null}

      {/* Tab 3: Closed Trades & 4-Quadrant Review */}
      {activeTab === 'closed' ? (
        <div className="mt-6 space-y-4">
          {closedTrades.length === 0 ? (
            <Card className="p-8 text-center text-sm text-slate-400">当前市场尚无已平仓结案交易。</Card>
          ) : (
            closedTrades.map((tr) => {
              const qMeta = QUADRANT_META[tr.quadrant];
              const isReviewed = Boolean(tr.trade_review?.completed_at);
              return (
                <Card key={tr.id} className="p-5">
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div>
                      <div className="flex flex-wrap items-center gap-2.5">
                        <span className="font-data text-xl font-black text-white">{displaySymbol(tr.symbol)}</span>
                        {tr.realized_r !== null ? (
                          <span
                            className={cn(
                              'font-data rounded-sm border px-2.5 py-0.5 text-xs font-black',
                              tr.realized_r >= 0
                                ? 'border-emerald-400/50 bg-emerald-500/15 text-emerald-200'
                                : 'border-rose-400/50 bg-rose-500/15 text-rose-200'
                            )}
                          >
                            {tr.realized_r >= 0 ? '+' : ''}{tr.realized_r.toFixed(2)}R
                          </span>
                        ) : null}
                        <span className={cn('rounded-sm border px-2.5 py-0.5 text-xs font-bold', qMeta.style)}>
                          {qMeta.label}
                        </span>
                        {tr.auto_violations.map((v) => (
                          <span key={v} className="inline-flex items-center gap-1 rounded-sm border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-200">
                            <AlertTriangle className="h-3 w-3" />
                            {VIOLATION_LABELS[v] ?? v}
                          </span>
                        ))}
                      </div>
                      <p className="font-data mt-2 text-xs text-slate-400">
                        买入均价 {tr.avg_entry_price} → 卖出均价 {tr.avg_exit_price ?? '-'} · 净盈亏{' '}
                        <strong className={cn((tr.realized_pnl ?? 0) >= 0 ? 'text-emerald-300' : 'text-rose-300')}>
                          {tr.realized_pnl !== null ? `${tr.realized_pnl >= 0 ? '+' : ''}${tr.realized_pnl}` : '-'} {tr.currency}
                        </strong>
                        {tr.holding_days !== null ? ` · 持有 ${tr.holding_days} 天` : ''}
                      </p>
                      {tr.trade_review?.lesson ? (
                        <p className="mt-2 text-xs text-slate-300">复盘结论：{tr.trade_review.lesson}</p>
                      ) : null}
                    </div>

                    <div className="flex flex-wrap gap-2">
                      {!tr.trade_plan_id ? (
                        <Button
                          variant="outline"
                          className="h-8 px-2.5 text-xs"
                          onClick={() => setLinkTradeId(linkTradeId === tr.id ? null : tr.id)}
                        >
                          <ClipboardList className="h-3.5 w-3.5" />补关联计划
                        </Button>
                      ) : null}
                      <Button
                        variant={isReviewed ? 'outline' : 'primary'}
                        className="h-8 px-2.5 text-xs"
                        onClick={() => {
                          setReviewingTradeId(reviewingTradeId === tr.id ? null : tr.id);
                          setReviewQuality((tr.trade_review?.process_quality as 'followed' | 'adjusted' | 'deviated') || 'followed');
                          setReviewLesson(tr.trade_review?.lesson || '');
                        }}
                      >
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        {isReviewed ? '修改复盘' : '15秒复盘'}
                      </Button>
                    </div>
                  </div>

                  {linkTradeId === tr.id ? (
                    <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-rule pt-3">
                      <Select aria-label="选择原交易计划" value={selectedPlanToLink} onValueChange={setSelectedPlanToLink}>
                        <option value="">选择匹配计划...</option>
                        {allPlansForMarket
                          .filter((p) => p.ticker.toUpperCase() === tr.symbol.toUpperCase())
                          .map((p) => (
                            <option key={p.id} value={p.id}>
                              {p.plan_date} · {p.entry_price ?? '-'} / 止损 {p.stop_loss ?? '-'}
                            </option>
                          ))}
                      </Select>
                      <Button
                        className="h-8 px-2.5 text-xs"
                        disabled={!selectedPlanToLink || linkPlanMutation.isPending}
                        onClick={() => linkPlanMutation.mutate({ tradeId: tr.id, planId: selectedPlanToLink })}
                      >
                        确认关联
                      </Button>
                    </div>
                  ) : null}

                  {reviewingTradeId === tr.id ? (
                    <div className="mt-4 space-y-3 border-t border-rule pt-4">
                      <div className="flex flex-wrap gap-2">
                        {([
                          ['followed', '严格按计划执行'],
                          ['adjusted', '盘中合理微调'],
                          ['deviated', '偏离计划违纪']
                        ] as const).map(([val, label]) => (
                          <button
                            key={val}
                            type="button"
                            onClick={() => setReviewQuality(val)}
                            className={cn(
                              'rounded-sm border px-3 py-1.5 text-xs font-bold',
                              reviewQuality === val ? 'border-brand-300 bg-brand-300 text-paper-ink' : 'border-rule bg-slate-950/60 text-slate-300'
                            )}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
                      <Input
                        aria-label="一句话复盘教训"
                        value={reviewLesson}
                        onChange={(e) => setReviewLesson(e.target.value)}
                        placeholder="写下一条下次可执行的教训或确认做得对的地方"
                      />
                      <div className="flex justify-end gap-2">
                        <Button variant="outline" className="h-8 px-2.5 text-xs" onClick={() => setReviewingTradeId(null)}>收起</Button>
                        <Button className="h-8 px-2.5 text-xs" disabled={reviewMutation.isPending} onClick={() => reviewMutation.mutate({ tradeId: tr.id })}>保存复盘</Button>
                      </div>
                    </div>
                  ) : null}
                </Card>
              );
            })
          )}
        </div>
      ) : null}

      {/* Tab 4: Raw Fills & Broker Import */}
      {activeTab === 'fills' ? (
        <div className="mt-6 space-y-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-slate-400">券商只读同步仅导入原始成交（Fill），系统自动按 FIFO 聚合进完整仓位并检验是否关联计划。</p>
            <Button variant="outline" className="h-8 px-2.5 text-xs" onClick={handleExportJournal}>
              <Download className="h-3.5 w-3.5" />导出 vibe.trade_record.v1
            </Button>
          </div>
          <BrokerJournalImport connections={brokersQuery.data ?? []} onImported={invalidateAll} />
          <Card className="overflow-hidden p-0">
            <div className="border-b border-rule px-5 py-3 text-xs font-bold text-slate-400">
              原始成交流水 ({marketFills.length})
            </div>
            <div className="divide-y divide-slate-900">
              {marketFills.map((fill) => (
                <Link
                  key={fill.id}
                  to={`/journal/${fill.id}`}
                  className="flex flex-wrap items-center justify-between gap-3 px-5 py-3.5 text-xs hover:bg-slate-800/45"
                >
                  <div className="flex items-center gap-3">
                    <span className={cn('font-data rounded-sm px-2 py-0.5 font-bold', fill.side === 'buy' ? 'bg-emerald-500/20 text-emerald-200' : 'bg-rose-500/20 text-rose-200')}>
                      {fill.side === 'buy' ? '买' : '卖'}
                    </span>
                    <span className="font-data font-bold text-white">{displaySymbol(fill.symbol)}</span>
                    <span className="font-data text-slate-400">{fill.quantity} × {fill.price}</span>
                  </div>
                  <div className="flex items-center gap-3">
                    {!fill.trade_plan_id ? (
                      <span className="rounded-sm border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-[11px] font-semibold text-amber-200">
                        未关联计划
                      </span>
                    ) : (
                      <span className="text-[11px] text-emerald-300">已关联计划</span>
                    )}
                    <span className="font-data text-slate-500">{new Date(fill.datetime).toLocaleString()}</span>
                  </div>
                </Link>
              ))}
              {marketFills.length === 0 ? (
                <p className="p-6 text-center text-sm text-slate-500">当前市场暂无原始成交记录。</p>
              ) : null}
            </div>
          </Card>
        </div>
      ) : null}
    </div>
  );
}
