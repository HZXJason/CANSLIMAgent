import type { Market } from '../../types';
import { cn } from '../../lib/utils';

const markets: Array<{ value: Market; label: string }> = [
  { value: 'SP500', label: '标普 500' },
  { value: 'NASDAQ100', label: '纳指 100' },
  { value: 'RUSSELL2000', label: '罗素 2000' }
];

interface MarketSelectorProps {
  selectedMarket: Market;
  onChange: (market: Market) => void;
  disabled?: boolean;
}

export default function MarketSelector({ selectedMarket, onChange, disabled = false }: MarketSelectorProps) {
  return (
    <div
      className="grid w-full grid-cols-3 gap-1.5 rounded-2xl border border-slate-800/90 bg-slate-950/45 p-1.5"
      role="tablist"
      aria-label="市场筛选"
    >
      {markets.map((market) => (
        <button
          key={market.value}
          type="button"
          role="tab"
          aria-selected={selectedMarket === market.value}
          disabled={disabled}
          className={cn(
            'interactive-press min-h-11 rounded-xl border px-2 text-sm font-bold sm:px-4',
            disabled
              ? 'cursor-not-allowed border-transparent text-slate-600'
              : selectedMarket === market.value
                ? 'border-cyan-300/30 bg-cyan-300/[0.12] text-cyan-50 shadow-[inset_0_-2px_0_rgba(103,216,243,0.75)]'
                : 'border-transparent text-slate-400 hover:border-slate-700/70 hover:bg-slate-800/65 hover:text-white'
          )}
          onClick={() => onChange(market.value)}
        >
          {market.label}
        </button>
      ))}
    </div>
  );
}
