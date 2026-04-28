import { get, post } from './api';

export interface SAOrg { id: number; name: string; domain: string; plan_tier: string; max_accounts: number; active: boolean; created_at: string; }
export interface SAAccount { id: number; org_id: number; name: string; platform: string; email: string; status: string; browser_logged_in: boolean; created_at: string; }
export interface SAUser { id: number; org_id: number; name: string; email: string; role: string; active: boolean; created_at: string; }
export interface SASession { account_id: number; org_id: number; status: string; cdp_port: number; vnc_port: number; started_at: string; last_active_at: string; }
export interface QueryResult { columns: string[]; rows: Record<string, unknown>[]; count: number; }

export async function getOrgs(): Promise<SAOrg[]> { const r = await get<{organizations: SAOrg[]}>('/superadmin/orgs'); return r.organizations ?? []; }
export async function getAccounts(): Promise<SAAccount[]> { const r = await get<{accounts: SAAccount[]}>('/superadmin/accounts'); return r.accounts ?? []; }
export async function getUsers(): Promise<SAUser[]> { const r = await get<{users: SAUser[]}>('/superadmin/users'); return r.users ?? []; }
export async function getSessions(): Promise<SASession[]> { const r = await get<{sessions: SASession[]}>('/superadmin/sessions'); return r.sessions ?? []; }
export async function runQuery(sql: string): Promise<QueryResult> { return post<QueryResult>('/superadmin/query', { sql }); }