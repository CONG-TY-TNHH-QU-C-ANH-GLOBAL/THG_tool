/**
 * Step 4a — Verified Execution Observability service.
 *
 * Reads the three read-only endpoints under /api/observability/execution/*.
 * Mirrors the backend wire shapes (snake_case) into TS interfaces without
 * remapping field names so the dashboard renders straight off the API
 * payload and any future field addition is visible without TS churn.
 *
 * NO write methods live here. This module is purely "observe reality."
 */

import { get } from './api';

// One bucket of the outcome × action_type distribution matrix.
export interface OutcomeBucket {
  outcome: string;
  action_type: string;
  count: number;
}

export interface DistributionResponse {
  window_hours: number;
  since: string;
  buckets: OutcomeBucket[];
  total: number;
}

// Evidence is whatever VerificationEvidence JSON the verifier emitted —
// schema may evolve so we type it as an open record. Common keys today:
// comment_permalink, message_bubble_id, dom_snippet, page_url_after,
// observed_at, notes, screenshot_path.
export type Evidence = Record<string, unknown>;

export interface ExecutionAttemptRow {
  id: number;
  action_ledger_id: number;
  outbound_id: number;
  org_id: number;
  account_id: number;
  target_url: string;
  action_type: string;
  attempt: number;
  status: string;
  outcome: string;
  failure_reason: string;
  evidence?: Evidence;
  dom_verified: boolean;
  started_at: string;
  finished_at?: string;
}

export interface RecentResponse {
  window_hours: number;
  since: string;
  attempts: ExecutionAttemptRow[];
  count: number;
}

export interface AccountHealthRow {
  account_id: number;
  trust_level: string;
  risk_score: number;
  recent_failures: number;
  cooldown_until: string;
  last_action_at: string;
  comments_today: number;
  inbox_today: number;
}

export interface AccountHealthResponse {
  accounts: AccountHealthRow[];
  count: number;
}

export async function getOutcomeDistribution(hours = 24): Promise<DistributionResponse> {
  return get<DistributionResponse>(`/observability/execution/distribution?hours=${hours}`);
}

export async function getRecentExecutionAttempts(hours = 24, limit = 100): Promise<RecentResponse> {
  return get<RecentResponse>(`/observability/execution/recent?hours=${hours}&limit=${limit}`);
}

export async function getAccountHealth(accountId?: number): Promise<AccountHealthResponse> {
  const q = accountId && accountId > 0 ? `?account_id=${accountId}` : '';
  return get<AccountHealthResponse>(`/observability/execution/account-health${q}`);
}

// ── PR-E: stuck-state observation surfaces ──────────────────────────────────

// One outbound row stuck in planned/executing with no execution_attempts.
export interface GapRow {
  outbound_id: number;
  org_id: number;
  account_id: number;
  action_type: string;
  target_url: string;
  execution_state: string;
  created_at: string;
  lease_expiry?: string;
  age_seconds: number;
}
export interface GapResponse {
  older_than_minutes: number;
  threshold: string;
  rows: GapRow[];
  count: number;
}

// One (hour, outcome) point on per-account outcome timeseries.
export interface TimeseriesPoint {
  bucket: string;
  outcome: string;
  count: number;
}
export interface TimeseriesResponse {
  account_id: number;
  window_hours: number;
  since: string;
  buckets: TimeseriesPoint[];
  count: number;
}

// One ledger row whose outcome disagrees with the latest attempt outcome.
export interface ReconcileRow {
  ledger_id: number;
  org_id: number;
  account_id: number;
  outbound_id: number;
  action_type: string;
  target_url: string;
  performed_at: string;
  ledger_outcome: string;
  attempt_outcome: string;
}
export interface ReconcileResponse {
  window_hours: number;
  since: string;
  rows: ReconcileRow[];
  count: number;
}

export async function getStuckOutbound(olderThanMinutes = 10, limit = 50): Promise<GapResponse> {
  return get<GapResponse>(`/observability/execution/gap-detection?older_than_minutes=${olderThanMinutes}&limit=${limit}`);
}

export async function getAccountTimeseries(accountId: number, hours = 72): Promise<TimeseriesResponse> {
  return get<TimeseriesResponse>(`/observability/execution/account-timeseries?account_id=${accountId}&hours=${hours}`);
}

export async function getLedgerReconcile(hours = 24, limit = 100): Promise<ReconcileResponse> {
  return get<ReconcileResponse>(`/observability/execution/ledger-reconcile?hours=${hours}&limit=${limit}`);
}
