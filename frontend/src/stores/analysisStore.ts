import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type {
  AIAgentProgress,
  AIAgentResult,
  AIDecision,
  ChartStatus,
  KLinePeriod,
  StockSummary,
  SymbolAnalysisState,
  ThoughtLine
} from '../types/analysis';

const ANALYSIS_CACHE_STORAGE_KEY = 'alphalens-analysis-state-v1';

const initialChartStatus: SymbolAnalysisState['chartStatus'] = {
  summary: 'loading',
  profile: 'loading',
  kline: 'loading',
  rs: 'loading',
  eps: 'loading',
  revenue: 'loading',
  grossMargin: 'loading',
  secondLayer: 'loading'
};

export function createInitialAnalysisState(): SymbolAnalysisState {
  return {
    stockSummary: null,
    companyProfile: null,
    klineData: null,
    klinePeriod: 'daily',
    rsData: null,
    epsData: null,
    revenueData: null,
    grossMarginData: null,
    secondLayerData: null,
    chartStatus: { ...initialChartStatus },
    aiStatus: 'idle',
    thoughtLines: [],
    liveNarration: '',
    reportContent: '',
    aiProgress: [],
    aiResults: [],
    aiDecision: null,
    aiError: null,
    thoughtExpanded: true,
    nLabelWritten: false
  };
}

function statusForCount(length: number): ChartStatus {
  return length > 0 ? 'success' : 'insufficient';
}

function normalizeCachedProgress(progress: AIAgentProgress, interrupted: boolean): AIAgentProgress {
  if (!interrupted || progress.status === 'done' || progress.status === 'error') return progress;
  return {
    ...progress,
    status: 'error',
    message: '页面刷新后连接已中断，请重新生成',
    timestamp: Date.now()
  };
}

function normalizeCachedState(state: Partial<SymbolAnalysisState>): SymbolAnalysisState {
  const next: SymbolAnalysisState = {
    ...createInitialAnalysisState(),
    ...state,
    chartStatus: { ...initialChartStatus, ...(state.chartStatus ?? {}) }
  };
  const interrupted = next.aiStatus === 'streaming';

  if (next.stockSummary) next.chartStatus.summary = 'success';
  if (next.companyProfile) next.chartStatus.profile = next.companyProfile.description ? 'success' : 'insufficient';
  if (next.klineData) next.chartStatus.kline = statusForCount(next.klineData.candles.length);
  if (next.rsData) next.chartStatus.rs = statusForCount(next.rsData.rs_line.length);
  if (next.epsData) next.chartStatus.eps = statusForCount(next.epsData.quarters.length);
  if (next.revenueData) next.chartStatus.revenue = statusForCount(next.revenueData.years.length);
  if (next.grossMarginData) next.chartStatus.grossMargin = statusForCount(next.grossMarginData.quarters.length);
  if (next.secondLayerData) next.chartStatus.secondLayer = 'success';

  if (interrupted) {
    next.aiStatus = next.reportContent || next.liveNarration || next.aiProgress.length > 0 ? 'error' : 'idle';
    next.aiError = '页面刷新后 AI 流式连接已中断，可重新生成。';
    next.aiProgress = next.aiProgress.map((item) => normalizeCachedProgress(item, true));
  }

  return next;
}

function normalizeCachedStates(states: Record<string, Partial<SymbolAnalysisState>> | undefined) {
  if (!states) return {};
  return Object.fromEntries(
    Object.entries(states).map(([symbol, state]) => [normalizeSymbol(symbol), normalizeCachedState(state)])
  );
}

interface AnalysisStore {
  analysisStates: Record<string, SymbolAnalysisState>;
  getStateForSymbol: (symbol: string) => SymbolAnalysisState;
  updateState: (symbol: string, patch: Partial<SymbolAnalysisState>) => void;
  updateChartStatus: (symbol: string, key: keyof SymbolAnalysisState['chartStatus'], status: ChartStatus) => void;
  setKLinePeriod: (symbol: string, period: KLinePeriod) => void;
  appendThought: (symbol: string, line: ThoughtLine) => void;
  appendLiveNarration: (symbol: string, content: string) => void;
  clearLiveNarration: (symbol: string) => void;
  appendReport: (symbol: string, content: string) => void;
  upsertAIProgress: (symbol: string, progress: Omit<AIAgentProgress, 'timestamp'>) => void;
  appendAIResult: (symbol: string, result: Omit<AIAgentResult, 'timestamp'>) => void;
  setAIDecision: (symbol: string, decision: AIDecision) => void;
  resetAI: (symbol: string) => void;
  markDone: (symbol: string) => void;
  markAIError: (symbol: string, message: string) => void;
  setNLabelWritten: (symbol: string, value: boolean) => void;
  setSummary: (symbol: string, summary: StockSummary) => void;
  removeState: (symbol: string) => void;
}

