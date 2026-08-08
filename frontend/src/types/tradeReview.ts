export type ProcessQuality = 'unreviewed' | 'followed' | 'adjusted' | 'deviated';
export type BehaviorObservationFeedback = 'accepted' | 'not_applicable' | 'needs_more_evidence';

export interface TradeExecutionDiff {
  plan_link_status: 'linked' | 'off_plan';
  symbol_match: boolean | null;
  direction_match: boolean | null;
  entry_slippage_percent: number | null;
  timing_status: 'unknown' | 'before_plan_date' | 'within_plan_window' | 'after_valid_until';
  known_deviations: string[];
  unknown_comparisons: string[];
}

export interface TradeReview {
  id: string;
  created_at: string;
  updated_at: string;
  journal_entry_id: string;
  version: number;
  trade_plan_id: string | null;
  plan_snapshot: Record<string, unknown>;
  execution_snapshot: Record<string, unknown>;
  diff: TradeExecutionDiff;
  process_quality: ProcessQuality;
  deviation_reason: string | null;
  emotion_before: string | null;
  emotion_after: string | null;
  outcome_notes: string | null;
  lesson: string | null;
  completed_at: string | null;
}

export interface UpdateTradeReviewInput {
  process_quality?: ProcessQuality;
  deviation_reason?: string | null;
  emotion_before?: string | null;
  emotion_after?: string | null;
  outcome_notes?: string | null;
  lesson?: string | null;
  complete?: boolean;
}

export interface BehaviorObservation {
  id: string;
  created_at: string;
  updated_at: string;
  key: string;
  scenario: string;
  status: 'insufficient' | 'eligible';
  sample_count: number;
  confidence: number;
  message: string;
  evidence_review_ids: string[];
  feedback: BehaviorObservationFeedback | null;
  feedback_at: string | null;
  user_correction?: string | null;
  ai_enabled?: boolean;
}
