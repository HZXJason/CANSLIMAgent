import { ChevronDown } from 'lucide-react';
import { ASSET_MARKET_OPTIONS } from '../lib/markets';
import { cn } from '../lib/utils';
import { useMarketStore } from '../stores/marketStore';
import type { AssetMarket } from '../types';

interface GlobalMarketSelectorProps {
  compact?: boolean;
  className?: string;
}

export default function GlobalMarketSelector({ compact = false, className }: GlobalMarketSelectorProps) {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const setAssetMarket = useMarketStore((state) => state.setAssetMarket);

  return (
    <label className={cn('relative inline-flex min-w-0 items-center', className)}>
      <span className="sr-only">股票市场</span>
      <select
        aria-label="股票市场"
        value={assetMarket}
        onChange={(event) => setAssetMarket(event.target.value as AssetMarket)}
        className={cn(
          'h-11 min-w-0 appearance-none border-0 bg-transparent py-0 pl-0 pr-5 text-sm font-bold text-white opacity-100 outline-none focus-visible:ring-2 focus-visible:ring-brand-300/70 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas',
          compact ? 'w-[3.25rem] text-xs' : 'w-[6.5rem]'
        )}
      >
        {ASSET_MARKET_OPTIONS.map((option) => (
          <option key={option.value} value={option.value} className="bg-surface text-white">
            {compact ? option.compactLabel : option.label}
          </option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-0 h-3.5 w-3.5 text-brand-300" aria-hidden="true" />
    </label>
  );
}
