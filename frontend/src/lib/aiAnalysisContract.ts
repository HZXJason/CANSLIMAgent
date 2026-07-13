import type { AIAnalysisContract, AIAnalysisStreamEventName } from '../types/analysis';

export const AI_ANALYSIS_CONTRACT_VERSION = 'ai-analysis.v1';
export const AI_ANALYSIS_CONFIG_PATH = '/analysis/ai/config';
export const AI_ANALYSIS_STREAM_PATH_TEMPLATE = '/analysis/{symbol}/ai';

export const AI_ANALYSIS_STREAM_EVENTS: AIAnalysisStreamEventName[] = [
  'heartbeat',
  'progress',
  'narration_delta',
  'report_start',
  'report_delta',
  'result',
  'thought',
  'report',
  'decision',
  'done',
  'error'
];

export const AI_ANALYSIS_CONTRACT: AIAnalysisContract = {
  version: AI_ANALYSIS_CONTRACT_VERSION,
  config_endpoint: '/api/v1' + AI_ANALYSIS_CONFIG_PATH,
  stream_endpoint: '/api/v1' + AI_ANALYSIS_STREAM_PATH_TEMPLATE,
  upstream_mode: 'sse',
  events: AI_ANALYSIS_STREAM_EVENTS
};

export function aiAnalysisStreamPath(symbol: string) {
  return '/analysis/' + encodeURIComponent(symbol.trim().toUpperCase()) + '/ai';
}
