// Lead Lifecycle types (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-4). Mirrors the Go
// projection models.LeadLifecycleState — the backend owns the derivation; the frontend
// only groups + renders by it (no client-side lifecycle logic, to avoid Go/TS drift).

export type LeadFreshnessState =
  | 'active'        // Cần xử lý — fresh / customer replied
  | 'waiting_reply' // Chờ phản hồi — we touched, awaiting reply
  | 'followup_due'  // Đến hạn follow-up — wait window elapsed
  | 'stale'         // cold; hidden by default
  | 'archived';     // Đã lưu trữ

export type LeadNextAction =
  | 'comment' | 'reply' | 'wait' | 'followup' | 'archive' | 'none';

export interface LeadLifecycleState {
  lead_id: number;
  freshness_state: LeadFreshnessState;
  next_action: LeadNextAction;
  next_action_at: string;
  last_seen_at: string;
  last_crawled_at: string;
  last_engaged_at: string;
  last_customer_reply_at: string;
  archived_at: string;
  archive_reason: string;
}

// The four dashboard tabs. stale is intentionally not a tab — hidden by default.
export type LifecycleTab = 'active' | 'waiting_reply' | 'followup_due' | 'archived';

export const LIFECYCLE_TABS: LifecycleTab[] = ['active', 'waiting_reply', 'followup_due', 'archived'];
