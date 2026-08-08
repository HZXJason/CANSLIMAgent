import type {
  CandidateSnapshot,
  CandidateSnapshotInput,
  ResearchCase,
  ResearchCaseInput,
  UserThesis,
  UserThesisInput
} from '../types/decisionSpine';
import { request } from './api';

const root = '/decision-spine';

export function createCandidateSnapshot(input: CandidateSnapshotInput) {
  return request<CandidateSnapshot>(root + '/candidates', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function fetchCandidateSnapshot(id: string) {
  return request<CandidateSnapshot>(root + '/candidates/' + encodeURIComponent(id));
}

export function createResearchCase(input: ResearchCaseInput) {
  return request<ResearchCase>(root + '/research-cases', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function fetchResearchCase(id: string) {
  return request<ResearchCase>(root + '/research-cases/' + encodeURIComponent(id));
}

export function createUserThesis(researchCaseId: string, input: UserThesisInput) {
  return request<UserThesis>(root + '/research-cases/' + encodeURIComponent(researchCaseId) + '/theses', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function reviseUserThesis(id: string, input: UserThesisInput) {
  return request<UserThesis>(root + '/theses/' + encodeURIComponent(id) + '/revise', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}

export function fetchResearchTheses(researchCaseId: string) {
  return request<UserThesis[]>(root + '/research-cases/' + encodeURIComponent(researchCaseId) + '/theses');
}
