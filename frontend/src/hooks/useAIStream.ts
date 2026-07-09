import { useCallback } from 'react';
import {
  aiAnalysisStreamURL,
  buildAIAnalysisStartRequest,
  parseAIAnalysisSSEBlock,
  readAIAnalysisErrorMessage,
  type ParsedAIAnalysisStreamEvent
} from '../lib/aiAnalysisGateway';
import { useAnalysisStore } from '../stores/analysisStore';
import type { AIAnalysisContext, AnalysisConfig } from '../types/analysis';

interface StreamConnection {
  abortController: AbortController;
}

interface StreamTextBuffer {
  narrationByAgent: Record<string, string>;
  report: string;
  timer: ReturnType<typeof setTimeout> | null;
}

const connections = new Map<string, StreamConnection>();
const textBuffers = new Map<string, StreamTextBuffer>();
const STREAM_FLUSH_MS = 120;

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

function readableAIError(message: string) {
  if (message.includes("'NoneType' object has no attribute 'group'") || /AttributeError.+group/.test(message)) {
    return 'AI 模型或网关返回的结构化内容解析失败，请换用直连模型或更稳定的轻量模型后重试。';
  }
  if (message.includes('RestrictedModelsError') || message.includes('Free tier users do not have access')) {
    return '当前 AI Gateway 免费层无法访问所选模型，请换用免费层可用模型或充值后重试。';
  }
  return message || '分析中断，请检查 AI 服务配置后重试。';
}

function getTextBuffer(symbol: string) {
  const normalized = normalizeSymbol(symbol);
  let buffer = textBuffers.get(normalized);
  if (!buffer) {
    buffer = { narrationByAgent: {}, report: '', timer: null };
    textBuffers.set(normalized, buffer);
  }
  return buffer;
}

function agentForNarration(agent: string) {
  const normalizedAgent = agent.trim();
  if (normalizedAgent && normalizedAgent !== 'Model' && normalizedAgent !== 'AnalysisAgent') {
    return normalizedAgent;
  }
  return 'AnalysisAgent';
}

function flushTextBuffer(symbol: string) {
  const normalized = normalizeSymbol(symbol);
  const buffer = textBuffers.get(normalized);
  if (!buffer) return;
  if (buffer.timer) {
    clearTimeout(buffer.timer);
    buffer.timer = null;
  }
  const narrationByAgent = buffer.narrationByAgent;
  const report = buffer.report;
  buffer.narrationByAgent = {};
  buffer.report = '';
  if (Object.keys(narrationByAgent).length === 0 && !report) return;

  const store = useAnalysisStore.getState();
  for (const [agent, content] of Object.entries(narrationByAgent)) {
    if (!content) continue;
    store.appendLiveNarration(normalized, content);
    store.appendAgentNarration(normalized, agent, content);
  }
  if (report) store.appendReport(normalized, report);
}

function clearTextBuffer(symbol: string) {
  const normalized = normalizeSymbol(symbol);
  const buffer = textBuffers.get(normalized);
  if (buffer?.timer) clearTimeout(buffer.timer);
  textBuffers.delete(normalized);
}

function scheduleTextFlush(symbol: string) {
  const buffer = getTextBuffer(symbol);
  if (buffer.timer) return;
  buffer.timer = setTimeout(() => flushTextBuffer(symbol), STREAM_FLUSH_MS);
}

function appendBufferedText(symbol: string, key: 'narration' | 'report', content: string, agent = 'AnalysisAgent') {
  if (!content) return;
  const buffer = getTextBuffer(symbol);
  if (key === 'narration') {
    const targetAgent = agentForNarration(agent);
    buffer.narrationByAgent[targetAgent] = (buffer.narrationByAgent[targetAgent] ?? '') + content;
  } else {
    buffer.report += content;
  }
  scheduleTextFlush(symbol);
}

