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

export interface Lead {
  id: number;
  name: string;
  status: LeadStatus;
  group: string;
  agent: string;
  last: string;
  score: number;
  phone: string;
  facebookUrl?: string;
  postUrl?: string;
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
  emailStatus?: 'sent' | 'failed' | 'not_configured' | 'pending' | string;
  emailError?: string;
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
  browserState?: 'initializing' | 'display_ready' | 'ready' | 'idle' | 'active' | 'checkpoint' | 'human_required' | 'error' | 'terminated' | string;
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
