import { ArrowLeft, BadgeCheck, CalendarDays } from 'lucide-react';
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
        data_as_of: summary.data_as_of
      }
    : null;
  const labels = stockForLabels ? computeStockLabels(stockForLabels, thresholds) : null;

  return (
    <section className="sticky top-0 z-30 rounded-[1.25rem] border border-slate-800/90 bg-slate-950/85 p-4 shadow-2xl shadow-slate-950/40 backdrop-blur">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center">
          <Button variant="outline" className="h-10 w-10 shrink-0 rounded-xl px-0" onClick={() => navigate('/screener')} aria-label="返回选股器">
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-2xl font-black text-white sm:text-3xl">{symbol}</h1>
              <span className="truncate text-lg font-semibold text-slate-300">{summary?.name ?? tabName}</span>
              {summary?.metrics.n ? (
                <span className="inline-flex items-center gap-1 rounded-full bg-emerald-400 px-3 py-1 text-xs font-bold text-slate-950">
                  <BadgeCheck className="h-3.5 w-3.5" />
                  N
                </span>
              ) : null}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-slate-400">
              <span>{marketLabel[summary?.market ?? ''] ?? '市场待识别'}</span>
              <span className="inline-flex items-center gap-1">
                <CalendarDays className="h-4 w-4 text-cyan-300" />
                数据截止 {formatDateCN(summary?.data_as_of ?? null)}
              </span>
            </div>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          {tagOrder.map((tag) => {
            const label = labels?.[tag];
            const status = label?.status ?? 'insufficient';
            const value = Array.isArray(label?.value) ? label.value.map((item) => item.toFixed(1) + '%').join(' / ') : String(label?.value ?? '暂无');
            return (
              <Tooltip key={tag} content={tag + '：' + labelStatusText(status) + '，原始值 ' + value}>
                <Badge variant={status === 'pass' ? 'pass' : status === 'fail' ? 'fail' : 'insufficient'}>{status === 'insufficient' ? '?' : tag}</Badge>
              </Tooltip>
            );
          })}
        </div>
      </div>
    </section>
  );
}
