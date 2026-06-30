import type { Market } from '../../types';
import { cn } from '../../lib/utils';

const markets: Array<{ value: Market; label: string; description: string; hint: string; disabled?: boolean }> = [
  { value: 'SP500', label: '标普 500', description: '大型核心股票池', hint: '稳健 CANSLIM 初筛' },
  { value: 'NASDAQ100', label: '纳指 100', description: '科技成长股票池', hint: '高动量成长扫描' },
  { value: 'RUSSELL2000', label: '罗素 2000', description: '暂未开放', hint: '等待稳定成分股数据源接入', disabled: true }
];

interface MarketSelectorProps {
  selectedMarket: Market;
  onChange: (market: Market) => void;
}

export default function MarketSelector({ selectedMarket, onChange }: MarketSelectorProps) {
  return (
    <div className="grid gap-4 md:grid-cols-3">
      {markets.map((market) => (
        <button
          key={market.value}
          disabled={market.disabled}
          className={cn(
            'group flex min-h-36 flex-col items-start justify-between rounded-[1.25rem] border p-4 text-left transition duration-200',
            market.disabled
              ? 'cursor-not-allowed border-slate-800 bg-slate-950/20 opacity-55'
              : selectedMarket === market.value
              ? 'border-cyan-400 bg-finance-blue/70 shadow-glow'
              : 'border-slate-800 bg-slate-950/20 hover:border-cyan-400/40 hover:bg-slate-800/60'
          )}
          onClick={() => {
            if (!market.disabled) onChange(market.value);
          }}
        >
          <span
            className={cn(
              'rounded-full border px-3 py-1 text-xs font-semibold',
              market.disabled
                ? 'border-slate-700 bg-slate-900 text-slate-500'
                : selectedMarket === market.value
                ? 'border-emerald-300/50 bg-emerald-400 text-slate-950'
                : 'border-slate-700 bg-slate-900 text-slate-400'
            )}
          >
            {market.disabled ? '暂未开放' : selectedMarket === market.value ? '已选' : '可选'}
          </span>
          <span className="mt-3 text-lg font-bold text-white">{market.label}</span>
          <span className="mt-1 text-sm text-slate-400">{market.description}</span>
          <span className="mt-4 text-xs text-cyan-200/80">{market.hint}</span>
        </button>
      ))}
    </div>
  );
}
