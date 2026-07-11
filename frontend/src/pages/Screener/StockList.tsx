import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { BadgeCheck, BarChart3, SearchX, Sparkles } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Tooltip } from '../../components/ui/tooltip';
import { SCORE_LABEL_KEYS, SCREENER_FILTER_KEYS, labelStatusText } from '../../lib/computeLabels';
import { cn } from '../../lib/utils';
import { hasGeneratedAIAnalysis, useAnalysisStore } from '../../stores/analysisStore';
import { useTabStore } from '../../stores/tabStore';
import type { ComputedStock, LabelResult, Market, ScreenerFilterKey } from '../../types';

const tagOrder: ScreenerFilterKey[] = SCREENER_FILTER_KEYS;
const scoreDenominator = SCORE_LABEL_KEYS.length;
const INITIAL_VISIBLE_STOCKS = 160;
const VISIBLE_STOCK_INCREMENT = 160;
const ROW_ANIMATION_LIMIT = 24;

const marketLabel: Record<Market, string> = {
  SP500: '标普500',
  NASDAQ100: '纳指100',
  RUSSELL2000: '罗素2000'
};

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
        <Badge variant="insufficient">?</Badge>
      </Tooltip>
    );
  }
  return (
    <Tooltip content={label.key + '：' + labelStatusText(label.status) + '，原始值 ' + formatValue(label)}>
      <Badge variant={label.status === 'pass' ? 'pass' : 'fail'}>{label.key}</Badge>
    </Tooltip>
  );
}

function NLabelBadge({ written }: { written: boolean | null }) {
  if (!written) {
    return (
      <Tooltip content="N：AI 分析尚未写回 New 标签">
        <Badge variant="neutral">N</Badge>
      </Tooltip>
    );
  }

  return (
    <Tooltip content="N：AI 分析已写回的 New 标签">
      <Badge variant="pass">
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
      <Badge variant="neutral" className="h-6 min-w-9 shrink-0 gap-1 border-cyan-400/50 bg-cyan-400/10 px-1.5 text-[11px] text-cyan-200">
        <Sparkles className="h-3 w-3" />
        AI
      </Badge>
    </Tooltip>
  );
}

function EmptyState({ filtered }: { filtered: boolean }) {
  return (
    <Card className="flex flex-col items-center justify-center gap-3 py-16 text-center">
      <SearchX className="h-12 w-12 text-slate-600" />
      <p className="text-lg font-medium text-slate-200">
        {filtered ? '没有同时满足所选条件的股票，可尝试减少选中标签' : '当前市场暂无符合条件的股票'}
      </p>
      <p className="max-w-lg text-sm text-slate-500">请尝试刷新数据、切换市场，或调整 CANSLIM 阈值方案。</p>
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

function stockHasAIAnalysis(stock: ComputedStock, aiAnalyzedSymbols: Set<string>) {
	return stock.ai_analyzed || aiAnalyzedSymbols.has(stock.symbol.toUpperCase());
}

interface StockRowProps {
  stock: ComputedStock;
  index: number;
  hasAIAnalysis: boolean;
  onOpen: (stock: ComputedStock) => void;
}

const StockRow = memo(function StockRow({ stock, index, hasAIAnalysis, onOpen }: StockRowProps) {
  const animated = index < ROW_ANIMATION_LIMIT;

  return (
    <button
      className={cn(
        'grid w-full grid-cols-[1fr_1.5fr_2fr_1fr_0.8fr] items-center gap-4 px-5 py-4 text-left transition hover:bg-slate-800/70',
        animated ? 'animate-fade-in opacity-0' : 'opacity-100'
      )}
      style={animated ? { animationDelay: index * 16 + 'ms' } : undefined}
      onClick={() => onOpen(stock)}
    >
      <span className="flex min-w-0 items-center gap-2 font-semibold text-white">
        <BarChart3 className="h-4 w-4 text-finance-blue" />
        <span className="truncate">{stock.symbol}</span>
        {hasAIAnalysis ? <AIAnalysisBadge /> : null}
      </span>
      <span className="truncate text-slate-300">{stock.name}</span>
      <span className="flex flex-wrap gap-2">
        {tagOrder.map((tag) => (
          <FilterBadge key={tag} stock={stock} tag={tag} />
        ))}
      </span>
      <span className="text-sm text-slate-400">{marketLabel[stock.market]}</span>
      <span className="text-right text-sm text-slate-300">
        {stock.passCount}/{scoreDenominator} · {stock.weightSum}
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
      openTab(stock.symbol, stock.name);
      navigate('/analysis/' + stock.symbol, { state: { name: stock.name } });
    },
    [navigate, openTab]
  );

  if (!hasRawData || stocks.length === 0) {
    return <EmptyState filtered={filtered && hasRawData} />;
  }

  return (
    <Card className="overflow-hidden p-0">
      <div className="flex items-center justify-between border-b border-slate-800 px-5 py-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.22em] text-cyan-300/70">Results</p>
          <h2 className="mt-1 text-lg font-bold text-white">候选股票结果</h2>
        </div>
        <span className="rounded-full border border-slate-700 bg-slate-950/50 px-3 py-1 text-xs text-slate-400">
          已显示 {visibleStocks.length}/{stocks.length} 支
        </span>
      </div>
      <div className="hidden grid-cols-[1fr_1.4fr_2fr_1fr_0.8fr] gap-4 border-b border-slate-800 px-5 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 md:grid">
        <span>代码</span>
        <span>名称</span>
        <span>标签</span>
        <span>市场</span>
        <span className="text-right">得分</span>
      </div>
      <div className="divide-y divide-slate-900">
        {visibleStocks.map((stock, index) => (
          <StockRow
            key={stockIdentity(stock)}
            stock={stock}
            index={index}
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
