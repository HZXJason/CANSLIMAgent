import { ArrowLeft, BadgeCheck, CalendarDays, Sparkles } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Tooltip } from '../../components/ui/tooltip';
import { CANSLIM_LABEL_KEYS, computeStockLabels, labelStatusText } from '../../lib/computeLabels';
import { formatDateCN } from '../../lib/utils';
import { useScreenerStore } from '../../stores/screenerStore';
import type { LabelKey, Market, StockWithMetrics } from '../../types';
import type { StockSummary } from '../../types/analysis';

const tagOrder: LabelKey[] = CANSLIM_LABEL_KEYS;

const marketLabel: Record<string, string> = {
  SP500: '标普 500',
  NASDAQ100: '纳指 100',
  RUSSELL2000: '罗素 2000'
};

interface StockHeaderProps {
  symbol: string;
  tabName: string;
  summary: StockSummary | null;
}

export default function StockHeader({ symbol, tabName, summary }: StockHeaderProps) {
  const navigate = useNavigate();
  const thresholds = useScreenerStore((state) => state.thresholds);
  const stockForLabels: StockWithMetrics | null = summary
    ? {
        symbol: summary.symbol,
        name: summary.name,
        market: summary.market as Market,
        metrics: summary.metrics,
		ai_analyzed: summary.ai_analyzed,
        data_as_of: summary.data_as_of
      }
    : null;
  const labels = stockForLabels ? computeStockLabels(stockForLabels, thresholds) : null;

  return (
    <section className="sticky top-3 z-20 rounded-2xl border border-slate-800/90 bg-slate-950/90 px-3 py-3 shadow-xl shadow-slate-950/30 backdrop-blur sm:px-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="ghost" className="h-9 w-9 shrink-0 rounded-xl px-0" onClick={() => navigate('/screener')} aria-label="返回选股器">
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <h1 className="text-xl font-black tracking-tight text-white sm:text-2xl">{symbol}</h1>
              <span className="max-w-72 truncate text-sm font-semibold text-slate-300 sm:text-base">{summary?.name ?? tabName}</span>
              {summary?.metrics.n ? (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-300/30 bg-emerald-400/10 px-2.5 py-1 text-xs font-bold text-emerald-200">
                  <BadgeCheck className="h-3.5 w-3.5" />
                  N
                </span>
              ) : null}
              {summary?.ai_analyzed ? (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-violet-300/30 bg-violet-400/10 px-2.5 py-1 text-xs font-bold text-violet-200">
                  <Sparkles className="h-3.5 w-3.5" />
                  AI 已分析
                </span>
              ) : null}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs font-medium text-slate-500">
              {summary?.market ? <span>{marketLabel[summary.market] ?? summary.market}</span> : null}
              {summary?.data_as_of ? (
                <span className="inline-flex items-center gap-1">
                  <CalendarDays className="h-3.5 w-3.5" />
                  {formatDateCN(summary.data_as_of)}
                </span>
              ) : null}
            </div>
          </div>
        </div>

        {labels ? <div className="flex flex-wrap gap-1.5 lg:justify-end">
          {tagOrder.map((tag) => {
            const label = labels?.[tag];
            const status = label?.status ?? 'insufficient';
            const value = Array.isArray(label?.value) ? label.value.map((item) => item.toFixed(1) + '%').join(' / ') : String(label?.value ?? '暂无');
            const sourceKey = tag === 'S' ? 's' : tag === 'L' ? 'l' : tag === 'I' ? 'i' : null;
            const source = sourceKey ? summary?.metric_sources?.[sourceKey] : undefined;
            const sourceText = source ? '，数据源 ' + source.source + (source.status === 'unavailable' ? '（暂不可用）' : '') : '';
            return (
              <Tooltip key={tag} content={tag + '：' + labelStatusText(status) + '，原始值 ' + value + sourceText}>
                <Badge variant={status === 'pass' ? 'pass' : status === 'fail' ? 'fail' : 'insufficient'} aria-label={tag + ' ' + labelStatusText(status)}>
                  {status === 'insufficient' ? '?' : tag}
                </Badge>
              </Tooltip>
            );
          })}
        </div> : null}
      </div>
    </section>
  );
}
