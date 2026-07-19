import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { assetMarketForSymbol, normalizeSymbolForAsset } from '../lib/markets';
import type { AssetMarket } from '../types';

export interface StockTab {
  symbol: string;
  name: string;
  openedAt: number;
}

interface TabStore {
  tabs: StockTab[];
  activeTabIds: Record<AssetMarket, string | null>;
  openTab: (symbol: string, name: string) => void;
  closeTab: (symbol: string) => void;
  setActiveTab: (symbol: string) => void;
}

interface PersistedTabState {
  tabs?: StockTab[];
  activeTabId?: string | null;
  activeTabIds?: Partial<Record<AssetMarket, string | null>>;
}

const EMPTY_ACTIVE_TABS: Record<AssetMarket, string | null> = { US: null, CN: null };

function canonicalSymbol(symbol: string) {
  const assetMarket = assetMarketForSymbol(symbol);
  if (!assetMarket) return null;
  return {
    assetMarket,
    symbol: normalizeSymbolForAsset(assetMarket, symbol)
  };
}

function normalizeName(symbol: string, name: string) {
  return name.trim() || symbol;
}

function canonicalTabs(tabs: StockTab[] = []) {
  const bySymbol = new Map<string, StockTab>();
  tabs.forEach((tab) => {
    const canonical = canonicalSymbol(tab.symbol);
    if (!canonical) return;
    const existing = bySymbol.get(canonical.symbol);
    bySymbol.set(canonical.symbol, {
      symbol: canonical.symbol,
      name: normalizeName(canonical.symbol, tab.name),
      openedAt: existing ? Math.min(existing.openedAt, tab.openedAt) : tab.openedAt
    });
  });
  return Array.from(bySymbol.values());
}

function migratedActiveTabs(state: PersistedTabState, tabs: StockTab[]) {
  const result = { ...EMPTY_ACTIVE_TABS };
  (['US', 'CN'] as const).forEach((assetMarket) => {
    const legacyActive = state.activeTabId && assetMarketForSymbol(state.activeTabId) === assetMarket ? state.activeTabId : null;
    const candidate = state.activeTabIds?.[assetMarket] ?? legacyActive;
    const canonical = candidate ? normalizeSymbolForAsset(assetMarket, candidate) : '';
    const matchingTabs = tabs.filter((tab) => assetMarketForSymbol(tab.symbol) === assetMarket);
    result[assetMarket] = matchingTabs.some((tab) => tab.symbol === canonical)
      ? canonical
      : matchingTabs[matchingTabs.length - 1]?.symbol ?? null;
  });
  return result;
}

export const useTabStore = create<TabStore>()(
  persist(
    (set) => ({
      tabs: [],
      activeTabIds: { ...EMPTY_ACTIVE_TABS },
      openTab: (symbol, name) =>
        set((state) => {
          const canonical = canonicalSymbol(symbol);
          if (!canonical) return state;
          const normalizedName = normalizeName(canonical.symbol, name);
          const existing = state.tabs.find((tab) => tab.symbol === canonical.symbol);
          return {
            tabs: existing
              ? state.tabs.map((tab) => (tab.symbol === canonical.symbol ? { ...tab, name: normalizedName } : tab))
              : [...state.tabs, { symbol: canonical.symbol, name: normalizedName, openedAt: Date.now() }],
            activeTabIds: { ...state.activeTabIds, [canonical.assetMarket]: canonical.symbol }
          };
        }),
      closeTab: (symbol) =>
        set((state) => {
          const canonical = canonicalSymbol(symbol);
          if (!canonical) return state;
          const marketTabs = state.tabs.filter((tab) => assetMarketForSymbol(tab.symbol) === canonical.assetMarket);
          const closedIndex = marketTabs.findIndex((tab) => tab.symbol === canonical.symbol);
          const tabs = state.tabs.filter((tab) => tab.symbol !== canonical.symbol);
          const activeTabIds = { ...state.activeTabIds };
          if (activeTabIds[canonical.assetMarket] === canonical.symbol) {
            const remainingMarketTabs = marketTabs.filter((tab) => tab.symbol !== canonical.symbol);
            activeTabIds[canonical.assetMarket] =
              remainingMarketTabs[Math.min(Math.max(closedIndex, 0), remainingMarketTabs.length - 1)]?.symbol ?? null;
          }
          return { tabs, activeTabIds };
        }),
      setActiveTab: (symbol) =>
        set((state) => {
          const canonical = canonicalSymbol(symbol);
          if (!canonical || !state.tabs.some((tab) => tab.symbol === canonical.symbol)) return state;
          return {
            activeTabIds: { ...state.activeTabIds, [canonical.assetMarket]: canonical.symbol }
          };
        })
    }),
    {
      name: 'alphalens-analysis-tabs',
      version: 2,
      migrate: (persistedState) => {
        const persisted = (persistedState ?? {}) as PersistedTabState;
        const tabs = canonicalTabs(persisted.tabs);
        return {
          tabs,
          activeTabIds: migratedActiveTabs(persisted, tabs)
        };
      },
      partialize: (state) => ({
        tabs: state.tabs,
        activeTabIds: state.activeTabIds
      })
    }
  )
);
