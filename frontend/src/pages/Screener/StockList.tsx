import { BadgeCheck, BarChart3, SearchX } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Card } from '../../components/ui/card';
import { Tooltip } from '../../components/ui/tooltip';
import { ACTIVE_LABEL_KEYS, labelStatusText } from '../../lib/computeLabels';
import { cn } from '../../lib/utils';
import { useTabStore } from '../../stores/tabStore';
import type { ComputedStock, LabelKey, LabelResult, Market } from '../../types';

const tagOrder: LabelKey[] = ACTIVE_LABEL_KEYS;

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

function NLabelBadge() {
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

export default function StockList({ stocks, hasRawData, filtered }: StockListProps) {
  const navigate = useNavigate();
  const openTab = useTabStore((state) => state.openTab);

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
        <span className="rounded-full border border-slate-700 bg-slate-950/50 px-3 py-1 text-xs text-slate-400">{stocks.length} 支</span>
      </div>
      <div className="hidden grid-cols-[1fr_1.4fr_2fr_1fr_0.8fr] gap-4 border-b border-slate-800 px-5 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 md:grid">
        <span>代码</span>
        <span>名称</span>
        <span>标签</span>
        <span>市场</span>
        <span className="text-right">得分</span>
      </div>
      <div className="divide-y divide-slate-900">
        {stocks.map((stock, index) => (
          <button
            key={stock.symbol}
            className={cn(
              'grid w-full grid-cols-[1fr_1.5fr_2fr_1fr_0.8fr] items-center gap-4 px-5 py-4 text-left transition hover:bg-slate-800/70',
              'animate-fade-in opacity-0'
            )}
            style={{ animationDelay: index * 35 + 'ms' }}
            onClick={() => {
              openTab(stock.symbol, stock.name);
              navigate('/analysis/' + stock.symbol, { state: { name: stock.name } });
            }}
          >
            <span className="flex items-center gap-2 font-semibold text-white">
              <BarChart3 className="h-4 w-4 text-finance-blue" />
              {stock.symbol}
            </span>
            <span className="truncate text-slate-300">{stock.name}</span>
            <span className="flex flex-wrap gap-2">
              {tagOrder.map((tag) => (
                <LabelBadge key={tag} label={stock.labels[tag]} />
              ))}
              {stock.metrics.n ? <NLabelBadge /> : null}
            </span>
            <span className="text-sm text-slate-400">{marketLabel[stock.market]}</span>
            <span className="text-right text-sm text-slate-300">
              {stock.passCount}/{tagOrder.length} · {stock.weightSum}
            </span>
          </button>
        ))}
      </div>
    </Card>
  );
}
