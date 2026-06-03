/**
 * Accounts service — the member's OWNED Facebook accounts.
 *
 * GET /api/accounts is owner-scoped on the backend: sales members see only
 * accounts where assigned_user_id == them; admins/platform roles see all in
 * the org (internal/server/workspace/handlers.go getAccounts). The frontend
 * therefore does NOT filter — it renders exactly what the API returns.
 */
import { get } from './api';

export interface MemberAccount {
  id: number;
  name: string;
  email: string;
  status: string;
  assignedUserId: number;
  assignedUserName: string;
  browserLoggedIn: boolean;
  fbUserId: string;
  fbDisplayName: string;
  fbUsername: string;
  fbProfileUrl: string;
}

// Raw row shape returned by the backend (snake_case JSON).
interface RawAccount {
  id: number;
  name?: string;
  email?: string;
  status?: string;
  assigned_user_id?: number;
  assigned_user_name?: string;
  browser_logged_in?: boolean;
  fb_user_id?: string;
  fb_display_name?: string;
  fb_username?: string;
  fb_profile_url?: string;
}

function mapAccount(r: RawAccount): MemberAccount {
  return {
    id: r.id,
    name: r.name ?? '',
    email: r.email ?? '',
    status: r.status ?? '',
    assignedUserId: r.assigned_user_id ?? 0,
    assignedUserName: r.assigned_user_name ?? '',
    browserLoggedIn: Boolean(r.browser_logged_in),
    fbUserId: r.fb_user_id ?? '',
    fbDisplayName: r.fb_display_name ?? '',
    fbUsername: r.fb_username ?? '',
    fbProfileUrl: r.fb_profile_url ?? '',
  };
}

export async function listAccounts(): Promise<MemberAccount[]> {
  const r = await get<{ accounts?: RawAccount[] }>('/accounts');
  return (r.accounts ?? []).map(mapAccount);
}
