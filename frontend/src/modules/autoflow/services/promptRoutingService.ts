/**
 * Watchpoint B — Prompt Routing Observability service.
 *
 * Four read-only endpoints under /api/observability/prompt-routing/*.
 * Backend wire shape is snake_case; preserved here so the dashboard
 * renders straight off the payload and future fields surface without
 * TS churn.
 */

import { get } from './api';

export interface PromptRoutingBucket {
  route: string;
  reason_code: string;
  action: string;
  count: number;
}

export interface DistributionResponse {
  window_hours: number;
  since: string;
  buckets: PromptRoutingBucket[];
  total: number;
}

export interface PromptRoutingRow {
  id: number;
  org_id: number;
  account_id: number;
  source: string;
  user_prompt: string;
  ai_response: string;
  action_taken: string;
  success: boolean;
  created_at: string;
  route: string;
  reason_code: string;
  reason?: string;
  sufficiency_score: number;
  missing_signals?: string[];
  inferred_signals?: string[];
  decision_raw?: Record<string, unknown>;
}

export interface RecentResponse {
  window_hours: number;
  since: string;
  rows: PromptRoutingRow[];
  count: number;
}

export interface ConflictRow {
  kind: 'false_positive_deterministic' | 'false_negative_deterministic';
  row: PromptRoutingRow;
  follow_up_prompt?: string;
  follow_up_at_rel?: string;
}

export interface ConflictsResponse {
  window_hours: number;
  since: string;
  false_positive_count: number;
  false_negative_count: number;
  false_positive_examples: ConflictRow[];
  false_negative_examples: ConflictRow[];
}

export interface MissingSignalBucket {
  signal: string;
  count: number;
}

export interface MissingSignalsResponse {
  window_hours: number;
  since: string;
  buckets: MissingSignalBucket[];
}

export async function getPromptRoutingDistribution(hours = 24): Promise<DistributionResponse> {
  return get<DistributionResponse>(`/observability/prompt-routing/distribution?hours=${hours}`);
}

export async function getPromptRoutingRecent(hours = 24, limit = 100): Promise<RecentResponse> {
  return get<RecentResponse>(`/observability/prompt-routing/recent?hours=${hours}&limit=${limit}`);
}

export async function getPromptRoutingConflicts(hours = 24): Promise<ConflictsResponse> {
  return get<ConflictsResponse>(`/observability/prompt-routing/conflicts?hours=${hours}`);
}

export async function getPromptRoutingMissingSignals(hours = 24): Promise<MissingSignalsResponse> {
  return get<MissingSignalsResponse>(`/observability/prompt-routing/missing-signals?hours=${hours}`);
}
