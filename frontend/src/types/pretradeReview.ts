export type PreTradeSeverity = 'low' | 'medium' | 'high';
export type PreTradeAIStatus = 'not_requested' | 'completed' | 'unavailable' | 'failed';

export interface PreTradeFinding {
  code: string;
  severity: PreTradeSeverity;
  category: string;
  message: string;
  field?: string | null;
  evidence: Record<string, unknown>;
}

export interface PreTradeSource {
  id: string;
  label: string;
  url?: string | null;
  data_as_of?: string | null;
  status: string;
}

export interface RiskAcknowledgement {
  id: string;
  created_at: string;
  review_id: string;
  finding_code: string;
  reason: string | null;
}

export interface PreTradeRiskReview {
  id: string;
  created_at: string;
  trade_plan_id: string;
  version: number;
  plan_snapshot: Record<string, unknown>;
  hard_conflicts: PreTradeFinding[];
  risk_warnings: PreTradeFinding[];
  behavior_reminders: PreTradeFinding[];
  unresolved_questions: PreTradeFinding[];
  changes_since_research: PreTradeFinding[];
  freshness: Record<string, unknown>;
  sources: PreTradeSource[];
  ai_status: PreTradeAIStatus;
  ai_provider: string | null;
  ai_model: string | null;
  acknowledgements: RiskAcknowledgement[];
}

export interface CreatePreTradeReviewInput {
  run_ai: boolean;
  llm_provider?: string;
  model?: string;
  backend_url?: string;
  current_evidence?: Record<string, unknown>;
}

export interface AcknowledgePreTradeFindingInput {
  finding_code: string;
  reason?: string | null;
}
