import type { FacebookStatus } from '../types';
import * as api from './api';

interface BackendFBStatus {
  connected: boolean; account?: string; groups?: number;
  leads_today?: number; expires_label?: string;
}

function toFBStatus(b: BackendFBStatus): FacebookStatus {
  return {
    connected: b.connected,
    account: b.account,
    groups: b.groups,
    leadsToday: b.leads_today,
    expiresLabel: b.expires_label,
  };
}

export async function getFacebookStatus(orgId: string): Promise<FacebookStatus> {
  void orgId;
  try {
    const res = await api.get<BackendFBStatus>('/facebook/status');
    return toFBStatus(res);
  } catch {
    return { connected: false };
  }
}

export async function connectFacebook(orgId: string): Promise<FacebookStatus> {
  void orgId;
  try {
    const res = await api.get<BackendFBStatus>('/facebook/status');
    return toFBStatus(res);
  } catch {
    return { connected: false };
  }
}

export async function disconnectFacebook(orgId: string): Promise<void> {
  void orgId;
}
