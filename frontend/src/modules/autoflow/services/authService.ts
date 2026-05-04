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

// Phase 4b: the access token now lives in an HttpOnly `access_token`
// cookie set by the server. JavaScript never sees it, so an XSS payload
// cannot exfiltrate the JWT. The non-HttpOnly `autoflow_session` cookie
// is the SPA's "I am logged in" signal — value is "1", never a secret.
//
// The server still echoes access_token in login/refresh response bodies
// for backward compatibility with non-browser clients (Telegram bot,
// CLI). Browser code must NOT persist that value to localStorage.
export const SESSION_PRESENT_COOKIE = 'autoflow_session';

export function hasSessionCookie(): boolean {
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some(c => c.trim().startsWith(`${SESSION_PRESENT_COOKIE}=1`));
}

// keepInMemoryToken stores the access token in the api module so
// apiFetch can still attach an Authorization header alongside the
// cookie. This is purely defence-in-depth — if the cookie is somehow
// stripped (cross-origin proxy, etc.) the header keeps the request
// authenticated. Token is never written to storage that survives a
// reload; on reload we rely on the cookie + /auth/refresh.
function keepInMemoryToken(token: string): void {
  api.setToken(token);
}

function dropInMemoryToken(): void {
  api.setToken(null);
}

// initToken is kept as a no-op so legacy callers in pages that ran
// pre-Phase-4 still type-check. The new boot path is restoreSession.
export function initToken(): void {
  // intentional no-op — sessions are restored via restoreSession()
}

// restoreSession is called once on app boot. The presence cookie is
// the cheap fast-path: when it's set we know there *was* a session
// here; we ask /auth/me and let apiFetch deal with a stale access
// cookie (it transparently refreshes).
//
// When the presence cookie is missing we still try one explicit
// refresh because the refresh-token cookie may outlive the presence
// cookie in edge cases (manual cookie wipe, browser TTL skew). Only
// a hard refresh failure means the user is genuinely logged out.
export async function restoreSession(): Promise<AuthUser | null> {
  if (hasSessionCookie()) {
    try {
      return await api.get<AuthUser>('/auth/me');
    } catch {
      return null;
    }
  }
  // No presence flag — try a refresh anyway in case the long-lived
  // refresh cookie is still good. /auth/refresh sets both cookies on
  // success, so the next /auth/me call has the fresh JWT.
  try {
    await refreshToken();
  } catch {
    return null;
  }
  try {
    return await api.get<AuthUser>('/auth/me');
  } catch {
    return null;
  }
}

export async function login(email: string, password: string): Promise<{ user: AuthUser; token: string }> {
  const res = await api.post<LoginResponse>('/auth/login', { email, password });
  keepInMemoryToken(res.access_token);
  return { user: res.user, token: res.access_token };
}

export async function register(body: RegisterBody): Promise<{ user: AuthUser; token: string }> {
  const res = await api.post<LoginResponse>('/register', body);
  keepInMemoryToken(res.access_token);
  return { user: res.user, token: res.access_token };
}

export async function getMe(): Promise<AuthUser> {
  return api.get<AuthUser>('/auth/me');
}

export async function refreshToken(): Promise<string> {
  const res = await api.post<{ access_token: string }>('/auth/refresh', {});
  keepInMemoryToken(res.access_token);
  return res.access_token;
}

export async function logout(): Promise<void> {
  try { await api.post('/auth/logout', {}); } catch { /* ignore */ }
  dropInMemoryToken();
}

export async function changePassword(current: string, next: string): Promise<void> {
  await api.put('/auth/me/password', { current_password: current, new_password: next, confirm_password: next });
}
