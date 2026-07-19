import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { BadgeCheck, BarChart3, ChevronRight, SearchX, Sparkles } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Tooltip } from '../../components/ui/tooltip';
import { SCORE_LABEL_KEYS, SCREENER_FILTER_KEYS, labelStatusText } from '../../lib/computeLabels';
import { assetMarketForScreenerMarket, displaySymbol, normalizeSymbolForAsset, screenerMarketLabel } from '../../lib/markets';
import { hasGeneratedAIAnalysis, useAnalysisStore } from '../../stores/analysisStore';
import { useTabStore } from '../../stores/tabStore';
import type { ComputedStock, LabelResult, ScreenerFilterKey } from '../../types';

const tagOrder: ScreenerFilterKey[] = SCREENER_FILTER_KEYS;
const scoreDenominator = SCORE_LABEL_KEYS.length;
const INITIAL_VISIBLE_STOCKS = 160;
const VISIBLE_STOCK_INCREMENT = 160;

interface StockListProps {
  stocks: ComputedStock[];
  hasRawData: boolean;
  filtered: boolean;
}

function formatValue(label: LabelResult) {
  if (Array.isArray(label.value)) {
    return label.value.map((value) => value.toFixed(1) + '%').join(' / ');
  }
  return String(label.value);
}

function LabelBadge({ label }: { label: LabelResult }) {
  if (label.status === 'insufficient') {
    return (
      <Tooltip content="数据不足，谨慎参考，基准期以 0 计算">
        <Badge variant="insufficient" aria-label={`${label.key} 数据不足`}>?</Badge>
      </Tooltip>
    );
  }
  return (
    <Tooltip content={label.key + '：' + labelStatusText(label.status) + '，原始值 ' + formatValue(label)}>
      <Badge variant={label.status === 'pass' ? 'pass' : 'fail'} aria-label={`${label.key} ${labelStatusText(label.status)}`}>{label.key}</Badge>
    </Tooltip>
  );
}

function NLabelBadge({ written }: { written: boolean | null }) {
  if (!written) {
    return (
      <Tooltip content="N：AI 分析尚未写回 New 标签">
        <Badge variant="neutral" aria-label="N 尚未由 AI 写回">N</Badge>
      </Tooltip>
    );
  }

  return (
    <Tooltip content="N：AI 分析已写回的 New 标签">
      <Badge variant="pass" aria-label="N 已由 AI 写回">
        <span className="inline-flex items-center gap-1">
          <BadgeCheck className="h-3 w-3" />
          N
        </span>
      </Badge>
    </Tooltip>
  );
}

function AIAnalysisBadge() {
  return (
    <Tooltip content="已生成 AI 分析报告">
      <Badge variant="neutral" aria-label="已生成 AI 分析报告" className="h-6 min-w-9 shrink-0 gap-1 border-brand-400/50 bg-brand-400/10 px-1.5 text-[11px] text-brand-200">
        <Sparkles className="h-3 w-3" />
        AI
      </Badge>
    </Tooltip>
  );
}

function EmptyState({ filtered }: { filtered: boolean }) {
  return (
    <Card className="flex flex-col items-center justify-center gap-3 py-14 text-center">
      <SearchX className="h-10 w-10 text-slate-600" />
      <p className="text-base font-medium text-slate-300">{filtered ? '没有符合当前标签的股票' : '当前市场暂无数据'}</p>
    </Card>
  );
}

function FilterBadge({ stock, tag }: { stock: ComputedStock; tag: ScreenerFilterKey }) {
  if (tag === 'N') return <NLabelBadge written={stock.metrics.n} />;
  return <LabelBadge label={stock.labels[tag]} />;
}

function stockIdentity(stock: ComputedStock) {
  return stock.market + ':' + stock.symbol;
}

function canonicalStockSymbol(stock: ComputedStock) {
  return normalizeSymbolForAsset(assetMarketForScreenerMarket(stock.market), stock.symbol);
}

function stockHasAIAnalysis(stock: ComputedStock, aiAnalyzedSymbols: Set<string>) {
	return stock.ai_analyzed || aiAnalyzedSymbols.has(canonicalStockSymbol(stock));
}

interface StockRowProps {
  stock: ComputedStock;
  hasAIAnalysis: boolean;
  onOpen: (stock: ComputedStock) => void;
}

