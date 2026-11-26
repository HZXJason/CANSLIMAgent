import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, Calculator, CheckCircle2, ShieldAlert } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { createTradePlan } from '../../lib/api';
import { createCandidateSnapshot, createResearchCase, createUserThesis } from '../../lib/decisionSpineApi';
import { fetchMarketRegime } from '../../lib/deskApi';
import { calculatePositionSize } from '../../lib/positionSizerApi';
import { fetchPlaybooks } from '../../lib/profileApi';
import { cn } from '../../lib/utils';
import type { Market } from '../../types';
import type { EvidenceCardsPack } from '../../types/evidenceCards';
import type { PositionSizeResult } from '../../types/trades';
import PreTradeRiskReviewCard from '../TradePlans/PreTradeRiskReviewCard';

const RISK_TIERS = [0.25, 0.5, 1.0, 1.5] as const;

export default function QuickPlanCockpit({
  symbol,
  market,
  evidencePack
}: {
  symbol: string;
  market: Market;
  evidencePack?: EvidenceCardsPack;
}) {
  const queryClient = useQueryClient();
  const assetMarket = /\.(SH|SZ|BJ)$/i.test(symbol) ? 'CN' : 'US';

  const [selectedPlaybookId, setSelectedPlaybookId] = useState<string>('');
  const [riskMultiple, setRiskMultiple] = useState<number>(1.0);
  const [entryPrice, setEntryPrice] = useState<string>('');
  const [stopLoss, setStopLoss] = useState<string>('');
  const [takeProfit, setTakeProfit] = useState<string>('');
  const [thesisSummary, setThesisSummary] = useState<string>('');
  const [falsifier, setFalsifier] = useState<string>('');
  const [sizerResult, setSizerResult] = useState<PositionSizeResult | null>(null);
  const [sizerError, setSizerError] = useState<string>('');
  const [createdPlanId, setCreatedPlanId] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string>('');

  const playbooksQuery = useQuery({
    queryKey: ['profile-playbooks'],
    queryFn: () => fetchPlaybooks(false)
  });

  const regimeQuery = useQuery({
    queryKey: ['market-regime', assetMarket],
    queryFn: () => fetchMarketRegime(assetMarket)
  });

  const activePlaybooks = (playbooksQuery.data ?? []).filter((pb) => pb.status === 'active');

  useEffect(() => {
    setCreatedPlanId(null);
    setSizerResult(null);
    setSizerError('');
    setSubmitError('');
  }, [symbol]);

  useEffect(() => {
    const entry = Number(entryPrice);
    const stop = Number(stopLoss);
    if (!Number.isFinite(entry) || !Number.isFinite(stop) || entry <= 0 || stop <= 0 || stop >= entry) {
      setSizerResult(null);
      return;
    }
    const timer = window.setTimeout(() => {
      const tp = Number(takeProfit);
      calculatePositionSize({
        symbol,
        direction: 'buy',
        risk_multiple: riskMultiple,
        entry_price: entry,
        stop_loss: stop,
        take_profit: Number.isFinite(tp) && tp > 0 ? tp : null,
        atr_14: evidencePack?.atr_14 ?? null,
        market_regime: regimeQuery.data?.regime ?? ''
      })
        .then((res) => {
          setSizerResult(res);
          setSizerError('');
        })
        .catch((err) => {
          setSizerError(err instanceof Error ? err.message : '头寸计算失败');
        });
    }, 180);
    return () => window.clearTimeout(timer);
  }, [entryPrice, stopLoss, takeProfit, riskMultiple, symbol, evidencePack?.atr_14, regimeQuery.data?.regime]);

  const applyLatestClose = () => {
    if (evidencePack?.latest_close) {
      setEntryPrice(String(evidencePack.latest_close));
    }
  };

  const applyQuickStop = (mode: 'atr' | 'pct5') => {
    const entry = Number(entryPrice) || evidencePack?.latest_close || 0;
    if (entry <= 0) return;
    if (!entryPrice) setEntryPrice(String(entry));
    if (mode === 'atr' && evidencePack?.atr_14) {
      setStopLoss((entry - evidencePack.atr_14).toFixed(2));
    } else {
      setStopLoss((entry * 0.95).toFixed(2));
    }
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      const entry = Number(entryPrice);
      const stop = Number(stopLoss);
      if (!Number.isFinite(entry) || entry <= 0 || !Number.isFinite(stop) || stop <= 0 || stop >= entry) {
        throw new Error('请填写有效的入场价与低于入场价的止损价');
      }
      if (!thesisSummary.trim() || !falsifier.trim()) {
        throw new Error('请填写 1 句话买入理由与 1 条证伪离场条件');
      }

      const sized =
        sizerResult ??
        (await calculatePositionSize({
          symbol,
          direction: 'buy',
          risk_multiple: riskMultiple,
          entry_price: entry,
          stop_loss: stop,
          take_profit: Number(takeProfit) > 0 ? Number(takeProfit) : null,
          atr_14: evidencePack?.atr_14 ?? null,
          market_regime: regimeQuery.data?.regime ?? ''
        }));

      const candidate = await createCandidateSnapshot({
        symbol,
        market,
        screen_name: 'Research Cockpit',
        inclusion_reasons: [thesisSummary.trim()],
        exclusion_risks: [],
        context_snapshot: { data_as_of: evidencePack?.data_as_of ?? null, latest_close: evidencePack?.latest_close ?? null }
      });

      const researchCase = await createResearchCase({
        candidate_snapshot_id: candidate.id,
        symbol,
        market,
        evidence_snapshot: {
          cards: evidencePack?.cards ?? [],
          sources: evidencePack?.sources ?? []
        }
      });

      const thesis = await createUserThesis(researchCase.id, {
        stance: 'long',
        summary: thesisSummary.trim(),
        accepted_arguments: [thesisSummary.trim()],
        rejected_arguments: [],
        unresolved_questions: [],
        falsifiers: [falsifier.trim()]
      });

      const selectedPb = activePlaybooks.find((pb) => pb.id === selectedPlaybookId);
      const today = new Date().toISOString().slice(0, 10);

      const plan = await createTradePlan({
        ticker: symbol,
        direction: 'buy',
        status: 'waiting',
        plan_date: today,
        user_thesis_id: thesis.id,
        playbook_id: selectedPb ? selectedPb.id : null,
        entry_trigger: selectedPb ? `${selectedPb.name} · 触发价 ${entry}` : `触发价 ${entry}`,
        entry_price: entry,
        stop_loss: stop,
        take_profit: Number(takeProfit) > 0 ? Number(takeProfit) : null,
        position_size: Math.min(100, Number(sized.position_percent.toFixed(2))),
        planned_quantity: sized.suggested_quantity,
        risk_multiple: riskMultiple,
        initial_risk_per_share: sized.per_share_risk,
        one_r_amount: sized.one_r_amount,
        max_risk_amount: sized.actual_risk_amount,
        max_risk_percent: sized.actual_risk_percent,
        invalidation: falsifier.trim(),
        rationale: thesisSummary.trim()
      });
      return plan;
    },
    onSuccess: (plan) => {
      setCreatedPlanId(plan.id);
      setSubmitError('');
      void queryClient.invalidateQueries({ queryKey: ['trade-plans'] });
      void queryClient.invalidateQueries({ queryKey: ['desk'] });
    },
    onError: (err) => {
      setSubmitError(err instanceof Error ? err.message : '计划创建失败');
    }
  });

  const regime = regimeQuery.data;

  return (
    <div className="space-y-4">
      <Card className="p-5">
        <div className="flex items-center justify-between border-b border-rule pb-3">
          <div>
            <p className="font-data text-[11px] uppercase tracking-[0.16em] text-brand-300">45s Execution Cockpit</p>
            <h2 className="mt-0.5 text-lg font-black text-white">1R 头寸计算与快速立计划</h2>
          </div>
          {regime ? (
            <span
              className={cn(
                'font-data rounded-sm border px-2.5 py-1 text-xs font-bold',
                regime.regime === 'correction'
                  ? 'border-rose-500/50 bg-rose-500/15 text-rose-200'
                  : regime.regime === 'confirmed_uptrend'
                  ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-200'
                  : 'border-amber-500/40 bg-amber-500/10 text-amber-200'
              )}
            >
              M：{regime.regime_label} ({regime.max_exposure_percent}%)
            </span>
          ) : null}
        </div>

        <div className="mt-4 space-y-4">
          {/* 1. Playbook selector */}
          <div>
            <label className="text-xs font-bold text-slate-300">1. 选择战术剧本 (Playbook)</label>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {activePlaybooks.map((pb) => {
                const active = selectedPlaybookId === pb.id;
                return (
                  <button
                    key={pb.id}
                    type="button"
                    onClick={() => {
                      setSelectedPlaybookId(active ? '' : pb.id);
                      if (!active && pb.default_risk_multiple) {
                        setRiskMultiple(pb.default_risk_multiple);
                      }
                    }}
                    className={cn(
                      'rounded-sm border px-2.5 py-1.5 text-xs font-semibold transition-colors',
                      active
                        ? 'border-brand-300 bg-brand-300/15 text-white'
                        : 'border-rule bg-slate-950/60 text-slate-400 hover:text-slate-200'
                    )}
                  >
                    {pb.name}
                  </button>
                );
              })}
              {activePlaybooks.length === 0 ? (
                <span className="text-xs text-slate-500">暂无启用剧本（可在“交易系统”新增）</span>
              ) : null}
            </div>
          </div>

          {/* 2. 3-number inputs + Risk tier */}
          <div className="grid gap-3 sm:grid-cols-3">
            <div>
              <div className="flex items-center justify-between">
                <label htmlFor="cockpit-entry" className="text-xs font-bold text-slate-300">入场触发价</label>
                {evidencePack?.latest_close ? (
                  <button type="button" onClick={applyLatestClose} className="text-[11px] font-semibold text-brand-300 hover:underline">
                    用收盘 {evidencePack.latest_close}
                  </button>
                ) : null}
              </div>
              <Input
                id="cockpit-entry"
                type="number"
                step="any"
                value={entryPrice}
                onChange={(e) => setEntryPrice(e.target.value)}
                placeholder="如 135.00"
                className="mt-1"
              />
            </div>

            <div>
              <div className="flex items-center justify-between">
                <label htmlFor="cockpit-stop" className="text-xs font-bold text-slate-300">失效止损价</label>
                <div className="flex gap-1.5 text-[11px]">
                  {evidencePack?.atr_14 ? (
                    <button type="button" onClick={() => applyQuickStop('atr')} className="font-semibold text-brand-300 hover:underline">
                      -1 ATR
                    </button>
                  ) : null}
                  <button type="button" onClick={() => applyQuickStop('pct5')} className="font-semibold text-slate-400 hover:underline">
                    -5%
                  </button>
                </div>
              </div>
              <Input
                id="cockpit-stop"
                type="number"
                step="any"
                value={stopLoss}
                onChange={(e) => setStopLoss(e.target.value)}
                placeholder="如 128.00"
                className="mt-1"
              />
            </div>

            <div>
              <label htmlFor="cockpit-target" className="text-xs font-bold text-slate-300">参考目标价 (选填)</label>
              <Input
                id="cockpit-target"
                type="number"
                step="any"
                value={takeProfit}
                onChange={(e) => setTakeProfit(e.target.value)}
                placeholder="如 155.00"
                className="mt-1"
              />
            </div>
          </div>

          <div>
            <span className="text-xs font-bold text-slate-300">2. 风险预算档位 (基于账户 1R)</span>
            <div className="mt-1.5 grid grid-cols-4 gap-2">
              {RISK_TIERS.map((tier) => (
                <button
                  key={tier}
                  type="button"
                  onClick={() => setRiskMultiple(tier)}
                  className={cn(
                    'font-data min-h-9 rounded-sm border text-xs font-bold transition-colors',
                    riskMultiple === tier
                      ? 'border-brand-300 bg-brand-300 text-paper-ink'
                      : 'border-rule bg-slate-950/60 text-slate-300 hover:border-slate-600'
                  )}
                >
                  {tier}R
                </button>
              ))}
            </div>
          </div>

          {/* Live Sizing Result */}
          {sizerResult ? (
            <div className="rounded-sm border border-brand-400/30 bg-slate-950/75 p-3.5">
              <div className="flex items-center justify-between text-xs text-slate-400">
                <span className="flex items-center gap-1.5 font-bold text-brand-200">
                  <Calculator className="h-3.5 w-3.5" />建议头寸计算结果
                </span>
                <span className="font-data">最小交易单位 {sizerResult.min_lot_size} 股</span>
              </div>
              <div className="font-data mt-2.5 grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
                <div>
                  <span className="block text-slate-500">建议股数</span>
                  <span className="text-base font-black text-white">{sizerResult.suggested_quantity} 股</span>
                </div>
                <div>
                  <span className="block text-slate-500">仓位占比</span>
                  <span className="text-base font-black text-white">{sizerResult.position_percent}%</span>
                </div>
                <div>
                  <span className="block text-slate-500">实际风险</span>
                  <span className="font-bold text-slate-200">{sizerResult.actual_risk_amount} ({sizerResult.actual_risk_r}R)</span>
                </div>
                <div>
                  <span className="block text-slate-500">叠加 Heat</span>
                  <span className="font-bold text-slate-200">{sizerResult.projected_heat_percent}%</span>
                </div>
              </div>
              {sizerResult.warnings.length > 0 ? (
                <div className="mt-2.5 space-y-1.5 border-t border-rule pt-2.5">
                  {sizerResult.warnings.map((w) => (
                    <div
                      key={w.code}
                      className={cn(
                        'flex items-start gap-1.5 text-xs',
                        w.severity === 'high' ? 'text-rose-300' : 'text-amber-300'
                      )}
                    >
                      <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                      <span>{w.message}</span>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : sizerError ? (
            <p className="text-xs text-rose-300">{sizerError}</p>
          ) : null}

          {/* 3. 2-sentence Thesis + Falsifier */}
          <div className="space-y-3">
            <div>
              <label htmlFor="cockpit-thesis" className="text-xs font-bold text-slate-300">3. 买入唯一核心理由（1 句话）</label>
              <Input
                id="cockpit-thesis"
                value={thesisSummary}
                onChange={(e) => setThesisSummary(e.target.value)}
                placeholder="例如：Q3 EPS +45% 加速叠加量比 2.1x 突破 6 周箱体"
                className="mt-1"
              />
            </div>
            <div>
              <label htmlFor="cockpit-falsifier" className="text-xs font-bold text-slate-300">4. 证伪离场条件（Falsifier，必填）</label>
              <Input
                id="cockpit-falsifier"
                value={falsifier}
                onChange={(e) => setFalsifier(e.target.value)}
                placeholder="例如：放量跌破枢轴低点或下季毛利率回落即认错离场"
                className="mt-1"
              />
            </div>
          </div>

          {submitError ? (
            <div className="rounded-sm border border-rose-500/40 bg-rose-950/35 px-3 py-2 text-xs text-rose-200">
              {submitError}
            </div>
          ) : null}

          <Button
            className="w-full"
            disabled={saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            <ShieldAlert className="h-4 w-4" />
            {saveMutation.isPending ? '保存并锁定论点快照...' : '保存观点与计划并执行红队审查'}
          </Button>
        </div>
      </Card>

      {createdPlanId ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2 rounded-sm border border-emerald-500/35 bg-emerald-500/10 px-4 py-2.5 text-xs font-semibold text-emerald-100">
            <CheckCircle2 className="h-4 w-4 text-emerald-300" />
            <span>已锁定不可变证据快照、论点与计划（状态：等待触发）。可在下方运行下单前红队风险审查：</span>
          </div>
          <PreTradeRiskReviewCard planId={createdPlanId} />
        </div>
      ) : null}
    </div>
  );
}
