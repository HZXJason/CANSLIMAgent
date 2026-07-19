import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { AssetMarket } from '../types';

interface MarketStore {
  assetMarket: AssetMarket;
  setAssetMarket: (market: AssetMarket) => void;
}

export const useMarketStore = create<MarketStore>()(
  persist(
    (set) => ({
      assetMarket: 'US',
      setAssetMarket: (assetMarket) => set({ assetMarket })
    }),
    {
      name: 'alphalens-asset-market',
      version: 1,
      partialize: (state) => ({ assetMarket: state.assetMarket })
    }
  )
);
