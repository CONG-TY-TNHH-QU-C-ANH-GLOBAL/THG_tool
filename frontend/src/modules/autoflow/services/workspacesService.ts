import { get, post } from './api';
import type { Workspace } from '../types';

export async function getWorkspaces(): Promise<Workspace[]> {
  try {
    const res = await get<{ workspaces: any[] }>('/browser/workspaces');
    return res.workspaces.map(item => ({
      accountId: item.account_id,
      accountName: item.account_name,
      accountStatus: item.account_status,
      loggedIn: item.logged_in,
      running: item.running,
      browserState: item.browser_state,
      errorMsg: item.error_msg,
      vncPort: item.vnc_port,
      cdpPort: item.cdp_port,
      startedAt: item.started_at,
    }));
  } catch {
    return [];
  }
}

export async function startNewWorkspace(): Promise<{ accountId: number; vncPort: number; cdpPort: number }> {
  const r = await post<any>('/browser/workspaces/new', {});
  return { accountId: r.account_id, vncPort: r.vnc_port, cdpPort: r.cdp_port };
}

export async function startWorkspace(id: number): Promise<{ vncPort: number; cdpPort: number }> {
  const r = await post<any>(`/browser/workspaces/${id}/start`, {});
  return { vncPort: r.vnc_port, cdpPort: r.cdp_port };
}

export async function stopWorkspace(id: number): Promise<void> {
  await post(`/browser/workspaces/${id}/stop`, {});
}

export async function setWorkspaceLoggedIn(id: number, loggedIn = true): Promise<void> {
  await post(`/browser/workspaces/${id}/set-logged-in`, { logged_in: loggedIn });
}
