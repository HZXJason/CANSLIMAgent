import { request } from './api';
import type { ResearchAssistantAnswer, ResearchAssistantInput, TodayBrief } from '../types/today';

export function fetchTodayBrief(market: 'US' | 'CN') {
  return request<TodayBrief>('/today?market=' + market);
}

export function askResearchAssistant(input: ResearchAssistantInput) {
  return request<ResearchAssistantAnswer>('/assistant/ask', { method: 'POST', body: JSON.stringify(input) });
}
