import { request } from './api';
import type { TodayBrief } from '../types/today';

export function fetchTodayBrief(market: 'US' | 'CN') {
  return request<TodayBrief>('/today?market=' + market);
}