function closeConnection(symbol: string) {
  const normalized = normalizeSymbol(symbol);
  const connection = connections.get(normalized);
  if (connection) {
    connection.abortController.abort();
    connections.delete(normalized);
  }
  clearTextBuffer(normalized);
}

function handleStreamEvent(symbol: string, parsed: ParsedAIAnalysisStreamEvent) {
  const store = useAnalysisStore.getState();

  if (parsed.event === 'heartbeat') return;

  if (parsed.event === 'narration_delta') {
    const payload = parseJSON<{ agent: string; content: string }>(parsed.data, { agent: 'AnalysisAgent', content: '' });
    appendBufferedText(symbol, 'narration', payload.content, payload.agent);
    return;
  }

  if (parsed.event === 'report_start') {
    flushTextBuffer(symbol);
    store.clearLiveNarration(symbol);
    return;
  }

  if (parsed.event === 'report_delta') {
    const payload = parseJSON<{ content: string }>(parsed.data, { content: '' });
    appendBufferedText(symbol, 'report', payload.content);
    return;
  }

  if (parsed.event === 'progress') {
    const payload = parseJSON<{ agent: string; status: 'pending' | 'running' | 'paused' | 'done' | 'error'; message: string }>(parsed.data, {
      agent: 'AnalysisAgent',
      status: 'running',
      message: ''
    });
    if (payload.status === 'done' || payload.status === 'error') {
      flushTextBuffer(symbol);
    }
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
    flushTextBuffer(symbol);
    store.markDone(symbol);
    closeConnection(symbol);
    return;
  }

  if (parsed.event === 'error') {
    const payload = parseJSON<{ message: string }>(parsed.data, { message: '分析中断，请检查 AI 服务配置后重试' });
    flushTextBuffer(symbol);
    store.markAIError(symbol, readableAIError(payload.message));
    closeConnection(symbol);
  }
}

async function connect(symbol: string, config: AnalysisConfig, context?: AIAnalysisContext) {
  const normalized = normalizeSymbol(symbol);
  closeConnection(normalized);

  const abortController = new AbortController();
  connections.set(normalized, { abortController });
  const store = useAnalysisStore.getState();
  clearTextBuffer(normalized);
  store.resetAI(normalized);

  try {
    const response = await fetch(aiAnalysisStreamURL(normalized), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildAIAnalysisStartRequest(config, context)),
      signal: abortController.signal
    });

    if (!response.ok) {
      throw new Error(await readAIAnalysisErrorMessage(response));
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
        const parsed = parseAIAnalysisSSEBlock(part);
        if (parsed) handleStreamEvent(normalized, parsed);
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      const parsed = parseAIAnalysisSSEBlock(buffer);
      if (parsed) handleStreamEvent(normalized, parsed);
    }

    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.aiStatus === 'streaming') {
      flushTextBuffer(normalized);
      store.markDone(normalized);
    }
  } catch (error) {
    if (abortController.signal.aborted) return;
    flushTextBuffer(normalized);
    const message = error instanceof Error ? error.message : '分析中断，请检查网络连接后重新生成';
    store.markAIError(normalized, readableAIError(message));
  } finally {
    const active = connections.get(normalized);
    if (active?.abortController === abortController) {
      connections.delete(normalized);
    }
  }
}

export function closeAIStream(symbol: string) {
  flushTextBuffer(symbol);
  closeConnection(symbol);
}

export function useAIStream(symbol: string) {
  const normalized = normalizeSymbol(symbol);

  const start = useCallback(
    (config: AnalysisConfig, context?: AIAnalysisContext) => {
      void connect(normalized, config, context);
    },
    [normalized]
  );

  const abort = useCallback(() => {
    flushTextBuffer(normalized);
    closeConnection(normalized);
    const state = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (state.aiStatus === 'streaming') {
      useAnalysisStore.getState().markPaused(normalized);
    }
  }, [normalized]);

  return { start, abort };
}
