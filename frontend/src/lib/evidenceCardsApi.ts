import { request } from './api';
import type { EvidenceCardsPack } from '../types/evidenceCards';

export function compileEvidenceCards(
  symbol: string,
  input: { include_ai?: boolean; llm_provider?: string; model?: string; backend_url?: string } = {}
) {
  return request<EvidenceCardsPack>('/stocks/' + encodeURIComponent(symbol) + '/evidence-cards', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}
