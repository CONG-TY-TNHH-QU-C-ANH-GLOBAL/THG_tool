import { get, post, put, del } from './api';
import type { LocalConnector } from '../types';

function mapConnector(item: any): LocalConnector {
  return {
    id: item.id,
    orgId: item.org_id,
    name: item.name,
    createdBy: item.created_by,
    hostname: item.hostname,
    os: item.os,
    version: item.version,
    kind: item.kind,
    transport: item.transport,
    assignedAccountId: item.assigned_account_id,
    capabilitiesJson: item.capabilities_json,
    currentUrl: item.current_url,
    fbUserId: item.fb_user_id,
    fbDisplayName: item.fb_display_name,
    fbUsername: item.fb_username,
    fbProfileUrl: item.fb_profile_url,
    streamStatus: item.stream_status,
    chromeError: item.chrome_error,
    lastSeen: item.last_seen,
    online: Boolean(item.online),
    active: Boolean(item.active),
    createdAt: item.created_at,
  };
}

export async function getLocalConnectors(): Promise<LocalConnector[]> {
  const res = await get<{ connectors: any[] }>('/connectors');
  return (res.connectors ?? []).map(mapConnector);
}

export async function createLocalConnectorPairingCode(name: string, accountId?: number): Promise<{ id: number; code: string; expires_at: string; ttl_seconds: number }> {
  return post('/connectors/pairing-code', { name, account_id: accountId ?? 0 });
}

export type PairingFacebookStatus =
  | 'waiting_pairing'
  | 'pairing_code_expired'
  | 'binding_released'
  | 'detected'
  | 'facebook_session_stale'
  | 'facebook_session_not_detected'
  | 'facebook_account_already_connected_to_another_member';

export interface PairingFacebookStatusResponse {
  status: PairingFacebookStatus;
  pairing_session_id: number;
  connector_id?: number;
  fb_user_id?: string;
  fb_display_name?: string;
  last_proof_at?: string;
}

// Verifies the Facebook login of ONE pairing session (exact connector +
// pairing_session_id) — never "latest workspace heartbeat".
export async function getPairingFacebookStatus(pairingSessionId: number): Promise<PairingFacebookStatusResponse> {
  return get(`/connectors/pairing/${pairingSessionId}/facebook-status`);
}

export async function assignLocalConnectorAccount(id: number, accountId: number): Promise<void> {
  await put(`/connectors/${id}/account`, { account_id: accountId });
}

export async function revokeLocalConnector(id: number): Promise<void> {
  await del(`/connectors/${id}`);
}

export async function disconnectLocalConnector(id: number): Promise<void> {
  await post(`/connectors/${id}/disconnect`, {});
}
