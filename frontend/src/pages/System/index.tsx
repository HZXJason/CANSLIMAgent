import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AlertTriangle, CheckCircle2, Download, Plus, ShieldCheck } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Select } from '../../components/ui/select';
import {
  createPlaybook,
  createPolicy,
  exportJournalRecords,
  exportTradingSystem,
  fetchPlaybooks,
  fetchPolicies,
  fetchTradingProfile,
  patchPlaybook,
  patchPolicy,
  saveTradingProfile
} from '../../lib/profileApi';
import { fetchBehaviorObservations, updateBehaviorObservationFeedback } from '../../lib/tradeReviewApi';
import { fetchTradesSummary } from '../../lib/tradesApi';
import { cn } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import SettingsPage from '../Settings';

type SystemTab = 'scorecard' | 'risk' | 'playbooks' | 'connections';

const TABS: Array<{ id: SystemTab; label: string }> = [
  { id: 'scorecard', label: '四象限行为账单' },
  { id: 'risk', label: '账户净值与 1R 底线' },
  { id: 'playbooks', label: '剧本与纪律库' },
  { id: 'connections', label: '连接与数据导出' }
];

function isValidTab(v: string | null): v is SystemTab {
  return v === 'scorecard' || v === 'risk' || v === 'playbooks' || v === 'connections';
}

