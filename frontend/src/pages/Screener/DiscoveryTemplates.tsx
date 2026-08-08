import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Braces, ChevronDown, Save, SlidersHorizontal } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Select } from '../../components/ui/select';
import { createScreenDefinition, fetchScreenDefinitions } from '../../lib/discoveryApi';
import type { AssetMarket, Market, ThresholdConfig } from '../../types';
import { conditionsToThresholds, type ScreenCondition } from '../../types/discovery';

const labels: Record<string, string> = { quarterly_eps_growth: '当季 EPS 增长', annual_eps_growth: '年度 EPS 增长', volume_ratio: '成交量比率', relative_strength: '相对强度', institution_delta: '机构变化', avg_dollar_volume: '平均成交额', volatility: '波动率' };

export default function DiscoveryTemplates({ assetMarket, stockPool, thresholds, onApply, onUnknownRules }: { assetMarket: AssetMarket; stockPool: Market; thresholds: ThresholdConfig; onApply: (values: Partial<ThresholdConfig>) => void; onUnknownRules: (labels: string[]) => void }) {
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [includeLiquidity, setIncludeLiquidity] = useState(false);
  const [managing, setManaging] = useState(false);
  const query = useQuery({ queryKey: ['screen-definitions', assetMarket], queryFn: () => fetchScreenDefinitions(assetMarket) });
  const conditions: ScreenCondition[] = [
    { field: 'quarterly_eps_growth', operator: 'gte', value: thresholds.c_min_growth, unknown_policy: 'unknown' },
    { field: 'annual_eps_growth', operator: 'gte', value: thresholds.a_min_growth, unknown_policy: 'unknown' },
    { field: 'volume_ratio', operator: 'gte', value: thresholds.s_volume_ratio, unknown_policy: 'unknown' },
    { field: 'relative_strength', operator: 'gte', value: thresholds.l_min_rs, unknown_policy: 'unknown' },
    { field: 'institution_delta', operator: 'gte', value: thresholds.i_min_delta, unknown_policy: 'unknown' },
    ...(includeLiquidity ? [{ field: 'avg_dollar_volume', operator: 'gte', value: 20_000_000, unknown_policy: 'unknown' } as ScreenCondition] : [])
  ];
  const save = useMutation({ mutationFn: () => createScreenDefinition({ name: name.trim(), description: '从当前可见条件保存', asset_market: assetMarket, stock_pool: stockPool, conditions }), onSuccess: () => { setName(''); setManaging(false); client.invalidateQueries({ queryKey: ['screen-definitions', assetMarket] }); } });
  return <Card className="p-4 sm:p-5">
    <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex items-center gap-2"><Braces className="h-4 w-4 text-brand-200" /><h2 className="font-black text-white">条件模板</h2></div>
      <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row lg:max-w-xl">
        <Select aria-label="发现模板" defaultValue="" onValueChange={(id) => { const selected = query.data?.find((item) => item.id === id); if (selected) { onApply(conditionsToThresholds(selected.conditions)); const unknown = selected.conditions.filter((item) => item.field === 'avg_dollar_volume' || item.field === 'volatility').map((item) => labels[item.field]); setIncludeLiquidity(unknown.includes(labels.avg_dollar_volume)); onUnknownRules(unknown); } }}>
          <option value="">载入结构化模板</option>{query.data?.map((item) => <option value={item.id} key={item.id}>{item.name}{item.is_system ? ' · 系统' : ''}</option>)}
        </Select>
        <Button type="button" variant="secondary" className="shrink-0" aria-expanded={managing} onClick={() => setManaging((value) => !value)}>
          <SlidersHorizontal className="h-4 w-4" />管理模板<ChevronDown className={`h-4 w-4 transition-transform ${managing ? 'rotate-180' : ''}`} />
        </Button>
      </div>
    </div>
    {managing ? <div className="mt-4 border-t border-rule pt-4">
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input aria-label="新模板名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="新模板名称" />
        <Button className="shrink-0" disabled={!name.trim() || save.isPending} onClick={() => save.mutate()}><Save className="h-4 w-4" />保存</Button>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">{conditions.map((condition) => <span className="rounded-full border border-slate-700 px-2.5 py-1 text-xs text-slate-300" key={condition.field}>{labels[condition.field]} {condition.operator === 'gte' ? '≥' : '≤'} {condition.value}</span>)}</div>
      <label className="mt-3 flex items-center gap-2 text-xs font-semibold text-slate-300"><input type="checkbox" checked={includeLiquidity} onChange={(event) => { setIncludeLiquidity(event.target.checked); onUnknownRules(event.target.checked ? [labels.avg_dollar_volume] : []); }} />加入平均成交额 ≥ 2000 万条件</label>
    </div> : null}
  </Card>;
}
