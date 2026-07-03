import { useCallback } from 'react';
import { API_BASE_URL } from '../lib/api';
import { useAnalysisStore } from '../stores/analysisStore';
import type { AnalysisConfig } from '../types/analysis';

interface StreamConnection {
  abortController: AbortController;
}

interface ParsedSSEEvent {
  event: string;
  data: string;
}

const connections = new Map<string, StreamConnection>();

function normalizeSymbol(symbol: string) {
  return symbol.trim().toUpperCase();
}

function parseJSON<T>(value: string, fallback: T): T {
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

function parseSSEBlock(block: string): ParsedSSEEvent | null {
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

function closeConnection(symbol: string) {
  const normalized = normalizeSymbol(symbol);
  const connection = connections.get(normalized);
  if (!connection) return;
  connection.abortController.abort();
  connections.delete(normalized);
}

function handleStreamEvent(symbol: string, parsed: ParsedSSEEvent) {
  const store = useAnalysisStore.getState();

  if (parsed.event === 'heartbeat') return;

  if (parsed.event === 'narration_delta') {
    const payload = parseJSON<{ agent: string; content: string }>(parsed.data, { agent: 'AnalysisAgent', content: '' });
    if (payload.content) store.appendLiveNarration(symbol, payload.content);
    return;
  }

  if (parsed.event === 'report_start') {
    store.clearLiveNarration(symbol);
    return;
  }

  if (parsed.event === 'report_delta') {
    const payload = parseJSON<{ content: string }>(parsed.data, { content: '' });
    if (payload.content) store.appendReport(symbol, payload.content);
    return;
  }

  if (parsed.event === 'progress') {
    const payload = parseJSON<{ agent: string; status: 'pending' | 'running' | 'done' | 'error'; message: string }>(parsed.data, {
      agent: 'AnalysisAgent',
      status: 'running',
      message: ''
    });
    store.upsertAIProgress(symbol, {
      agent: payload.agent || 'AnalysisAgent',
      status: payload.status || 'running',
      message: payload.message || ''
    });
    return;
  }

  if (parsed.event === 'result') {
    const payload = parseJSON<{ agent: string; content: string }>(parsed.data, { agent: 'AnalysisAgent', content: '' });
    if (!payload.content) return;
    store.appendAIResult(symbol, {
      agent: payload.agent || 'AnalysisAgent',
      content: payload.content
    });
    return;
  }

  if (parsed.event === 'decision') {
    const payload = parseJSON<{ action: string; confidence: string; summary: string }>(parsed.data, {
      action: 'HOLD',
      confidence: 'LOW',
      summary: ''
    });
    store.setAIDecision(symbol, payload);
    return;
  }

  if (parsed.event === 'thought') {
    const payload = parseJSON<{ agent: string; content: string }>(parsed.data, { agent: '分析助手', content: '' });
    if (!payload.content) return;
    store.appendThought(symbol, {
      agent: payload.agent || '分析助手',
      content: payload.content,
      timestamp: Date.now()
    });
    return;
  }

  if (parsed.event === 'report') {
    const payload = parseJSON<{ content: string }>(parsed.data, { content: '' });
    if (payload.content) store.appendReport(symbol, payload.content);
    return;
  }

  if (parsed.event === 'done') {
    store.markDone(symbol);
    closeConnection(symbol);
    return;
  }

  if (parsed.event === 'error') {
    const payload = parseJSON<{ message: string }>(parsed.data, { message: '分析中断，请检查 AI 服务配置后重试' });
    store.markAIError(symbol, payload.message);
    closeConnection(symbol);
  }
}

async function readErrorMessage(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string; message?: string };
    return payload.error || payload.message || 'AI 分析请求失败';
  } catch {
    return 'AI 分析请求失败';
  }
}

async function connect(symbol: string, config: AnalysisConfig) {
  const normalized = normalizeSymbol(symbol);
  closeConnection(normalized);

  const abortController = new AbortController();
  connections.set(normalized, { abortController });
  const store = useAnalysisStore.getState();
  store.resetAI(normalized);

  try {
    const response = await fetch(API_BASE_URL + '/analysis/' + encodeURIComponent(normalized) + '/ai', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
      signal: abortController.signal
    });

    if (!response.ok) {
      throw new Error(await readErrorMessage(response));
    }
    if (!response.body) {
      throw new Error('浏览器不支持流式读取响应');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split(/\r?\n\r?\n/);
      buffer = parts.pop() ?? '';
      for (const part of parts) {
        const parsed = parseSSEBlock(part);
        if (parsed) handleStreamEvent(normalized, parsed);
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      const parsed = parseSSEBlock(buffer);
      if (parsed) handleStreamEvent(normalized, parsed);
    }

    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.aiStatus === 'streaming') {
      store.markDone(normalized);
    }
  } catch (error) {
    if (abortController.signal.aborted) return;
    store.markAIError(normalized, error instanceof Error ? error.message : '分析中断，请检查网络连接后重新生成');
  } finally {
    const active = connections.get(normalized);
    if (active?.abortController === abortController) {
      connections.delete(normalized);
    }
  }
}

export function closeAIStream(symbol: string) {
  closeConnection(symbol);
}

export function useAIStream(symbol: string) {
  const normalized = normalizeSymbol(symbol);

  const start = useCallback(
    (config: AnalysisConfig) => {
      void connect(normalized, config);
    },
    [normalized]
  );

  const abort = useCallback(() => {
    closeConnection(normalized);
    const state = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (state.aiStatus === 'streaming') {
      useAnalysisStore.getState().updateState(normalized, { aiStatus: 'idle' });
    }
  }, [normalized]);

  return { start, abort };
}