const StockRow = memo(function StockRow({ stock, hasAIAnalysis, onOpen }: StockRowProps) {
  const passedTags = tagOrder.filter((tag) => tag === 'N' ? Boolean(stock.metrics.n) : stock.labels[tag].status === 'pass');
  const rowSummary = `${stock.passCount}/${scoreDenominator} 项计分标签达标，通过 ${passedTags.length > 0 ? passedTags.join('、') : '暂无'}，权重 ${stock.weightSum}`;

  return (
    <button
      type="button"
      className="group grid w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2.5 px-4 py-4 text-left transition-[background-color,box-shadow] duration-150 ease-out hover:bg-slate-800/55 active:bg-slate-800/75 sm:px-5 md:grid-cols-[0.9fr_1.4fr_2fr_0.9fr_0.7fr] md:gap-4"
      onClick={() => onOpen(stock)}
      aria-label={`打开 ${displaySymbol(stock.symbol)} ${stock.name}，${rowSummary}`}
    >
      <span className="flex min-w-0 items-center gap-2 font-semibold text-white">
        <BarChart3 className="h-4 w-4 text-brand-300/70" />
        <span className="truncate">{displaySymbol(stock.symbol)}</span>
        {hasAIAnalysis ? <AIAnalysisBadge /> : null}
      </span>
      <span className="col-span-2 row-start-2 flex min-w-0 items-center gap-2 truncate text-sm text-slate-400 md:col-span-1 md:row-auto md:block md:text-base md:text-slate-300">
        <span className="truncate">{stock.name}</span>
        <span className="shrink-0 text-slate-600 md:hidden">· {screenerMarketLabel(stock.market, true)}</span>
      </span>
      <span className="col-span-2 row-start-3 flex flex-wrap gap-1.5 md:col-span-1 md:row-auto md:gap-2">
        {tagOrder.map((tag) => (
          <FilterBadge key={tag} stock={stock} tag={tag} />
        ))}
      </span>
      <span className="hidden text-sm text-slate-400 md:block">{screenerMarketLabel(stock.market, true)}</span>
      <span className="col-start-2 row-start-1 inline-flex items-center justify-end gap-1.5 text-right text-sm font-bold tabular-nums text-slate-200 md:col-auto md:row-auto">
        <span>{stock.passCount}/{scoreDenominator} · {stock.weightSum}</span>
        <ChevronRight className="h-4 w-4 text-slate-600 transition-transform duration-150 group-hover:translate-x-0.5 group-hover:text-slate-400 md:hidden" />
      </span>
    </button>
  );
});

export default function StockList({ stocks, hasRawData, filtered }: StockListProps) {
  const navigate = useNavigate();
  const openTab = useTabStore((state) => state.openTab);
  const analysisStates = useAnalysisStore((state) => state.analysisStates);
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_STOCKS);

  useEffect(() => {
    setVisibleCount(INITIAL_VISIBLE_STOCKS);
  }, [stocks]);

  const visibleStocks = useMemo(() => stocks.slice(0, visibleCount), [stocks, visibleCount]);
  const aiAnalyzedSymbols = useMemo(
    () =>
      new Set(
        Object.entries(analysisStates)
          .filter(([, state]) => hasGeneratedAIAnalysis(state))
          .map(([symbol]) => symbol)
      ),
    [analysisStates]
  );
  const hiddenCount = stocks.length - visibleStocks.length;
  const handleOpenStock = useCallback(
    (stock: ComputedStock) => {
      const canonicalSymbol = canonicalStockSymbol(stock);
      if (!canonicalSymbol) return;
      openTab(canonicalSymbol, stock.name);
      navigate('/analysis/' + encodeURIComponent(canonicalSymbol), { state: { name: stock.name } });
    },
    [navigate, openTab]
  );

  if (!hasRawData || stocks.length === 0) {
    return <EmptyState filtered={filtered && hasRawData} />;
  }

  return (
    <Card className="overflow-hidden p-0">
      <div className="flex items-center justify-between border-b border-slate-800/80 px-4 py-4 sm:px-5">
        <div>
          <h2 className="text-base font-bold text-white">候选股票</h2>
          <p className="mt-1 text-xs text-slate-500">按综合权重排序，点击进入证据与图表</p>
        </div>
        <span className="rounded-full border border-slate-800 bg-slate-950/35 px-3 py-1.5 text-sm font-bold tabular-nums text-slate-300">{visibleStocks.length} / {stocks.length}</span>
      </div>
      <div className="hidden grid-cols-[0.9fr_1.4fr_2fr_0.9fr_0.7fr] gap-4 border-b border-slate-800 px-5 py-2.5 text-xs font-semibold text-slate-500 md:grid">
        <span>代码</span>
        <span>名称</span>
        <span>标签</span>
        <span>市场</span>
        <span className="text-right">得分</span>
      </div>
      <div className="divide-y divide-slate-900">
        {visibleStocks.map((stock) => (
          <StockRow
            key={stockIdentity(stock)}
            stock={stock}
            hasAIAnalysis={stockHasAIAnalysis(stock, aiAnalyzedSymbols)}
            onOpen={handleOpenStock}
          />
        ))}
      </div>
      {hiddenCount > 0 ? (
        <div className="border-t border-slate-800 px-5 py-4 text-center">
          <Button
            variant="outline"
            onClick={() => setVisibleCount((count) => Math.min(count + VISIBLE_STOCK_INCREMENT, stocks.length))}
          >
            再显示 {Math.min(VISIBLE_STOCK_INCREMENT, hiddenCount)} 支
          </Button>
        </div>
      ) : null}
    </Card>
  );
}
