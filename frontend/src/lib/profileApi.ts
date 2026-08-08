import { request } from './api';
import type { AIMemoryPreference, PlaybookInput, PlaybookSuggestion, PrincipleInput, ProfileDashboard, ProfileRange, TradingPlaybook, TradingPrinciple, TradingProfile, TradingProfileInput } from '../types/profile';

export const fetchTradingProfile = () => request<TradingProfile>('/profile');
export const saveTradingProfile = (input: TradingProfileInput) => request<TradingProfile>('/profile', { method: 'PUT', body: JSON.stringify(input) });
export const fetchPrinciples = () => request<TradingPrinciple[]>('/profile/principles');
export const createPrinciple = (input: PrincipleInput) => request<TradingPrinciple>('/profile/principles', { method: 'POST', body: JSON.stringify(input) });
export const patchPrinciple = (id: string, input: Partial<PrincipleInput>) => request<TradingPrinciple>('/profile/principles/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify(input) });
export const fetchPlaybooks = (includeArchived = true) => request<TradingPlaybook[]>('/profile/playbooks?include_archived=' + String(includeArchived));
export const createPlaybook = (input: PlaybookInput) => request<TradingPlaybook>('/profile/playbooks', { method: 'POST', body: JSON.stringify(input) });
export const patchPlaybook = (id: string, input: Partial<PlaybookInput>) => request<TradingPlaybook>('/profile/playbooks/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify(input) });
export const fetchProfileDashboard = (range: ProfileRange) => request<ProfileDashboard>('/profile/dashboard?range=' + range);
export const fetchAIMemory = () => request<AIMemoryPreference>('/profile/ai-memory');
export const patchAIMemory = (input: Partial<Pick<AIMemoryPreference, 'principles_enabled' | 'observations_enabled' | 'playbooks_enabled'>>) => request<AIMemoryPreference>('/profile/ai-memory', { method: 'PATCH', body: JSON.stringify(input) });
export const patchProfileObservation = (id: string, input: { user_correction?: string | null; ai_enabled?: boolean }) => request('/profile/behavior-observations/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify(input) });
export const suggestPlaybooks = (input: { llm_provider: string; model: string; backend_url: string }) => request<{ status: string; suggestions: PlaybookSuggestion[] }>('/profile/playbook-suggestions', { method: 'POST', body: JSON.stringify(input) });
export const exportTradingSystem = () => request<Record<string, unknown>>('/profile/export');
export const exportJournalRecords = () => request<Record<string, unknown>>('/journal/export');
