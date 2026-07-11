import type { APIResponse, Market, RefreshJob, RefreshJobResponse, StockWithMetrics, ThresholdConfig, ThresholdPreset } from '../types';
import type {
	AIAnalysisMarkResponse,
	AIAnalysisConfigResponse,
  AIModelDiscoveryInput,
  AIModelDiscoveryResponse,
  CompanyProfile,
  EPSData,
  GrossMarginData,
  KLineData,
  KLinePeriod,
  NLabelResponse,
  RevenueData,
  RSData,
  SecondLayerData,
  StockSummary
} from '../types/analysis';
import type { JournalEntry, JournalReflectionInput, NewJournalEntryInput, StoredJournalReflection } from '../types/journal';
import type { ExecuteTradePlanInput, ExecuteTradePlanResponse, NewTradePlanInput, TradePlan, TradePlanFilters, TradePlanUpdateInput } from '../types/tradePlan';
import { AI_ANALYSIS_CONFIG_PATH, AI_ANALYSIS_MODELS_PATH } from './aiAnalysisContract';

export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api/v1';

const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), DEFAULT_REQUEST_TIMEOUT_MS);
  const combinedSignal = init?.signal ?? controller.signal;

  try {
    const response = await fetch(API_BASE_URL + path, {
      ...init,
      signal: combinedSignal,
      headers: {
        ...Object.fromEntries(headers.entries())
      }
    });
    const payload = (await response.json()) as APIResponse<T>;
    if (!response.ok || !payload.success) {
      throw new Error(payload.error ?? '请求失败');
    }
    return payload.data;
  } finally {
    clearTimeout(timeoutId);
  }
}

export function fetchCachedResults(market: Market) {
  return request<StockWithMetrics[]>('/screener/results?market=' + market);
}

export function fetchPresets() {
  return request<ThresholdPreset[]>('/presets');
}

export function createPreset(name: string, thresholds: ThresholdConfig) {
  return request<ThresholdPreset>('/presets', {
    method: 'POST',
    body: JSON.stringify({ name, ...thresholds })
  });
}

export function deletePreset(id: number) {
  return request<{ deleted: boolean }>('/presets/' + id, {
    method: 'DELETE'
  });
}

export function refreshMarket(market: Market) {
  return request<RefreshJobResponse>('/screener/refresh', {
    method: 'POST',
    body: JSON.stringify({ market })
  });
}

export function fetchRefreshJob(jobID: string) {
  return request<RefreshJob>('/screener/refresh/' + jobID);
}

export function fetchStockSummary(symbol: string) {
  return request<StockSummary>('/stocks/' + encodeURIComponent(symbol) + '/summary');
}

export function fetchCompanyProfile(symbol: string) {
  return request<CompanyProfile>('/stocks/' + encodeURIComponent(symbol) + '/profile');
}

export function fetchKLine(symbol: string, period: KLinePeriod) {
  return request<KLineData>('/stocks/' + encodeURIComponent(symbol) + '/kline?period=' + period);
}

export function fetchRS(symbol: string, period: KLinePeriod) {
  return request<RSData>('/stocks/' + encodeURIComponent(symbol) + '/rs?period=' + period);
}

export function fetchEPSQuarterly(symbol: string) {
  return request<EPSData>('/stocks/' + encodeURIComponent(symbol) + '/eps/quarterly');
}

export function fetchRevenueAnnual(symbol: string) {
  return request<RevenueData>('/stocks/' + encodeURIComponent(symbol) + '/revenue/annual');
}

export function fetchGrossMarginQuarterly(symbol: string) {
  return request<GrossMarginData>('/stocks/' + encodeURIComponent(symbol) + '/margin/gross/quarterly');
}

export function fetchSecondLayerData(symbol: string) {
  return request<SecondLayerData>('/stocks/' + encodeURIComponent(symbol) + '/second-layer-data');
}

export function fetchAIAnalysisConfig() {
  return request<AIAnalysisConfigResponse>(AI_ANALYSIS_CONFIG_PATH);
}

export function discoverAIModels(input: AIModelDiscoveryInput) {
  return request<AIModelDiscoveryResponse>(AI_ANALYSIS_MODELS_PATH, {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function updateNLabel(symbol: string, nLabel: boolean, analysisSummary: string) {
  return request<NLabelResponse>('/stocks/' + encodeURIComponent(symbol) + '/n-label', {
    method: 'PATCH',
    body: JSON.stringify({ n_label: nLabel, analysis_summary: analysisSummary })
  });
}

export function markAIAnalysisCompleted(symbol: string) {
	return request<AIAnalysisMarkResponse>('/stocks/' + encodeURIComponent(symbol) + '/ai-analysis', {
		method: 'PATCH'
	});
}

export function fetchJournalEntries() {
  return request<JournalEntry[]>('/journal');
}

export function fetchJournalEntry(id: string) {
  return request<JournalEntry>('/journal/' + encodeURIComponent(id));
}

export function createJournalEntry(input: NewJournalEntryInput) {
  return request<JournalEntry>('/journal', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function updateJournalReflection(id: string, input: JournalReflectionInput) {
  return request<JournalEntry>('/journal/' + encodeURIComponent(id), {
    method: 'PATCH',
    body: JSON.stringify(input)
  });
}

export function generateJournalReflection() {
  return request<StoredJournalReflection>('/journal/reflections', { method: 'POST' });
}

export function fetchLatestJournalReflection() {
  return request<StoredJournalReflection | null>('/journal/reflections/latest');
}

export function fetchTradePlans(filters: TradePlanFilters = {}) {
  const params = new URLSearchParams();
  if (filters.ticker) params.set('ticker', filters.ticker);
  if (filters.status) params.set('status', filters.status);
  const query = params.toString();
  return request<TradePlan[]>('/trade-plans' + (query ? '?' + query : ''));
}

export function fetchTradePlan(id: string) {
  return request<TradePlan>('/trade-plans/' + encodeURIComponent(id));
}

export function createTradePlan(input: NewTradePlanInput) {
  return request<TradePlan>('/trade-plans', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function updateTradePlan(id: string, input: TradePlanUpdateInput) {
  return request<TradePlan>('/trade-plans/' + encodeURIComponent(id), {
    method: 'PATCH',
    body: JSON.stringify(input)
  });
}

export function executeTradePlan(id: string, input: ExecuteTradePlanInput) {
  return request<ExecuteTradePlanResponse>('/trade-plans/' + encodeURIComponent(id) + '/execute', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}
