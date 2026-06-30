import { create } from 'zustand';

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

export const useTabStore = create<TabStore>((set) => ({
  tabs: [],
  activeTabId: null,
  openTab: (symbol, name) =>
    set((state) => {
      const normalized = symbol.toUpperCase();
      const exists = state.tabs.some((tab) => tab.symbol === normalized);
      return {
        tabs: exists ? state.tabs : [...state.tabs, { symbol: normalized, name, openedAt: Date.now() }],
        activeTabId: normalized
      };
    }),
  closeTab: (symbol) =>
    set((state) => {
      const normalized = symbol.toUpperCase();
      const tabs = state.tabs.filter((tab) => tab.symbol !== normalized);
      let activeTabId = state.activeTabId;
      if (state.activeTabId === normalized) {
        activeTabId = tabs[tabs.length - 1]?.symbol ?? null;
      }
      return { tabs, activeTabId };
    }),
  setActiveTab: (symbol) => set({ activeTabId: symbol.toUpperCase() })
}));
