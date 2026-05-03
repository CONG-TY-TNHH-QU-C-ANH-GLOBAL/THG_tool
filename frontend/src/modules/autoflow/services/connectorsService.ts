import { get, post, put, del } from './api';
import type { LocalConnector, LocalConnectorAction, LocalConnectorScreen } from '../types';

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
    actions: (item.actions ?? []).map(mapAction),
  };
}

function mapAction(item: any): LocalConnectorAction {
  return {
    id: item.id,
    accountId: item.account_id,
    agentId: item.agent_id,
    type: item.type,
    status: item.status,
    errorMsg: item.error_msg,
    createdAt: item.created_at,
    claimedAt: item.claimed_at,
    completedAt: item.completed_at,
  };
}

export async function getLocalConnectors(): Promise<LocalConnector[]> {
  const res = await get<{ connectors: any[] }>('/connectors');
  return (res.connectors ?? []).map(mapConnector);
}

export async function getLocalConnectorScreen(accountId?: number): Promise<LocalConnectorScreen | null> {
  const qs = accountId ? `?account_id=${accountId}` : '';
  const res = await get<{ screen: any | null; actions?: any[] }>(`/connectors/screen${qs}`);
  if (!res.screen) return null;
  return mapScreen({ ...res.screen, actions: res.actions ?? [] });
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

export async function sendConnectorInput(accountId: number, type: 'click' | 'key' | 'text' | 'scroll', payload: Record<string, unknown>): Promise<{ id: number; status: string }> {
  return post('/connectors/input', {
    account_id: accountId,
    type,
    payload,
  });
}
