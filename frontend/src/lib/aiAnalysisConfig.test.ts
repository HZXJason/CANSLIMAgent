// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import type { AnalysisConfig } from '../types/analysis';
import { AI_ANALYSIS_CONFIG_STORAGE_KEY, writeStoredAIAnalysisConfig } from './aiAnalysisConfig';

describe('AI analysis config storage', () => {
  it('never persists legacy browser API keys', () => {
    const config = {
      llm_provider: 'openai',
      deep_think_llm: 'gpt-5.4',
      quick_think_llm: 'gpt-5.4-mini',
      backend_url: '',
      max_debate_rounds: 1,
      max_risk_rounds: 1,
      online_tools: true,
      analysts: ['fundamental'],
      api_keys: { openai: 'sk-browser-secret' }
    } as AnalysisConfig & { api_keys: Record<string, string> };

    writeStoredAIAnalysisConfig(config);

    const stored = window.localStorage.getItem(AI_ANALYSIS_CONFIG_STORAGE_KEY) ?? '';
    expect(stored).not.toContain('api_keys');
    expect(stored).not.toContain('sk-browser-secret');
  });
});
