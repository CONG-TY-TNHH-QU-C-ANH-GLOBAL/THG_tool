// Lead Lifecycle service (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-4). Thin transport
// over the backend lifecycle endpoints. The backend owns all lifecycle derivation; this
// layer only fetches + maps. Reuses toLead/BackendLead from leadsService (DRY).
import type { Lead, LeadLifecycleState } from '../types';
import * as api from './api';
import { toLead, type BackendLead } from './leadsService';

interface LifecycleBatchResponse {
  lifecycles: Record<string, LeadLifecycleState | undefined>;
}

// getLeadLifecyclesBatch returns a map keyed by lead_id (missing ids → undefined, caller
// defaults to 'active' for display). Backend caps at 100 ids/call.
export async function getLeadLifecyclesBatch(
  leadIds: number[],
): Promise<Record<number, LeadLifecycleState | undefined>> {
  if (leadIds.length === 0) return {};
  const out: Record<number, LeadLifecycleState | undefined> = {};
  for (let i = 0; i < leadIds.length; i += 100) {
    const chunk = leadIds.slice(i, i + 100);
    try {
      const res = await api.get<LifecycleBatchResponse>(`/leads/lifecycle?ids=${chunk.join(',')}`);
      for (const [k, v] of Object.entries(res.lifecycles ?? {})) {
        out[Number(k)] = v;
      }
    } catch {
      // Best-effort: lifecycle grouping degrades to "active" when unavailable.
    }
  }
  return out;
}

interface ArchivedResponse { leads: BackendLead[]; count: number; }

// getArchivedLeads fetches the "Đã lưu trữ" tab (the only read path that surfaces archived).
export async function getArchivedLeads(limit = 50, offset = 0): Promise<Lead[]> {
  try {
    const res = await api.get<ArchivedResponse>(`/leads/archived?limit=${limit}&offset=${offset}`);
    return (res.leads ?? []).map(toLead);
  } catch {
    return [];
  }
}

// archiveLead / unarchiveLead are the manual operator actions (no hard delete).
export async function archiveLead(leadId: number, reason = 'manual_not_relevant'): Promise<void> {
  await api.post(`/leads/${leadId}/archive`, { reason });
}

export async function unarchiveLead(leadId: number): Promise<void> {
  await api.post(`/leads/${leadId}/unarchive`, {});
}
