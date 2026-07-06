import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface StockTab {
  symbol: string;
  name: string;
  openedAt: number;
}

interface TabStore {
  tabs: StockTab[];
  activeTabId: string | null;
  openTab: (symbol: string, name: string) => void;
  closeTab: (symbol: string) => void;
  setActiveTab: (symbol: string) => void;
}

function normalizeSymbol(symbol: string) {
  return symbol.trim().toUpperCase();
}

function normalizeName(symbol: string, name: string) {
  return name.trim() || symbol;
}

export const useTabStore = create<TabStore>()(
  persist(
    (set) => ({
      tabs: [],
      activeTabId: null,
      openTab: (symbol, name) =>
        set((state) => {
          const normalized = normalizeSymbol(symbol);
          const normalizedName = normalizeName(normalized, name);
          const existing = state.tabs.find((tab) => tab.symbol === normalized);
          return {
            tabs: existing
              ? state.tabs.map((tab) => (tab.symbol === normalized ? { ...tab, name: normalizedName } : tab))
              : [...state.tabs, { symbol: normalized, name: normalizedName, openedAt: Date.now() }],
            activeTabId: normalized
          };
        }),
      closeTab: (symbol) =>
        set((state) => {
          const normalized = normalizeSymbol(symbol);
          const tabs = state.tabs.filter((tab) => tab.symbol !== normalized);
          let activeTabId = state.activeTabId;
          if (state.activeTabId === normalized) {
            activeTabId = tabs[tabs.length - 1]?.symbol ?? null;
          }
          return { tabs, activeTabId };
        }),
      setActiveTab: (symbol) => set({ activeTabId: normalizeSymbol(symbol) })
    }),
    {
      name: 'alphalens-analysis-tabs',
      partialize: (state) => ({
        tabs: state.tabs,
        activeTabId: state.activeTabId
      })
    }
  )
);
