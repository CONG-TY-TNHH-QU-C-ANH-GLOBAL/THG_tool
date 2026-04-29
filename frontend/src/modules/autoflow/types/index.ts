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
  from: 'lead' | 'agent';
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
  name: string;
  email: string;
  role: string;
  status: MemberStatus;
  joined: string;
  convs: number;
  converted: number;
  cmts: number;
}

export interface FileRecord {
  id: number;
  name: string;
  size: string;
  date: string;
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
  accountStatus: string;
  loggedIn: boolean;
  running: boolean;
  browserState?: 'initializing' | 'display_ready' | 'ready' | 'idle' | 'active' | 'error' | 'terminated' | string;
  errorMsg?: string;
  vncPort?: number;
  cdpPort?: number;
  startedAt?: string;
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
