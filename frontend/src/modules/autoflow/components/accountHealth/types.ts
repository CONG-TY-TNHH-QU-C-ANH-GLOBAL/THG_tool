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
  fb_user_id: string;
  fb_display_name: string;
  connector_id: number;
  machine_label: string;
  browser_profile_id: string;
  extension_version: string;
  capabilities: CapabilityReadiness[];
  required_action: string;
}

// Customer-facing capability labels (never the raw key).
export const CAPABILITY_LABELS: Record<string, string> = {
  crawl: 'Tìm khách',
  comment: 'Bình luận',
  inbox: 'Nhắn tin',
  post: 'Đăng bài',
};
