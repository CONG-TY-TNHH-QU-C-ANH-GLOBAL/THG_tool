import { del, get, post, put } from './api';

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

// ActorIdentity is the read-only Facebook identity of the account that
// executes an outbound action — the "đăng bởi" actor, distinct from the
// initiating staff/system principal (created_by). Keyed by account_id in
// OutboxResponse.actors. See specs/COMMENT_INTELLIGENCE_PIPELINE.md §7a.
export interface ActorIdentity {
  account_id: number;
  account_name: string;
  fb_user_id: string;
  fb_display_name: string;
  fb_username: string;
  fb_profile_url: string;
  // Verified-Actor state (P1b): last verdict for the account and whether it is
  // blocked from auto-execute on an actor mismatch. See pipeline doc §7b.
  actor_verdict?: 'verified' | 'mismatch' | 'unknown' | '';
  actor_blocked?: boolean;
}

export interface OutboxResponse {
  messages: OutboundMessage[];
  count: number;
  counts: Record<string, number>;
  // actors maps account_id → ActorIdentity for every account in the org,
  // so the UI can render the executing Facebook actor without an N+1 fetch.
  actors?: Record<string, ActorIdentity>;
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

// clearActorBlock lifts a Verified-Actor block on an account (P1b) so it can
// auto-execute again. Admin-only on the backend
// (POST /accounts/:id/clear-actor-block). Call after the operator has confirmed
// the correct Facebook identity is logged in on that account.
export async function clearActorBlock(accountId: number): Promise<{ cleared: boolean; account_id: number }> {
  return post<{ cleared: boolean; account_id: number }>(`/accounts/${accountId}/clear-actor-block`, {});
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
