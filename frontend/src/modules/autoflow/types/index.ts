export * from './lifecycle';
import type { LeadLifecycleState } from './lifecycle';

export type Role = 'founder' | 'superadmin' | 'admin' | 'staff' | 'sales';
export type PlanTier = 'Starter' | 'Pro' | 'Enterprise';
export type LeadStatus = 'Hot' | 'Warm' | 'Cold';
export type ThreadStatus = 'Active' | 'Converted' | 'Pending';
export type PostStatus = 'Live' | 'Ended';
export type MemberStatus = 'Active' | 'Suspended';

export interface Organization {
  id: number;
  name: string;
  abbr: string;
  plan: PlanTier;
  color: string;
  logo_url?: string;
  avatar_url?: string;
}

// Thread role — the participant's structural position in the FB thread.
// Orthogonal to status (Hot/Warm/Cold). See project_thread_role_architecture.md.
//   intent_originator / buyer_responder = an actual lead
//   supplier_responder / competitor / noise = not a lead
export type LeadThreadRole =
  | 'intent_originator'
  | 'supplier_responder'
  | 'buyer_responder'
  | 'competitor'
  | 'noise';

export interface Lead {
  id: number;
  name: string;
  status: LeadStatus;
  group: string;
  agent: string;
  last: string;
  score: number;
  phone: string;
  // The three distinct URLs — never collapse them (see thread-role memory):
  //   postUrl            = canonical battlefield (the post everyone discusses)
  //   facebookUrl        = actor profile URL (the participant's FB profile)
  //   engagementPermalink= exact comment permalink (set for comment-sourced leads)
  facebookUrl?: string;
  postUrl?: string;
  engagementPermalink?: string;
  sourceType?: string;
  threadRole?: LeadThreadRole;
  engagement?: LeadEngagementState;
  lifecycle?: LeadLifecycleState;
}

// Lead Engagement = derived coordination state from the Action Ledger.
// NOT a CRM status. NOT a manual field. See feedback_battlefield_badge_framing.md.
export type LeadEngagementBadge =
  | 'priority'           // untouched battlefield
  | 'protected'          // active engagement in flight (last <15min)
  | 'followup_pending'   // conversation alive, original engager still natural owner
  | 'visible'            // previously touched, no immediate claim
  | 'closed';            // terminal — converted / closed thread

export interface LeadEngagementEntry {
  user_id: number;
  user_name: string;
  account_id: number;
  account_name: string;
  fb_display_name?: string;
  fb_profile_url?: string;
  actor_verdict?: string;   // verified | mismatch | unknown | ''
  actor_blocked?: boolean;
  channel?: string;         // 'facebook'
  action: string;           // comment | inbox | group_post | profile_post
  target_url: string;
  outcome: string;
  performed_at: string;     // ISO-ish from backend
}

// CommentEligibility (§6) — explains, with the SAME gates as comment_all_leads,
// whether a lead can be commented now. Counts + reason codes only, no secrets.
export interface CommentEligibility {
  next_comment_eligible: boolean;
  eligibility_state: string;        // 'eligible' | 'no_ready_account' | 'coverage_full' | 'already_commented_by_this_actor' | ...
  ineligibility_reason: string;
  ineligibility_message_vi: string;
  commented_by_count: number;
  max_coverage: number;
  candidate_actor_count: number;
  ready_actor_count: number;
  eligible_actor_count: number;
  last_comment_attempt_status: string;
  last_comment_attempt_reason: string;
}

export interface LeadEngagementState {
  lead_id: number;
  badge: LeadEngagementBadge;
  entries: LeadEngagementEntry[];
  last_engaged_at: string;
  last_engaged_by: string;
  last_engaged_action: string;
  thread_status: string;
  eligibility?: CommentEligibility;
}

export interface Thread {
  id: number;
  lead: string;
  agent: string;
  last: string;
  time: string;
  status: ThreadStatus;
  unread: number;
}

export interface Message {
  from: 'lead' | 'agent' | 'user' | 'assistant' | 'system';
  text: string;
  time: string;
}