type PersistedAnalysisStore = Pick<AnalysisStore, 'analysisStates'>;

function normalizeSymbol(symbol: string) {
  return symbol.trim().toUpperCase();
}

function withSymbolState(
  states: Record<string, SymbolAnalysisState>,
  symbol: string,
  updater: (state: SymbolAnalysisState) => SymbolAnalysisState
) {
  const normalized = normalizeSymbol(symbol);
  const current = states[normalized] ?? createInitialAnalysisState();
  return { ...states, [normalized]: updater(current) };
}

export const useAnalysisStore = create<AnalysisStore>()(
  persist(
    (set, get) => ({
      analysisStates: {},
      getStateForSymbol: (symbol) => get().analysisStates[normalizeSymbol(symbol)] ?? createInitialAnalysisState(),
      updateState: (symbol, patch) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({ ...current, ...patch }))
        })),
      updateChartStatus: (symbol, key, status) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            chartStatus: { ...current.chartStatus, [key]: status }
          }))
        })),
      setKLinePeriod: (symbol, period) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({ ...current, klinePeriod: period }))
        })),
      appendThought: (symbol, line) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            thoughtLines: [...current.thoughtLines, line]
          }))
        })),
      appendLiveNarration: (symbol, content) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            liveNarration: current.liveNarration + content
          }))
        })),
      clearLiveNarration: (symbol) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            liveNarration: ''
          }))
        })),
      appendReport: (symbol, content) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            reportContent: current.reportContent + content
          }))
        })),
      upsertAIProgress: (symbol, progress) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => {
            const nextProgress = {
              ...progress,
              timestamp: Date.now()
            };
            const existingIndex = current.aiProgress.findIndex((item) => item.agent === progress.agent);
            if (existingIndex === -1) {
              return {
                ...current,
                aiProgress: [...current.aiProgress, nextProgress]
              };
            }
            const aiProgress = [...current.aiProgress];
            aiProgress[existingIndex] = nextProgress;
            return {
              ...current,
              aiProgress
            };
          })
        })),
      appendAIResult: (symbol, result) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => {
            const timestamp = Date.now();
            const reportSection = '\n\n## ' + result.agent + '\n\n' + result.content;
            return {
              ...current,
              aiResults: [...current.aiResults, { ...result, timestamp }],
              reportContent: current.reportContent + reportSection
            };
          })
        })),
      setAIDecision: (symbol, decision) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => {
            const decisionSection = '\n\n## 最终决策\n\n' + decision.summary;
            const reportContent = decision.summary && current.reportContent.includes(decision.summary)
              ? current.reportContent
              : current.reportContent + decisionSection;
            return {
              ...current,
              aiDecision: decision,
              reportContent
            };
          })
        })),
      resetAI: (symbol) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            aiStatus: 'streaming',
            thoughtLines: [],
            liveNarration: '',
            reportContent: '',
            aiProgress: [],
            aiResults: [],
            aiDecision: null,
            aiError: null,
            thoughtExpanded: true,
            nLabelWritten: false
          }))
        })),
      markDone: (symbol) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            aiStatus: 'done',
            aiProgress: current.aiProgress.map((item) =>
              item.status === 'error'
                ? item
                : {
                    ...item,
                    status: 'done',
                    message: '分析完成',
                    timestamp: Date.now()
                  }
            ),
            thoughtExpanded: false
          }))
        })),
      markAIError: (symbol, message) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            aiStatus: 'error',
            aiError: message
          }))
        })),
      setNLabelWritten: (symbol, value) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({ ...current, nLabelWritten: value }))
        })),
      setSummary: (symbol, summary) =>
        set((state) => ({
          analysisStates: withSymbolState(state.analysisStates, symbol, (current) => ({
            ...current,
            stockSummary: summary,
            nLabelWritten: Boolean(summary.metrics.n)
          }))
        })),
      removeState: (symbol) =>
        set((state) => {
          const normalized = normalizeSymbol(symbol);
          const next = { ...state.analysisStates };
          delete next[normalized];
          return { analysisStates: next };
        })
    }),
    {
      name: ANALYSIS_CACHE_STORAGE_KEY,
      storage: createJSONStorage(() => sessionStorage),
      partialize: (state): PersistedAnalysisStore => ({
        analysisStates: normalizeCachedStates(state.analysisStates)
      }),
      merge: (persisted, current) => {
        const persistedState = persisted as Partial<PersistedAnalysisStore> | undefined;
        return {
          ...current,
          analysisStates: normalizeCachedStates(persistedState?.analysisStates)
        };
      }
    }
  )
);
