import type { AIAnalysisContext, AIAnalysisStreamEventName, AnalysisConfig } from '../types/analysis';
import { API_BASE_URL } from './api';
import { aiAnalysisStreamPath } from './aiAnalysisContract';

export interface ParsedAIAnalysisStreamEvent {
  event: AIAnalysisStreamEventName | string;
  data: string;
  id?: number;
}

export type AIAnalysisStartRequest = AnalysisConfig & {
  analysis_context?: AIAnalysisContext;
  run_id: string;
  resume: boolean;
  last_event_id: number;
};

export function buildAIAnalysisStartRequest(
  config: AnalysisConfig,
  context: AIAnalysisContext | undefined,
  run: { runId: string; resume: boolean; lastEventId: number }
): AIAnalysisStartRequest {
  const payload = {
    ...config,
    run_id: run.runId,
    resume: run.resume,
    last_event_id: run.lastEventId
  };
  return context ? { ...payload, analysis_context: context } : payload;
}

export function aiAnalysisStreamURL(symbol: string) {
  return API_BASE_URL + aiAnalysisStreamPath(symbol);
}

export function parseAIAnalysisSSEBlock(block: string): ParsedAIAnalysisStreamEvent | null {
  const lines = block.split(/\r?\n/);
  let event = 'message';
  let id: number | undefined;
  const dataLines: string[] = [];

  for (const line of lines) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim();
      continue;
    }
    if (line.startsWith('id:')) {
      const parsed = Number(line.slice(3).trim());
      if (Number.isSafeInteger(parsed) && parsed >= 0) id = parsed;
      continue;
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  if (!event && dataLines.length === 0) return null;
  return { event, data: dataLines.join('\n'), id };
}

export async function readAIAnalysisErrorMessage(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string; message?: string };
    return payload.error || payload.message || 'AI 分析请求失败';
  } catch {
    return 'AI 分析请求失败';
  }
}