export interface Post {
  id: number;
  group: string;
  content: string;
  time: string;
  likes: number;
  comments: number;
  shares: number;
  status: PostStatus;
}

export interface Comment {
  id: number;
  agent: string;
  lead: string;
  post: string;
  comment: string;
  time: string;
}

export interface StaffMember {
  id: number;
  orgId: number;
  name: string;
  email: string;
  role: string;
  status: MemberStatus;
  joined: string;
  online: boolean; // PR-M5: has a live extension connector right now
  convs: number;
  converted: number;
  cmts: number;
}

export interface StaffInvite {
  id: number;
  email: string;
  role: string;
  token: string;
  inviteUrl: string;
  inviteFullUrl?: string;
  createdBy: number;
  expiresAt: string;
  createdAt: string;
  emailStatus?: string;
  emailError?: string;
  /** Lifecycle: pending | accepted | expired | revoked (backend-derived). */
  status?: string;
}

export interface FileRecord {
  id: number;
  name: string;
  size: string;
  sizeBytes: number;
  date: string;
}

export type DataSourceType = 'google_sheet' | 'google_drive';
export type DataSourceStatus = 'pending' | 'synced' | 'error' | 'needs_auth';

export interface DataSource {
  id: number;
  org_id: number;
  type: DataSourceType;
  name: string;
  source_url: string;
  status: DataSourceStatus;
  item_count: number;
  summary: string;
  metadata_json: string;
  last_error: string;
  last_sync_at?: string;
  created_at: string;
  updated_at: string;
}

export interface KPIConfig {
  conv: number;
  conv2: number;
  cmt: number;
  bonus: number;
  bonusAmt: number;
  pen: number;
  penAmt: number;
}

export interface ScoredStaff extends StaffMember {
  pts: number;
}

export interface FacebookStatus {
  connected: boolean;
  account?: string;
  expiresLabel?: string;
  groups?: number;
  leadsToday?: number;
  agents?: number;
}

export interface Workspace {
  accountId: number;
  accountName: string;
  email?: string;
  accountStatus: string;
  loggedIn: boolean;
  fbUserId?: string;
  fbDisplayName?: string;
  fbUsername?: string;
  fbProfileUrl?: string;
  running: boolean;
  browserState?: string;
  errorMsg?: string;
  vncPort?: number;
  cdpPort?: number;
  startedAt?: string;
}

export interface LocalConnector {
  id: number;
  orgId: number;
  name: string;
  createdBy: number;
  hostname: string;
  os: string;
  version: string;
  kind: string;
  transport: string;
  assignedAccountId: number;
  capabilitiesJson: string;
  currentUrl: string;
  fbUserId: string;
  fbDisplayName: string;
  fbUsername: string;
  fbProfileUrl: string;
  streamStatus: string;
  chromeError: string;
  lastSeen?: string;
  online: boolean;
  active: boolean;
  createdAt: string;
}

export interface LocalConnectorScreen {
  accountId: number;
  orgId: number;
  agentId: number;
  imageData: string;
  currentUrl: string;
  fbUserId: string;
  fbDisplayName: string;
  fbUsername: string;
  fbProfileUrl: string;
  streamStatus: string;
  chromeError: string;
  updatedAt: string;
  actions?: LocalConnectorAction[];
}

export interface LocalConnectorAction {
  id: number;
  accountId: number;
  agentId: number;
  type: string;
  status: string;
  errorMsg: string;
  createdAt: string;
  claimedAt?: string;
  completedAt?: string;
}

export interface WorkspaceSessionSnapshot {
  accountId: number;
  accountName: string;
  loggedIn: boolean;
  fbUserId?: string;
  storedFbUserId?: string;
  currentUrl?: string;
  currentTitle?: string;
  checkpoint: boolean;
  humanRequired?: boolean;
  humanReason?: string;
  cookieError?: string;
}

export interface OrgSummary {
  id: number;
  name: string;
  plan: PlanTier;
  users: number;
  status: MemberStatus;
  joined: string;
  rev: string;
}
