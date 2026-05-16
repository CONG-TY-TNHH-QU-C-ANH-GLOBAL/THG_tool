import type { Lead, LeadEngagementState, LeadStatus, LeadThreadRole } from '../types';
import * as api from './api';

interface LeadsResponse { leads: BackendLead[]; count: number; }
interface BackendLead {
  id: number; author: string; author_url: string; content: string;
  score: string; service_match: string; author_role: string; pain_point: string;
  niche: string; source_url: string; secondary_url?: string; source_type?: string;
  thread_role?: string;
  classified_at: string; created_at: string;
}

const THREAD_ROLES: LeadThreadRole[] = [
  'intent_originator', 'supplier_responder', 'buyer_responder', 'competitor', 'noise',
];

function normalizeThreadRole(raw?: string): LeadThreadRole {
  const v = (raw ?? '').toLowerCase().trim() as LeadThreadRole;
  // Legacy / unknown rows default to intent_originator — every pre-Phase-B
  // crawl was a post-sourced lead. Mirrors models.NormalizeThreadRole.
  return THREAD_ROLES.includes(v) ? v : 'intent_originator';
}

function normalizeScore(s: string): LeadStatus {
  const map: Record<string, LeadStatus> = { hot: 'Hot', warm: 'Warm', cold: 'Cold' };
  return map[s.toLowerCase()] ?? 'Cold';
}

function numericScore(s: LeadStatus): number {
  if (s === 'Hot') return 92;
  if (s === 'Warm') return 70;
  return 45;
}

function toLead(b: BackendLead): Lead {
  const status = normalizeScore(b.score);
  return {
    id: b.id,
    name: b.author || `Lead #${b.id}`,
    status,
    group: b.niche,
    agent: b.author_role || b.service_match || 'AI classifier',
    last: new Date(b.created_at).toLocaleDateString('vi'),
    score: numericScore(status),
    phone: b.pain_point || '',
    // 3-URL separation — never collapse these (thread-role memory):
    facebookUrl: b.author_url || undefined,           // actor profile
    postUrl: b.source_url || undefined,               // canonical post
    engagementPermalink: b.secondary_url || undefined, // exact comment permalink
    sourceType: b.source_type || undefined,
    threadRole: normalizeThreadRole(b.thread_role),
  };
}

export async function getLeads(orgId: string, status?: LeadStatus | 'All'): Promise<Lead[]> {
  void orgId;
  try {
    const param = status && status !== 'All' ? `?score=${status.toLowerCase()}&limit=50` : '?limit=50';
    const res = await api.get<LeadsResponse>(`/leads${param}`);
    return (res.leads ?? []).map(toLead);
  } catch {
    return [];
  }
}

export async function createLead(orgId: string, data: Pick<Lead, 'name' | 'phone' | 'group'>): Promise<Lead> {
  void orgId;
  void data;
  throw new Error('manual lead creation is not wired to production API');
}

export async function deleteLead(orgId: string, leadId: number, sourceType?: string): Promise<void> {
  void orgId;
  const qs = sourceType ? `?source=${encodeURIComponent(sourceType)}` : '';
  await api.del(`/leads/${leadId}${qs}`);
}

// deleteAllLeads clears every lead for the org (legacy + task_leads tables).
// Admin-only on the backend. Optional niche narrows the legacy side.
export async function deleteAllLeads(orgId: string, niche?: string): Promise<{ deleted: number }> {
  void orgId;
  const qs = niche ? `?niche=${encodeURIComponent(niche)}` : '';
  return api.del<{ deleted: number }>(`/leads/all${qs}`);
}

export interface ReclassifyRequest {
  user_prompt: string;
  target_role?: string;
  positive_signals?: string[];
  only_unknown?: boolean;
  limit?: number;
}

export interface ReclassifyResponse {
  matched: number;
  reclassified: number;
  failed: number;
  message?: string;
}

export async function reclassifyLeads(orgId: string, body: ReclassifyRequest): Promise<ReclassifyResponse> {
  void orgId;
  return api.post<ReclassifyResponse>('/leads/reclassify', body);
}

// Classifier observability — answers "why did the AI reject every post
// in my crawl?". Admin-only on the backend.
export interface ClassificationEntry {
  id: number;
  org_id: number;
  task_id: string;
  account_id: number;
  source_url: string;
  author_name: string;
  content_snippet: string;
  ai_intent: string;
  ai_priority: string;
  ai_reason: string;
  ai_score: number;
  target_role: string;
  decision: 'kept' | 'rejected' | 'cold' | 'error' | 'skipped_filter';
  user_prompt: string;
  created_at: string;
}

export interface ClassificationBreakdown {
  total: number;
  kept: number;
  rejected: number;
  by_intent: Record<string, number>;
  by_reason: Array<{ reason: string; count: number }>;
}

export interface ClassificationsResponse {
  classifications: ClassificationEntry[];
  count: number;
  breakdown: ClassificationBreakdown | null;
}

export async function getRecentClassifications(
  params: { taskId?: string; decision?: string; limit?: number } = {},
): Promise<ClassificationsResponse> {
  const q = new URLSearchParams();
  if (params.taskId) q.set('task_id', params.taskId);
  if (params.decision) q.set('decision', params.decision);
  if (params.limit) q.set('limit', String(params.limit));
  const qs = q.toString();
  return api.get<ClassificationsResponse>('/leads/classifications/recent' + (qs ? '?' + qs : ''));
}

// Lead Engagement batch fetch — see project_distributed_coordination.md PR-4.
// Returns a map keyed by lead_id; missing ids resolve to undefined (caller
// must default to 'priority' for display purposes).
interface EngagementBatchResponse {
  engagements: Record<string, LeadEngagementState | undefined>;
}

export async function getLeadEngagementsBatch(
  orgId: string,
  leadIds: number[],
): Promise<Record<number, LeadEngagementState | undefined>> {
  void orgId;
  if (leadIds.length === 0) return {};
  // Backend caps at 100 per call.
  const chunks: number[][] = [];
  for (let i = 0; i < leadIds.length; i += 100) {
    chunks.push(leadIds.slice(i, i + 100));
  }
  const out: Record<number, LeadEngagementState | undefined> = {};
  for (const chunk of chunks) {
    try {
      const res = await api.get<EngagementBatchResponse>(
        `/leads/engagement?ids=${chunk.join(',')}`,
      );
      const map = res.engagements ?? {};
      for (const [k, v] of Object.entries(map)) {
        out[Number(k)] = v;
      }
    } catch {
      // Best-effort: engagement is decorative. Leads still render without it.
    }
  }
  return out;
}
