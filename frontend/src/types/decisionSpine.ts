import type { Market } from './index';

export type UserThesisStance = 'long' | 'short' | 'watch' | 'pass' | 'insufficient';

export interface CandidateSnapshotInput {
  symbol: string;
  market: Market;
  screen_name: string;
  screen_definition_id?: string | null;
  inclusion_reasons: string[];
  exclusion_risks: string[];
  context_snapshot: Record<string, unknown>;
  data_as_of?: string | null;
}

export interface CandidateSnapshot extends CandidateSnapshotInput {
  id: string;
  created_at: string;
}

export interface ResearchCaseInput {
  candidate_snapshot_id: string;
  symbol: string;
  market: Market;
  analysis_run_id?: string | null;
  evidence_snapshot: Record<string, unknown>;
}

export interface ResearchCase extends ResearchCaseInput {
  id: string;
  created_at: string;
  updated_at: string;
}

export interface UserThesisInput {
  stance: UserThesisStance;
  summary: string;
  accepted_arguments: string[];
  rejected_arguments: string[];
  unresolved_questions: string[];
  falsifiers: string[];
}

export interface UserThesis extends UserThesisInput {
  id: string;
  created_at: string;
  research_case_id: string;
  parent_id: string | null;
  symbol: string;
  version: number;
}