export default function SystemPage() {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const rawTab = searchParams.get('tab');
  const activeTab: SystemTab = isValidTab(rawTab) ? rawTab : 'scorecard';

  const [notice, setNotice] = useState('');
  const [errorMsg, setErrorMsg] = useState('');

  // Risk & 1R profile form
  const [equity, setEquity] = useState('');
  const [currency, setCurrency] = useState('USD');
  const [stdRiskPct, setStdRiskPct] = useState('1.0');
  const [maxHeatPct, setMaxHeatPct] = useState('5.0');
  const [maxPosPct, setMaxPosPct] = useState('20.0');
  const [maxDailyLoss, setMaxDailyLoss] = useState('3.0');
  const [maxPositions, setMaxPositions] = useState('6');

  // New playbook & policy form
  const [pbName, setPbName] = useState('');
  const [pbRiskMult, setPbRiskMult] = useState('1.0');
  const [pbStopRules, setPbStopRules] = useState('');
  const [policyTitle, setPolicyTitle] = useState('');
  const [policyStatement, setPolicyStatement] = useState('');

  const summaryQuery = useQuery({
    queryKey: ['trades-summary', assetMarket],
    queryFn: () => fetchTradesSummary(assetMarket)
  });
  const observationsQuery = useQuery({
    queryKey: ['behavior-observations'],
    queryFn: fetchBehaviorObservations
  });
  const profileQuery = useQuery({
    queryKey: ['trading-profile'],
    queryFn: fetchTradingProfile
  });
  const playbooksQuery = useQuery({
    queryKey: ['profile-playbooks'],
    queryFn: () => fetchPlaybooks(false)
  });
  const policiesQuery = useQuery({
    queryKey: ['profile-policies'],
    queryFn: fetchPolicies
  });

  useEffect(() => {
    const p = profileQuery.data;
    if (!p) return;
    if (p.account_equity != null) setEquity(String(p.account_equity));
    if (p.account_currency) setCurrency(p.account_currency);
    if (p.standard_risk_percent != null) setStdRiskPct(String(p.standard_risk_percent));
    if (p.max_portfolio_heat_percent != null) setMaxHeatPct(String(p.max_portfolio_heat_percent));
    if (p.max_portfolio_concentration_percent != null) setMaxPosPct(String(p.max_portfolio_concentration_percent));
    if (p.max_daily_loss_percent != null) setMaxDailyLoss(String(p.max_daily_loss_percent));
    if (p.max_open_positions != null) setMaxPositions(String(p.max_open_positions));
  }, [profileQuery.data]);

  const saveProfileMutation = useMutation({
    mutationFn: () => {
      const existing = profileQuery.data;
      return saveTradingProfile({
        primary_markets: existing?.primary_markets?.length ? existing.primary_markets : [assetMarket],
        trading_horizons: existing?.trading_horizons?.length ? existing.trading_horizons : ['swing'],
        account_equity: Number(equity) > 0 ? Number(equity) : null,
        account_currency: currency,
        standard_risk_percent: Number(stdRiskPct) > 0 ? Number(stdRiskPct) : 1.0,
        max_portfolio_heat_percent: Number(maxHeatPct) > 0 ? Number(maxHeatPct) : 5.0,
        max_portfolio_concentration_percent: Number(maxPosPct) > 0 ? Number(maxPosPct) : 20.0,
        max_risk_per_trade_percent: Number(stdRiskPct) > 0 ? Number(stdRiskPct) : 1.0,
        max_daily_loss_percent: Number(maxDailyLoss) > 0 ? Number(maxDailyLoss) : null,
        max_open_positions: Number(maxPositions) > 0 ? Number(maxPositions) : null
      });
    },
    onSuccess: () => {
      setNotice('账户净值与 1R 风控底线已保存');
      setErrorMsg('');
      void queryClient.invalidateQueries({ queryKey: ['trading-profile'] });
      void queryClient.invalidateQueries({ queryKey: ['desk'] });
      void queryClient.invalidateQueries({ queryKey: ['trades-summary'] });
    },
    onError: (err) => setErrorMsg(err instanceof Error ? err.message : '保存失败')
  });

  const createPlaybookMutation = useMutation({
    mutationFn: () =>
      createPlaybook({
        name: pbName.trim(),
        markets: [assetMarket],
        horizons: ['swing'],
        stop_rules: pbStopRules.trim() || null,
        default_risk_multiple: Number(pbRiskMult) || 1.0,
        status: 'active',
        ai_enabled: true
      }),
    onSuccess: () => {
      setNotice('新战术剧本已启用');
      setPbName('');
      setPbStopRules('');
      void queryClient.invalidateQueries({ queryKey: ['profile-playbooks'] });
    }
  });

  const archivePlaybookMutation = useMutation({
    mutationFn: (id: string) => patchPlaybook(id, { status: 'archived' }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['profile-playbooks'] })
  });

  const createPolicyMutation = useMutation({
    mutationFn: () =>
      createPolicy({
        kind: 'discipline',
        category: 'risk',
        title: policyTitle.trim(),
        statement: policyStatement.trim(),
        strength: 'hard',
        origin: 'manual',
        status: 'active',
        prompt_enabled: true
      }),
    onSuccess: () => {
      setNotice('新交易纪律已激活');
      setPolicyTitle('');
      setPolicyStatement('');
      void queryClient.invalidateQueries({ queryKey: ['profile-policies'] });
    }
  });

  const approvePolicyMutation = useMutation({
    mutationFn: (id: string) => patchPolicy(id, { status: 'active', prompt_enabled: true }),
    onSuccess: () => {
      setNotice('AI 建议纪律已审批激活');
      void queryClient.invalidateQueries({ queryKey: ['profile-policies'] });
    }
  });

  const feedbackMutation = useMutation({
    mutationFn: ({ id, feedback }: { id: string; feedback: 'accepted' | 'not_applicable' | 'needs_more_evidence' }) =>
      updateBehaviorObservationFeedback(id, feedback),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['behavior-observations'] })
  });

  const downloadJSON = async (fn: () => Promise<Record<string, unknown>>, filename: string) => {
    try {
      const data = await fn();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
      setNotice(`已导出 ${filename}`);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : '导出失败');
    }
  };

  const summary = summaryQuery.data;

  return (
    <div className="mx-auto w-full max-w-[1380px] px-4 py-6 sm:px-6 lg:py-8">
      <PageHeader title="交易系统" />

      <div role="tablist" aria-label="交易系统模块" className="mt-5 grid grid-cols-2 border-y border-rule sm:grid-cols-4">
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

      {/* Tab 1: 4-Quadrant Scorecard */}
      {activeTab === 'scorecard' ? (
        <div className="mt-6 space-y-6">
          {summary ? (
            <>
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
                <Card className="p-4">
                  <span className="text-xs text-slate-400">合规风险闭环率</span>
                  <p className="font-data mt-1 text-2xl font-black text-white">{summary.disciplined_r_loop_rate.toFixed(1)}%</p>
                </Card>
                <Card className="p-4">
                  <span className="text-xs text-slate-400">单笔期望值 (Expectancy)</span>
                  <p className={cn('font-data mt-1 text-2xl font-black', summary.expectancy_r >= 0 ? 'text-emerald-300' : 'text-rose-300')}>
                    {summary.expectancy_r >= 0 ? '+' : ''}{summary.expectancy_r.toFixed(2)}R
                  </p>
                </Card>
                <Card className="p-4">
                  <span className="text-xs text-slate-400">平均赢 / 平均亏</span>
                  <p className="font-data mt-1 text-lg font-black text-white">+{summary.avg_win_r.toFixed(2)}R / {summary.avg_loss_r.toFixed(2)}R</p>
                </Card>
                <Card className="p-4">
                  <span className="text-xs text-slate-400">违纪损耗 (Indiscipline)</span>
                  <p className="font-data mt-1 text-2xl font-black text-amber-300">-{summary.cost_of_indiscipline_r.toFixed(2)}R</p>
                </Card>
                <Card className="p-4">
                  <span className="text-xs text-slate-400">已结案样本数</span>
                  <p className="font-data mt-1 text-2xl font-black text-white">{summary.closed_trades} 笔</p>
                </Card>
              </div>

              {/* 2x2 Process x Outcome Matrix */}
              <Card className="p-5">
                <h2 className="text-base font-bold text-white">过程 × 结果四象限矩阵（打破盈亏成败论）</h2>
                <div className="mt-4 grid gap-4 md:grid-cols-2">
                  <div className="rounded-sm border border-emerald-400/40 bg-emerald-950/20 p-4">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-black text-emerald-200">好过程 + 盈利 · 标准盈利 (Model Win)</span>
                      <span className="font-data text-2xl font-black text-white">{summary.model_wins}</span>
                    </div>
                    <p className="mt-1.5 text-xs text-slate-300">严格按剧本入场、审查并兑现预期盈亏比，可沉淀为标准案例。</p>
                  </div>

                  <div className="rounded-sm border border-emerald-500/35 bg-slate-950/60 p-4">
                    <div className="flex items-center justify-between">
                      <span className="flex items-center gap-1.5 text-sm font-black text-emerald-300">
                        <CheckCircle2 className="h-4 w-4" />好过程 + 亏损 · 合规止损 (Good Loss)
                      </span>
                      <span className="font-data text-2xl font-black text-white">{summary.good_losses}</span>
                    </div>
                    <p className="mt-1.5 text-xs text-slate-300">正常交易生意成本！在 1R 内果断离场应获正向肯定，严禁因此篡改系统。</p>
                  </div>

                  <div className="rounded-sm border border-rose-500/40 bg-rose-950/20 p-4">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-black text-rose-200">坏过程 + 亏损 · 违纪失误 (Unforced Error)</span>
                      <span className="font-data text-2xl font-black text-white">{summary.unforced_errors}</span>
                    </div>
                    <p className="mt-1.5 text-xs text-slate-300">追高超 0.25R、计划外裸奔或扛单超损导致的可避免亏损。</p>
                  </div>

                  <div className="rounded-sm border border-rose-500 bg-rose-950/35 p-4">
                    <div className="flex items-center justify-between">
                      <span className="flex items-center gap-1.5 text-sm font-black text-rose-100">
                        <AlertTriangle className="h-4 w-4 text-rose-300" />坏过程 + 盈利 · 毒性盈利 (Toxic Win)
                      </span>
                      <span className="font-data text-2xl font-black text-rose-200">{summary.toxic_wins}</span>
                    </div>
                    <p className="mt-1.5 text-xs text-rose-100/90">最高优先级红牌警告：靠扛单或计划外乱买赚到的钱，最容易诱导未来大亏。</p>
                  </div>
                </div>
              </Card>
            </>
          ) : null}

          <Card className="p-5">
            <h2 className="text-base font-bold text-white">样本门控行为观察（≥ 10 笔完整闭环生成）</h2>
            <div className="mt-3 space-y-3">
              {(observationsQuery.data ?? []).map((obs) => (
                <div key={obs.id || obs.key} className="rounded-sm border border-rule bg-slate-950/55 p-3.5 text-xs">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-bold text-white">{obs.message}</span>
                    <span className="font-data text-slate-400">样本 {obs.sample_count} · 置信度 {(obs.confidence * 100).toFixed(0)}%</span>
                  </div>
                  {obs.status === 'eligible' && obs.id ? (
                    <div className="mt-2.5 flex gap-2">
                      <Button variant="outline" className="h-8 px-2.5 text-xs" onClick={() => feedbackMutation.mutate({ id: obs.id, feedback: 'accepted' })}>确认属实</Button>
                      <Button variant="outline" className="h-8 px-2.5 text-xs" onClick={() => feedbackMutation.mutate({ id: obs.id, feedback: 'not_applicable' })}>当前不适用</Button>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </Card>
        </div>
      ) : null}

      {/* Tab 2: Account Equity & 1R Risk Floor */}
      {activeTab === 'risk' ? (
        <Card className="mt-6 max-w-2xl p-6">
          <h2 className="flex items-center gap-2 text-lg font-bold text-white">
            <ShieldCheck className="h-5 w-5 text-brand-300" />账户净值与 1R 风控底线
          </h2>
          <p className="mt-1 text-xs text-slate-400">研究台的 45 秒头寸计算器与作战室 Portfolio Heat 均直接读取此处的实时参数。</p>

          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="sys-equity" className="text-xs font-bold text-slate-300">账户总净值 (Account Equity)</label>
              <Input id="sys-equity" type="number" step="any" value={equity} onChange={(e) => setEquity(e.target.value)} placeholder="100000" className="mt-1" />
            </div>
            <div>
              <label htmlFor="sys-currency" className="text-xs font-bold text-slate-300">基准币种</label>
              <Select id="sys-currency" value={currency} onValueChange={setCurrency} className="mt-1">
                <option value="USD">USD 美元</option>
                <option value="CNY">CNY 人民币</option>
              </Select>
            </div>
            <div>
              <label htmlFor="sys-1r" className="text-xs font-bold text-slate-300">标准单笔风险 1R (%)</label>
              <Input id="sys-1r" type="number" step="0.1" value={stdRiskPct} onChange={(e) => setStdRiskPct(e.target.value)} className="mt-1" />
            </div>
            <div>
              <label htmlFor="sys-heat" className="text-xs font-bold text-slate-300">最大在途总敞口 Heat (%)</label>
              <Input id="sys-heat" type="number" step="0.5" value={maxHeatPct} onChange={(e) => setMaxHeatPct(e.target.value)} className="mt-1" />
            </div>
            <div>
              <label htmlFor="sys-conc" className="text-xs font-bold text-slate-300">单票最大仓位上限 (%)</label>
              <Input id="sys-conc" type="number" step="1" value={maxPosPct} onChange={(e) => setMaxPosPct(e.target.value)} className="mt-1" />
            </div>
            <div>
              <label htmlFor="sys-daily-loss" className="text-xs font-bold text-slate-300">单日最大回撤停手线 (%)</label>
              <Input id="sys-daily-loss" type="number" step="0.5" value={maxDailyLoss} onChange={(e) => setMaxDailyLoss(e.target.value)} className="mt-1" />
            </div>
            <div>
              <label htmlFor="sys-max-pos" className="text-xs font-bold text-slate-300">同时最大持仓家数</label>
              <Input id="sys-max-pos" type="number" step="1" value={maxPositions} onChange={(e) => setMaxPositions(e.target.value)} className="mt-1" />
            </div>
          </div>

          <div className="mt-6 flex justify-end">
            <Button disabled={saveProfileMutation.isPending} onClick={() => saveProfileMutation.mutate()}>
              {saveProfileMutation.isPending ? '保存中...' : '保存风控参数'}
            </Button>
          </div>
        </Card>
      ) : null}

      {/* Tab 3: Playbooks & Policies */}
      {activeTab === 'playbooks' ? (
        <div className="mt-6 grid gap-6 lg:grid-cols-2">
          <Card className="p-5">
            <h2 className="text-base font-bold text-white">战术剧本库 (Playbooks)</h2>
            <div className="mt-3 space-y-2.5">
              {(playbooksQuery.data ?? []).map((pb) => (
                <div key={pb.id} className="flex items-start justify-between gap-3 rounded-sm border border-rule bg-slate-950/55 p-3">
                  <div>
                    <p className="text-sm font-bold text-white">{pb.name}</p>
                    <p className="mt-1 text-xs text-slate-400">
                      默认 {pb.default_risk_multiple ?? 1}R · {pb.stop_rules || '遵循 1 ATR / 逻辑失效止损'}
                    </p>
                  </div>
                  <Button variant="outline" className="h-8 px-2.5 text-xs" onClick={() => archivePlaybookMutation.mutate(pb.id)}>归档</Button>
                </div>
              ))}
            </div>
            <div className="mt-4 space-y-2 border-t border-rule pt-4">
              <Input value={pbName} onChange={(e) => setPbName(e.target.value)} placeholder="新剧本名称（如：紧凑旗舰突破）" />
              <div className="grid grid-cols-2 gap-2">
                <Input type="number" step="0.25" value={pbRiskMult} onChange={(e) => setPbRiskMult(e.target.value)} placeholder="默认 R 倍数" />
                <Input value={pbStopRules} onChange={(e) => setPbStopRules(e.target.value)} placeholder="止损规则说明" />
              </div>
              <Button className="h-9 px-3 text-xs" disabled={!pbName.trim() || createPlaybookMutation.isPending} onClick={() => createPlaybookMutation.mutate()}>
                <Plus className="h-3.5 w-3.5" />新增剧本
              </Button>
            </div>
          </Card>

          <Card className="p-5">
            <h2 className="text-base font-bold text-white">纪律与红线规则 (Policies)</h2>
            <div className="mt-3 space-y-2.5">
              {(policiesQuery.data ?? []).map((pol) => (
                <div key={pol.id} className="flex items-start justify-between gap-3 rounded-sm border border-rule bg-slate-950/55 p-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-bold text-white">{pol.title}</span>
                      <span className={cn('rounded-sm px-1.5 py-0.5 text-[10px] font-bold', pol.status === 'active' ? 'bg-emerald-500/20 text-emerald-200' : 'bg-amber-500/20 text-amber-200')}>
                        {pol.status}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-slate-400">{pol.statement}</p>
                  </div>
                  {pol.status === 'draft' ? (
                    <Button className="h-8 px-2.5 text-xs" onClick={() => approvePolicyMutation.mutate(pol.id)}>审批激活</Button>
                  ) : null}
                </div>
              ))}
            </div>
            <div className="mt-4 space-y-2 border-t border-rule pt-4">
              <Input value={policyTitle} onChange={(e) => setPolicyTitle(e.target.value)} placeholder="纪律标题（如：连续两笔止损停手）" />
              <Input value={policyStatement} onChange={(e) => setPolicyStatement(e.target.value)} placeholder="具体执行声明" />
              <Button className="h-9 px-3 text-xs" disabled={!policyTitle.trim() || !policyStatement.trim() || createPolicyMutation.isPending} onClick={() => createPolicyMutation.mutate()}>
                <Plus className="h-3.5 w-3.5" />新增纪律
              </Button>
            </div>
          </Card>
        </div>
      ) : null}

      {/* Tab 4: Connections & Data Export */}
      {activeTab === 'connections' ? (
        <div className="mt-6 space-y-6">
          <Card className="flex flex-wrap items-center justify-between gap-3 p-5">
            <div>
              <h2 className="text-base font-bold text-white">个人交易数据归档导出</h2>
              <p className="mt-1 text-xs text-slate-400">本地优先架构：随时将完整交易系统配置与成交流水导出为标准 JSON。</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                className="h-9 px-3 text-xs"
                onClick={() => downloadJSON(exportTradingSystem, `alphalens-system-${new Date().toISOString().slice(0, 10)}.json`)}
              >
                <Download className="h-3.5 w-3.5" />导出交易系统 (v2)
              </Button>
              <Button
                variant="outline"
                className="h-9 px-3 text-xs"
                onClick={() => downloadJSON(exportJournalRecords, `alphalens-journal-${new Date().toISOString().slice(0, 10)}.json`)}
              >
                <Download className="h-3.5 w-3.5" />导出成交流水
              </Button>
            </div>
          </Card>
          <SettingsPage />
        </div>
      ) : null}
    </div>
  );
}
