import type { AIAnalysisContext, AIAnalysisStreamEventName, AnalysisConfig } from '../types/analysis';
import { API_BASE_URL } from './api';
import { aiAnalysisStreamPath } from './aiAnalysisContract';

export interface ParsedAIAnalysisStreamEvent {
  event: AIAnalysisStreamEventName | string;
  data: string;
}

export type AIAnalysisStartRequest = AnalysisConfig & {
  analysis_context?: AIAnalysisContext;
};

export function buildAIAnalysisStartRequest(config: AnalysisConfig, context?: AIAnalysisContext): AIAnalysisStartRequest {
  return context ? { ...config, analysis_context: context } : { ...config };
}

export function aiAnalysisStreamURL(symbol: string) {
  return API_BASE_URL + aiAnalysisStreamPath(symbol);
}

export function parseAIAnalysisSSEBlock(block: string): ParsedAIAnalysisStreamEvent | null {
  const lines = block.split(/\r?\n/);
  let event = 'message';
  const dataLines: string[] = [];

  for (const line of lines) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim();
      continue;
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  if (!event && dataLines.length === 0) return null;
  return { event, data: dataLines.join('\n') };
}

export async function readAIAnalysisErrorMessage(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string; message?: string };
    return payload.error || payload.message || 'AI 分析请求失败';
  } catch {
    return 'AI 分析请求失败';
  }
}
