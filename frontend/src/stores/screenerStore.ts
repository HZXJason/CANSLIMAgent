import { create } from 'zustand';
import type { Market, ScanProgress, ScreenerFilterKey, StockWithMetrics, ThresholdConfig, ThresholdPreset } from '../types';

export const DEFAULT_THRESHOLDS: ThresholdConfig = {
  c_min_growth: 25,
  a_min_growth: 25,
  a_min_years: 3,
  s_volume_ratio: 150,
  s_ma_period: 50,
  l_min_rs: 80,
  i_min_delta: 1
};

interface ScreenerStore {
  selectedMarket: Market;
  setMarket: (market: Market) => void;
  thresholds: ThresholdConfig;
  setThresholds: (thresholds: Partial<ThresholdConfig>) => void;
  activePresetId: number | null;
  setActivePresetId: (id: number | null) => void;
  presets: ThresholdPreset[];
  setPresets: (presets: ThresholdPreset[]) => void;
  applyPreset: (preset: ThresholdPreset) => void;
  thresholdPanelOpen: boolean;
  toggleThresholdPanel: () => void;
  rawStocks: StockWithMetrics[];
  setMarketStocks: (market: Market, stocks: StockWithMetrics[]) => void;
  addStockResult: (stock: StockWithMetrics) => void;
  clearMarketResults: (market: Market) => void;
  clearResults: () => void;
  scanProgress: ScanProgress;
  setScanProgress: (progress: ScanProgress) => void;
  dataAsOf: string | null;
  setDataAsOf: (value: string | null) => void;
  isScanning: boolean;
  setIsScanning: (value: boolean) => void;
  activeTagFilters: ScreenerFilterKey[];
  toggleTagFilter: (tag: ScreenerFilterKey) => void;
}

function stockKey(stock: StockWithMetrics) {
  return stock.market + ':' + stock.symbol;
}

export const useScreenerStore = create<ScreenerStore>((set) => ({
  selectedMarket: 'SP500',
  setMarket: (market) => set({ selectedMarket: market }),
  thresholds: DEFAULT_THRESHOLDS,
  setThresholds: (thresholds) => set((state) => ({ thresholds: { ...state.thresholds, ...thresholds }, activePresetId: null })),
  activePresetId: null,
  setActivePresetId: (id) => set({ activePresetId: id }),
  presets: [],
  setPresets: (presets) => set({ presets }),
  applyPreset: (preset) =>
    set({
      activePresetId: preset.id,
      thresholds: {
        c_min_growth: preset.c_min_growth,
        a_min_growth: preset.a_min_growth,
        a_min_years: preset.a_min_years,
        s_volume_ratio: preset.s_volume_ratio,
        s_ma_period: preset.s_ma_period,
        l_min_rs: preset.l_min_rs,
        i_min_delta: preset.i_min_delta
      }
    }),
  thresholdPanelOpen: false,
  toggleThresholdPanel: () => set((state) => ({ thresholdPanelOpen: !state.thresholdPanelOpen })),
  rawStocks: [],
  setMarketStocks: (market, stocks) =>
    set((state) => {
      const nextMarketStocks = stocks.filter((stock) => stock.market === market);
      const otherMarketStocks = state.rawStocks.filter((stock) => stock.market !== market);
      const merged = new Map<string, StockWithMetrics>();
      [...otherMarketStocks, ...nextMarketStocks].forEach((stock) => {
        merged.set(stockKey(stock), stock);
      });
      return {
        rawStocks: Array.from(merged.values()),
        dataAsOf: nextMarketStocks[0]?.data_as_of ?? null,
        scanProgress: { analyzed: nextMarketStocks.length, total: nextMarketStocks.length }
      };
    }),
  addStockResult: (stock) =>
    set((state) => {
      const next = state.rawStocks.filter((item) => stockKey(item) !== stockKey(stock));
      next.push(stock);
      return { rawStocks: next, dataAsOf: stock.data_as_of };
    }),
  clearMarketResults: (market) =>
    set((state) => ({
      rawStocks: state.rawStocks.filter((stock) => stock.market !== market),
      scanProgress: { analyzed: 0, total: 0 },
      dataAsOf: null
    })),
  clearResults: () => set({ rawStocks: [], scanProgress: { analyzed: 0, total: 0 }, dataAsOf: null }),
  scanProgress: { analyzed: 0, total: 0 },
  setScanProgress: (progress) => set({ scanProgress: progress }),
  dataAsOf: null,
  setDataAsOf: (value) => set({ dataAsOf: value }),
  isScanning: false,
  setIsScanning: (value) => set({ isScanning: value }),
  activeTagFilters: [],
  toggleTagFilter: (tag) =>
    set((state) => ({
      activeTagFilters: state.activeTagFilters.includes(tag)
        ? state.activeTagFilters.filter((item) => item !== tag)
        : [...state.activeTagFilters, tag]
    }))
}));
