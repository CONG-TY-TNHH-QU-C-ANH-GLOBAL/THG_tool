import { get, post, put, del } from './api';
import type { LocalConnector, LocalConnectorScreen } from '../types';

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
    streamStatus: item.stream_status,
    lastSeen: item.last_seen,
    online: Boolean(item.online),
    active: Boolean(item.active),
    createdAt: item.created_at,
  };
}

function mapScreen(item: any): LocalConnectorScreen {
  return {
    accountId: item.account_id,
    orgId: item.org_id,
    agentId: item.agent_id,
    imageData: item.image_data,
    currentUrl: item.current_url,
    fbUserId: item.fb_user_id,
    streamStatus: item.stream_status,
    updatedAt: item.updated_at,
  };
}

export async function getLocalConnectors(): Promise<LocalConnector[]> {
  const res = await get<{ connectors: any[] }>('/connectors');
  return (res.connectors ?? []).map(mapConnector);
}

export async function getLocalConnectorScreen(accountId?: number): Promise<LocalConnectorScreen | null> {
  const qs = accountId ? `?account_id=${accountId}` : '';
  const res = await get<{ screen: any | null }>(`/connectors/screen${qs}`);
  return res.screen ? mapScreen(res.screen) : null;
}

export async function createLocalConnectorPairingCode(name: string, accountId?: number): Promise<{ id: number; code: string; expires_at: string; ttl_seconds: number }> {
  return post('/connectors/pairing-code', { name, account_id: accountId ?? 0 });
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
