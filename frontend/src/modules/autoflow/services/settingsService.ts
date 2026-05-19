import * as api from './api';

export interface OrgBrand {
  id: number;
  name: string;
  domain: string;
  plan_tier: string;
  max_accounts: number;
  abbr?: string;
  color?: string;
  logo_url?: string;
  avatar_url?: string;
}

export interface AuditLog {
  id: number;
  user_id: number;
  action: string;
  ip: string;
  metadata: string;
  timestamp: string;
}

export interface AgentToken {
  id: number;
  name: string;
  created_by: number;
  hostname: string;
  os: string;
  version: string;
  kind?: string;
  transport?: string;
  assigned_account_id?: number;
  current_url?: string;
  fb_user_id?: string;
  stream_status?: string;
  online?: boolean;
  last_seen?: string;
  active: boolean;
  created_at: string;
}

export interface BillingSummary {
  plan_tier: string;
  max_accounts: number;
  account_count: number;
  staff_count: number;
  groups: number;
  leads_today: number;
  outbox_counts: Record<string, number>;
  payment_status: string;
}

export async function getOrgBrand(): Promise<OrgBrand | null> {
  const res = await api.get<{ org: OrgBrand | null }>('/org');
  return res.org ?? null;
}

export async function updateOrgBrand(body: { name: string; domain: string; abbr: string; color: string }): Promise<OrgBrand> {
  const res = await api.put<{ org: OrgBrand }>('/org', body);
  return res.org;
}

export async function uploadOrgAsset(kind: 'logo' | 'avatar', file: File): Promise<string> {
  const res = await api.upload<{ url: string }>(`/org/assets/${kind}`, file);
  return res.url;
}

export async function getAgentTokens(): Promise<AgentToken[]> {
  const res = await api.get<{ tokens: AgentToken[] }>('/admin/agent-tokens');
  return res.tokens ?? [];
}

export async function createAgentToken(name: string): Promise<{ id: number; name: string; token: string }> {
  return api.post('/admin/agent-tokens', { name });
}

export async function revokeAgentToken(id: number): Promise<void> {
  await api.del(`/admin/agent-tokens/${id}`);
}

export async function getBillingSummary(): Promise<BillingSummary> {
  return api.get('/billing/summary');
}

export async function getAuditLogs(): Promise<AuditLog[]> {
  const res = await api.get<{ logs: AuditLog[] }>('/auth/audit');
  return res.logs ?? [];
}

export interface BusinessContext {
  business_profile: string;
  business_name: string;
  business_industry: string;
  services: string;
  target_customers: string;
  target_author_role: string;
  target_signals: string;
  negative_signals: string;
  business_location: string;
  markets: string;
  business_usp: string;
  tone: string;
  approval_policy: string;
  reject_rules: string;
  private_files: string;
  data_sources: string;
}

export async function getBusinessContext(): Promise<BusinessContext> {
  return api.get('/context/business');
}

export async function saveBusinessContext(context: Partial<BusinessContext>): Promise<void> {
  await api.put('/context/business', context);
}

// ── Magic Omnibox: auto-infer business profile ──────────────────────────────
//
// The user pastes a website URL (catalog / landing page) or a 1-line
// description; the backend fetches the URL, strips HTML to text, runs an
// LLM extractor, and returns each of the 13 fields paired with a
// confidence score. The FE pre-fills the form and lets the user edit
// inline before persisting via saveBusinessContext.

export interface InferredField {
  value: string;
  confidence: number; // 0..1
}

export interface InferBusinessContextResult {
  business_profile: InferredField;
  business_name: InferredField;
  business_industry: InferredField;
  services: InferredField;
  target_customers: InferredField;
  target_author_role: InferredField;
  target_signals: InferredField;
  negative_signals: InferredField;
  business_location: InferredField;
  markets: InferredField;
  business_usp: InferredField;
  tone: InferredField;
  approval_policy: InferredField;
  reject_rules: InferredField;
  source_summary: string;
  source_url?: string;
}

export async function inferBusinessContext(input: { source_url?: string; note?: string }): Promise<InferBusinessContextResult> {
  return api.post<InferBusinessContextResult>('/context/business/infer', input);
}

// Inferred field names map 1:1 to BusinessContext keys minus the two
// "summary" pseudo-fields.
export type InferredFieldKey =
  | 'business_profile'
  | 'business_name'
  | 'business_industry'
  | 'services'
  | 'target_customers'
  | 'target_author_role'
  | 'target_signals'
  | 'negative_signals'
  | 'business_location'
  | 'markets'
  | 'business_usp'
  | 'tone'
  | 'approval_policy'
  | 'reject_rules';

export const INFERRED_FIELD_KEYS: InferredFieldKey[] = [
  'business_profile',
  'business_name',
  'business_industry',
  'services',
  'target_customers',
  'target_author_role',
  'target_signals',
  'negative_signals',
  'business_location',
  'markets',
  'business_usp',
  'tone',
  'approval_policy',
  'reject_rules',
];

export type OutboundMode = 'auto' | 'draft';

export interface OrgPolicy {
  outbound_mode: OutboundMode;
}

export async function getOrgPolicy(): Promise<OrgPolicy> {
  return api.get('/org/policy');
}

export async function updateOrgPolicy(body: OrgPolicy): Promise<OrgPolicy> {
  return api.put('/org/policy', body);
}
