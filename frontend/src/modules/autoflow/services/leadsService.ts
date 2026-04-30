import type { Lead, LeadStatus } from '../types';
import * as api from './api';

interface LeadsResponse { leads: BackendLead[]; count: number; }
interface BackendLead {
  id: number; author: string; author_url: string; content: string;
  score: string; service_match: string; author_role: string; pain_point: string;
  niche: string; source_url: string; classified_at: string; created_at: string;
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
    facebookUrl: b.author_url || b.source_url,
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
