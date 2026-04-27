import type { Lead, LeadStatus } from '../types';
import { MOCK_LEADS } from './mockData';
import * as api from './api';

interface LeadsResponse { leads: BackendLead[]; count: number; }
interface BackendLead {
  id: number; author: string; author_url: string; content: string;
  score: string; niche: string; source_url: string; classified_at: string; created_at: string;
}

function normalizeScore(s: string): LeadStatus {
  const map: Record<string, LeadStatus> = { hot: 'Hot', warm: 'Warm', cold: 'Cold' };
  return map[s.toLowerCase()] ?? 'Cold';
}

function toLead(b: BackendLead, idx: number): Lead {
  return {
    id: b.id,
    name: b.author || `Lead #${b.id}`,
    status: normalizeScore(b.score),
    group: b.niche,
    agent: 'Agent_01',
    last: new Date(b.created_at).toLocaleDateString('vi'),
    score: 50,
    phone: '',
  };
}

export async function getLeads(orgId: string, status?: LeadStatus | 'All'): Promise<Lead[]> {
  void orgId;
  try {
    const param = status && status !== 'All' ? `?score=${status.toLowerCase()}&limit=50` : '?limit=50';
    const res = await api.get<LeadsResponse>(`/leads${param}`);
    return (res.leads ?? []).map(toLead);
  } catch {
    return status && status !== 'All' ? MOCK_LEADS.filter(l => l.status === status) : [...MOCK_LEADS];
  }
}

export async function createLead(orgId: string, data: Pick<Lead, 'name' | 'phone' | 'group'>): Promise<Lead> {
  void orgId;
  const lead: Lead = { ...data, id: Date.now(), status: 'Warm', agent: '', last: 'vừa xong', score: 50 };
  MOCK_LEADS.push(lead);
  return lead;
}
