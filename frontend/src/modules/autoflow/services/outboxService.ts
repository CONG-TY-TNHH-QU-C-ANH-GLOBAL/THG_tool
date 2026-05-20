import { del, get, put } from './api';

// Outbound row lifecycle (PR-1 verified-state-centric refactor, May-2026).
//
// Two orthogonal dimensions:
//
//   execution_state       — transport lifecycle:
//                           'planned' | 'executing' | 'finished' | 'expired'
//
//   verification_outcome  — post-DOM observation (NULL until finished):
//                           'verified_success' | 'context_drift' | 'rate_limited'
//                           | 'blocked' | 'captcha' | 'shadow_rejected'
//                           | 'execution_failed'
//
// Legacy `status` field is the back-compat wire value derived from the
// pair. New UI code should consume execution_state + verification_outcome.
export type ExecutionState = 'planned' | 'executing' | 'finished' | 'expired';
export type VerificationOutcome =
  | 'verified_success'
  | 'context_drift'
  | 'rate_limited'
  | 'blocked'
  | 'captcha'
  | 'shadow_rejected'
  | 'execution_failed'
  | '';

export interface OutboundMessage {
  id: number;
  type: string;           // 'comment' | 'inbox' | 'group_post' | 'profile_post'
  platform: string;
  account_id: number;
  target_url: string;
  target_name: string;
  content: string;
  context: string;
  image_path: string;
  status: string;         // LEGACY — 'approved' | 'sending' | 'sent' | 'failed' (derived)
  execution_state: ExecutionState;
  verification_outcome?: VerificationOutcome;
  ai_model: string;
  sent_at: string;
  created_at: string;
}

export interface OutboxResponse {
  messages: OutboundMessage[];
  count: number;
  counts: Record<string, number>;
}

export async function getOutbox(params?: { type?: string; status?: string; limit?: number }): Promise<OutboxResponse> {
  const q = new URLSearchParams();
  if (params?.type) q.set('type', params.type);
  if (params?.status) q.set('status', params.status);
  if (params?.limit) q.set('limit', String(params.limit));
  const qs = q.toString();
  return get<OutboxResponse>('/outbox' + (qs ? '?' + qs : ''));
}

// approveOutbox / rejectOutbox were removed in the autonomous-first
// refactor (May-2026). The system no longer has a human-approval gate
// — every queued outbound is planned and executes when an account is
// available. UI code that previously presented approve/reject buttons
// now only offers delete + open-target.

export async function updateOutboxContent(id: number, content: string): Promise<void> {
  await put(`/outbox/${id}/content`, { content });
}

export async function deleteOutbox(id: number): Promise<void> {
  await del(`/outbox/${id}`);
}

// deleteAllOutboundComments clears every comment outbox row for the org.
// Admin-only on the backend (DELETE /outbox/comments/all).
export async function deleteAllOutboundComments(): Promise<{ deleted: number }> {
  return del<{ deleted: number }>('/outbox/comments/all');
}

// deleteAllOutboundPosts clears every group_post + profile_post outbox row
// for the org. Admin-only on the backend (DELETE /outbox/posts/all).
export async function deleteAllOutboundPosts(): Promise<{ deleted: number }> {
  return del<{ deleted: number }>('/outbox/posts/all');
}
