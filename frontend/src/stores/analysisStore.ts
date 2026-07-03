import { create } from 'zustand';
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

const initialChartStatus: SymbolAnalysisState['chartStatus'] = {
  summary: 'loading',
  kline: 'loading',
  rs: 'loading',
  eps: 'loading',
  revenue: 'loading',
  grossMargin: 'loading'
};

export function createInitialAnalysisState(): SymbolAnalysisState {
  return {
    stockSummary: null,
    klineData: null,
    klinePeriod: 'daily',
    rsData: null,
    epsData: null,
    revenueData: null,
    grossMarginData: null,
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

export const useAnalysisStore = create<AnalysisStore>((set, get) => ({
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
}));
