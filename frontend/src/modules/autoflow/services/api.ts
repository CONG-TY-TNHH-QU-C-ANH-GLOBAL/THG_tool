/**
 * Central API client with automatic token refresh.
 *
 * All outbound requests go through apiFetch(). On 401:
 *   1. ensureRefresh() fires — only ONE /auth/refresh call even with N concurrent 401s
 *   2. All N callers wait on the same Promise, then retry with the new token
 *   3. If refresh fails → clear session → throw UNAUTHENTICATED → UI redirects to login
 *
 * Nothing outside this file needs to think about 401 handling.
 */

import { useAuthStore } from '../stores/authStore';

const BASE = '/api';
let rateLimitedUntil = 0;

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function noteRateLimit(res: Response): void {
  if (res.status !== 429) return;
  const retryAfter = Number(res.headers.get('Retry-After') ?? '0');
  const delay = Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter * 1000 : 10_000;
  rateLimitedUntil = Math.max(rateLimitedUntil, Date.now() + delay);
}

async function waitForRateLimitWindow(): Promise<void> {
  const waitMs = rateLimitedUntil - Date.now();
  if (waitMs > 0) await sleep(Math.min(waitMs, 30_000));
}

// ── Layer 2: Silent pre-expiry refresh scheduler ─────────────────────────────

function getTokenExpiry(token: string): number {
  try {
    const payload = JSON.parse(atob(token.split('.')[1]));
    return (payload.exp as number) * 1000;
  } catch {
    return 0;
  }
}

let refreshTimer: ReturnType<typeof setTimeout> | null = null;

export function scheduleRefresh(token: string): void {
  if (refreshTimer) clearTimeout(refreshTimer);
  const expiry = getTokenExpiry(token);
  if (!expiry) return;
  const delay = Math.max(expiry - Date.now() - 60_000, 0);
  refreshTimer = setTimeout(() => ensureRefresh(), delay);
}

export function cancelRefreshSchedule(): void {
  if (refreshTimer) { clearTimeout(refreshTimer); refreshTimer = null; }
}

// ── Deduplication: at most one /auth/refresh in-flight ──────────────────────

let isRefreshing = false;
let refreshWaiters: Array<(token: string | null) => void> = [];

/**
 * Raw refresh — uses fetch() directly so it NEVER goes through apiFetch,
 * preventing any possibility of an infinite refresh loop.
 */
async function doRefresh(): Promise<string | null> {
  try {
    await waitForRateLimitWindow();
    const res = await fetch(`${BASE}/auth/refresh`, {
      method: 'POST',
      credentials: 'include', // sends httpOnly refresh_token cookie
    });
    noteRateLimit(res);
    if (res.status === 429) {
      return useAuthStore.getState().token;
    }
    if (!res.ok) return null;
    const data = await res.json();
    return (data.access_token as string) ?? null;
  } catch {
    return null;
  }
}

/**
 * Deduplication wrapper: if a refresh is already in-flight, queue and share
 * its result. Otherwise start one, then resolve all waiters.
 */
async function ensureRefresh(): Promise<string | null> {
  if (isRefreshing) {
    return new Promise<string | null>(resolve => refreshWaiters.push(resolve));
  }

  isRefreshing = true;
  const newToken = await doRefresh();
  isRefreshing = false;

  const store = useAuthStore.getState();
  if (newToken) {
    store.setToken(newToken);
  } else {
    // Refresh token expired or revoked — clear everything and force re-login.
    store.setToken(null);
    store.setUser(null);
  }

  refreshWaiters.forEach(r => r(newToken));
  refreshWaiters = [];

  return newToken;
}

// ── Central fetch wrapper ─────────────────────────────────────────────────────

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const makeReq = (token: string | null): Promise<Response> =>
    fetch(`${BASE}${path}`, {
      ...init,
      credentials: 'include',
      headers: {
        // Only set Content-Type for requests with a body (avoids CORS preflight on GETs)
        ...(init.body != null ? { 'Content-Type': 'application/json' } : {}),
        ...(init.headers as Record<string, string> | undefined ?? {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
    });

  await waitForRateLimitWindow();
  let res = await makeReq(useAuthStore.getState().token);
  noteRateLimit(res);

  if (res.status === 401) {
    const newToken = await ensureRefresh();
    if (!newToken) throw new Error('UNAUTHENTICATED');
    await waitForRateLimitWindow();
    res = await makeReq(newToken);
    noteRateLimit(res);
  }

  return res;
}

// ── Convenience wrappers ──────────────────────────────────────────────────────

async function handleJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const err = body as Record<string, string>;
    const fallback = res.status === 429 ? 'API đang bị giới hạn tạm thời, hệ thống sẽ tự thử lại' : `HTTP ${res.status}`;
    const message = err.error ?? err.message ?? fallback;
    throw new Error(err.hint ? `${message}. ${err.hint}` : message);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export async function get<T>(path: string): Promise<T> {
  return handleJSON<T>(await apiFetch(path));
}

export async function post<T>(path: string, body: unknown): Promise<T> {
  return handleJSON<T>(await apiFetch(path, {
    method: 'POST',
    body: JSON.stringify(body),
  }));
}

export async function put<T>(path: string, body: unknown): Promise<T> {
  return handleJSON<T>(await apiFetch(path, {
    method: 'PUT',
    body: JSON.stringify(body),
  }));
}

export async function patch<T>(path: string, body: unknown): Promise<T> {
  return handleJSON<T>(await apiFetch(path, {
    method: 'PATCH',
    body: JSON.stringify(body),
  }));
}

export async function del<T = void>(path: string): Promise<T> {
  return handleJSON<T>(await apiFetch(path, { method: 'DELETE' }));
}

export async function upload<T>(path: string, file: File): Promise<T> {
  // FormData requests must NOT have Content-Type set (browser sets it with boundary).
  // Handled separately so the apiFetch Content-Type logic doesn't interfere.
  const makeUpload = (token: string | null): Promise<Response> => {
    const fd = new FormData();
    fd.append('file', file);
    return fetch(`${BASE}${path}`, {
      method: 'POST',
      credentials: 'include',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    });
  };

  let res = await makeUpload(useAuthStore.getState().token);
  noteRateLimit(res);
  if (res.status === 401) {
    const newToken = await ensureRefresh();
    if (!newToken) throw new Error('UNAUTHENTICATED');
    await waitForRateLimitWindow();
    res = await makeUpload(newToken);
    noteRateLimit(res);
  }
  return handleJSON<T>(res);
}

// Legacy compatibility — callers that used setToken() directly still work.
export function setToken(token: string | null) {
  useAuthStore.getState().setToken(token);
}
