import { SCREENER_MARKETS } from '../../lib/markets';
import { cn } from '../../lib/utils';
import type { AssetMarket, Market } from '../../types';

interface MarketSelectorProps {
  assetMarket: AssetMarket;
  selectedMarket: Market;
  onChange: (market: Market) => void;
  disabled?: boolean;
}

export default function MarketSelector({ assetMarket, selectedMarket, onChange, disabled = false }: MarketSelectorProps) {
  const markets = SCREENER_MARKETS[assetMarket];

  return (
    <div
      className={cn(
        'grid w-full gap-1.5 rounded-2xl border border-slate-800/90 bg-slate-950/45 p-1.5',
        markets.length > 3 ? 'grid-cols-2 sm:grid-cols-3 lg:grid-cols-5' : 'grid-cols-3'
      )}
      role="tablist"
      aria-label={assetMarket === 'CN' ? 'A股板块筛选' : '美股市场筛选'}
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
                ? 'border-brand-300/30 bg-brand-300/[0.12] text-brand-50 shadow-[inset_0_-2px_0_var(--accent)]'
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
