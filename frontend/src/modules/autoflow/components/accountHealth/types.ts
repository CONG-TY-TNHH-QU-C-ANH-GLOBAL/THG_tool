// API shapes for the Account Health Board — mirror of the backend
// models.AccountReadiness returned by GET /api/accounts/readiness (PR-D).
export interface CapabilityReadiness {
  capability: string; // crawl | comment | inbox | post
  can: boolean;
  reasons: string[]; // machine reason codes; UI translates via reasonMessages
}

export interface AccountReadiness {
  account_id: number;
  account_name: string;
  assigned_user_name: string;
  fb_user_id: string;
  fb_display_name: string;
  connector_id: number;
  machine_label: string;
  browser_profile_id: string;
  extension_version: string;
  capabilities: CapabilityReadiness[];
  required_action: string;
  // P1.3E requester-scoped executability. `executable` is the ONLY field that may drive the
  // green "Sẵn sàng" badge: true only when the current user can run automation on THIS account
  // right now via their OWN live, identity-matched connector. Optional for old-client safety.
  configured?: boolean;
  control_allowed?: boolean;
  paired?: boolean;
  connector_online?: boolean;
  heartbeat_fresh?: boolean;
  live_identity_matched?: boolean;
  session_usable?: boolean;
  executable?: boolean;
  exec_reason_code?: string; // ready | no_connector | connector_stale | pairing_pending | identity_mismatch | session_blocked | account_blocked | not_controllable
  exec_reason_message?: string;
}

// Customer-facing capability labels (never the raw key).
export const CAPABILITY_LABELS: Record<string, string> = {
  crawl: 'Tìm khách',
  comment: 'Bình luận',
  inbox: 'Nhắn tin',
  post: 'Đăng bài',
};
