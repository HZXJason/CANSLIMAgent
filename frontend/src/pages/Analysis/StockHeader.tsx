import { ArrowLeft, BadgeCheck, CalendarDays, Sparkles } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Tooltip } from '../../components/ui/tooltip';
import { CANSLIM_LABEL_KEYS, computeStockLabels, labelStatusText } from '../../lib/computeLabels';
import { SCREENER_MARKETS, assetMarketForScreenerMarket, displaySymbol, screenerMarketLabel } from '../../lib/markets';
import { formatDateCN } from '../../lib/utils';
import { useScreenerStore } from '../../stores/screenerStore';
import type { AssetMarket, LabelKey, Market, StockWithMetrics } from '../../types';
import type { StockSummary } from '../../types/analysis';

const tagOrder: LabelKey[] = CANSLIM_LABEL_KEYS;

interface StockHeaderProps {
  symbol: string;
  tabName: string;
  summary: StockSummary | null;
  assetMarket: AssetMarket;
}

export default function StockHeader({ symbol, tabName, summary, assetMarket }: StockHeaderProps) {
  const navigate = useNavigate();
  const visibleSymbol = displaySymbol(symbol);
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
  const summaryMarket = SCREENER_MARKETS[assetMarket].some((market) => market.value === summary?.market)
    ? summary?.market as Market
    : null;
  const marketLabel = summaryMarket && assetMarketForScreenerMarket(summaryMarket) === assetMarket
    ? screenerMarketLabel(summaryMarket)
    : assetMarket === 'CN' ? 'A股市场' : '美股市场';
  const hasMetadata = Boolean(marketLabel || summary?.data_as_of || summary?.metrics.n || summary?.ai_analyzed);

  return (
    <section
      className="glass-chrome sticky top-[calc(4rem+0.5rem)] z-30 rounded-[18px] border border-white/[0.09] px-3 py-2.5 shadow-[0_18px_46px_rgba(0,0,0,0.28)] md:top-3 sm:px-4 sm:py-3"
      aria-label={`${visibleSymbol} 股票摘要`}
    >
      <div className="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:justify-between lg:gap-4">
        <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
          <Button
            type="button"
            variant="ghost"
            className="h-11 w-11 shrink-0 px-0 sm:h-10 sm:w-10"
            onClick={() => navigate('/screener')}
            aria-label="返回选股器"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-baseline gap-2.5">
              <h1 className="shrink-0 text-xl font-extrabold tracking-[-0.025em] text-white sm:text-2xl">{visibleSymbol}</h1>
              <span className="min-w-0 truncate text-sm font-semibold text-slate-300 sm:max-w-80 sm:text-base">{summary?.name ?? tabName}</span>
            </div>
            {hasMetadata ? (
              <div className="mt-1.5 flex min-w-0 items-center gap-2 overflow-x-auto whitespace-nowrap pb-0.5 text-xs font-medium text-slate-400">
                <span className="shrink-0">{marketLabel}</span>
                {summary?.data_as_of ? (
                  <span className="inline-flex shrink-0 items-center gap-1">
                    <CalendarDays className="h-3.5 w-3.5" />
                    {formatDateCN(summary.data_as_of)}
                  </span>
                ) : null}
                {summary?.metrics.n ? (
                  <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-emerald-300/25 bg-emerald-400/10 px-2 py-0.5 font-bold text-emerald-200">
                    <BadgeCheck className="h-3.5 w-3.5" />
                    N
                  </span>
                ) : null}
                {summary?.ai_analyzed ? (
                  <span className="inline-flex shrink-0 items-center gap-1 rounded-full border border-violet-300/25 bg-violet-400/10 px-2 py-0.5 font-bold text-violet-200">
                    <Sparkles className="h-3.5 w-3.5" />
                    AI 已分析
                  </span>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>

        {labels ? (
          <div className="flex flex-wrap gap-1.5 pl-[3.375rem] sm:pl-[3.625rem] lg:shrink-0 lg:justify-end lg:pl-0">
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
          </div>
        ) : null}
      </div>
    </section>
  );
}
