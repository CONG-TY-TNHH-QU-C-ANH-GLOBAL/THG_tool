/**
 * Membership freshness (SaaS UX Hardening PR-1).
 *
 * Single-org-per-user model today: an invite accept MOVES the user.
 * refreshMembership() is the deterministic "make my session match the
 * DB" primitive — fresh token + cookies + current org/role. Call it
 * after invite accept and whenever the API answers SESSION_STALE.
 */
import { get, post } from './api';
import type { AuthUser } from './authService';

export interface Membership {
  org_id: number;
  org_name: string;
  role: string;
}

export async function refreshMembership(): Promise<{
  access_token: string;
  org_id: number;
  org_name: string;
  user: AuthUser;
}> {
  return post('/auth/refresh-membership', {});
}

export async function listMemberships(): Promise<Membership[]> {
  const r = await get<{ memberships?: Membership[] }>('/auth/me/memberships');
  return r.memberships ?? [];
}

/**
 * joined-workspace toast handoff: the accept flow navigates immediately,
 * so the toast text crosses the route change via sessionStorage.
 */
const JOINED_KEY = 'thg_joined_workspace_name';

export function rememberJoinedWorkspace(orgName: string): void {
  try {
    sessionStorage.setItem(JOINED_KEY, orgName);
  } catch {
    /* storage unavailable — toast is best-effort */
  }
}

export function consumeJoinedWorkspace(): string | null {
  try {
    const v = sessionStorage.getItem(JOINED_KEY);
    if (v) sessionStorage.removeItem(JOINED_KEY);
    return v;
  } catch {
    return null;
  }
}
