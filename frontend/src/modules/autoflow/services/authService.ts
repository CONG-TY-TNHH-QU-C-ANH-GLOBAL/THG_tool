import * as api from './api';

export type Role = 'founder' | 'superadmin' | 'admin' | 'sales';

export function isPlatformRole(role?: string | null): boolean {
  return role === 'founder' || role === 'superadmin';
}

export interface AuthUser {
  id: number;
  email: string;
  name: string;
  role: Role;
  org_id: number;
}

interface LoginResponse {
  access_token: string;
  user: AuthUser;
}

interface RegisterBody {
  org_name: string;
  org_domain: string;
  admin_name: string;
  admin_email: string;
  admin_password: string;
}

const TOKEN_KEY = 'autoflow_token';

export function getStoredToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

function storeToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
  api.setToken(token);
}

function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
  api.setToken(null);
}

export function initToken(): void {
  const t = getStoredToken();
  if (t) api.setToken(t);
}

export async function login(email: string, password: string): Promise<{ user: AuthUser; token: string }> {
  const res = await api.post<LoginResponse>('/auth/login', { email, password });
  storeToken(res.access_token);
  return { user: res.user, token: res.access_token };
}

export async function register(body: RegisterBody): Promise<{ user: AuthUser; token: string }> {
  const res = await api.post<LoginResponse>('/register', body);
  storeToken(res.access_token);
  return { user: res.user, token: res.access_token };
}

export async function getMe(): Promise<AuthUser> {
  return api.get<AuthUser>('/auth/me');
}

export async function refreshToken(): Promise<string> {
  const res = await api.post<{ access_token: string }>('/auth/refresh', {});
  storeToken(res.access_token);
  return res.access_token;
}

export async function logout(): Promise<void> {
  try { await api.post('/auth/logout', {}); } catch { /* ignore */ }
  clearToken();
}

export async function changePassword(current: string, next: string): Promise<void> {
  await api.put('/auth/me/password', { current_password: current, new_password: next });
}
