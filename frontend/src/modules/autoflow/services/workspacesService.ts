import { get, post } from './api';
import type { Workspace, WorkspaceSessionSnapshot } from '../types';

export async function getWorkspaces(): Promise<Workspace[]> {
  try {
    const res = await get<{ workspaces: any[] }>('/browser/workspaces');
    return res.workspaces.map(item => ({
      accountId: item.account_id,
      accountName: item.account_name,
      email: item.email,
      accountStatus: item.account_status,
      loggedIn: item.logged_in,
      fbUserId: item.fb_user_id,
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
  return { accountId: r.account_id, vncPort: r.vnc_port ?? 0, cdpPort: r.cdp_port ?? 0 };
}

export async function startWorkspace(id: number): Promise<{ vncPort: number; cdpPort: number }> {
  const r = await post<any>(`/browser/workspaces/${id}/start`, {});
  return { vncPort: r.vnc_port ?? 0, cdpPort: r.cdp_port ?? 0 };
}

export async function stopWorkspace(id: number): Promise<void> {
  await post(`/browser/workspaces/${id}/stop`, {});
}

export async function setWorkspaceLoggedIn(id: number, loggedIn = true): Promise<void> {
  await post(`/browser/workspaces/${id}/set-logged-in`, { logged_in: loggedIn });
}

export async function syncWorkspaceSession(id: number): Promise<WorkspaceSessionSnapshot> {
  const r = await post<any>(`/browser/workspaces/${id}/sync-session`, {});
  return {
    accountId: r.account_id,
    accountName: r.account_name,
    loggedIn: r.logged_in,
    fbUserId: r.fb_user_id,
    storedFbUserId: r.stored_fb_user_id,
    currentUrl: r.current_url,
    currentTitle: r.current_title,
    checkpoint: Boolean(r.checkpoint),
    humanRequired: Boolean(r.human_required),
    humanReason: r.human_reason,
    cookieError: r.cookie_error,
  };
}
