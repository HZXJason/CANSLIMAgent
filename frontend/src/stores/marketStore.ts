import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { AssetMarket } from '../types';

interface MarketStore {
  assetMarket: AssetMarket;
  hasSelectedMarket: boolean;
  setAssetMarket: (market: AssetMarket) => void;
}

export const useMarketStore = create<MarketStore>()(
  persist(
    (set) => ({
      assetMarket: 'US',
      hasSelectedMarket: false,
      setAssetMarket: (assetMarket) => set({ assetMarket, hasSelectedMarket: true })
    }),
    {
      name: 'alphalens-asset-market',
      version: 2,
      migrate: (persistedState, version) => {
        const previous = persistedState as Partial<MarketStore> | undefined;
        return {
          assetMarket: previous?.assetMarket === 'CN' ? 'CN' : 'US',
          hasSelectedMarket: version >= 2 ? Boolean(previous?.hasSelectedMarket) : false
        };
      },
      partialize: (state) => ({ assetMarket: state.assetMarket, hasSelectedMarket: state.hasSelectedMarket })
    }
  )
);
