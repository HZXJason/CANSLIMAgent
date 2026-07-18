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
      className="grid w-full grid-cols-3 gap-1 rounded-2xl border border-slate-800 bg-slate-950/45 p-1"
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
            'min-h-11 rounded-xl px-2 text-sm font-semibold transition sm:px-4',
            disabled
              ? 'cursor-not-allowed text-slate-600'
              : selectedMarket === market.value
                ? 'bg-cyan-400 text-slate-950 shadow-sm shadow-cyan-950/40'
                : 'text-slate-400 hover:bg-slate-800/80 hover:text-white'
          )}
          onClick={() => onChange(market.value)}
        >
          {market.label}
        </button>
      ))}
    </div>
  );
}
