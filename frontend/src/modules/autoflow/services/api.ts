const BASE = '/api';

let authToken: string | null = null;

export function setToken(token: string | null) {
  authToken = token;
}

function headers(): HeadersInit {
  const h: HeadersInit = { 'Content-Type': 'application/json' };
  if (authToken) (h as Record<string,string>)['Authorization'] = `Bearer ${authToken}`;
  return h;
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export async function get<T>(path: string): Promise<T> {
  return handleResponse<T>(await fetch(`${BASE}${path}`, { headers: headers() }));
}

export async function post<T>(path: string, body: unknown): Promise<T> {
  return handleResponse<T>(await fetch(`${BASE}${path}`, {
    method: 'POST', headers: headers(), body: JSON.stringify(body),
  }));
}

export async function put<T>(path: string, body: unknown): Promise<T> {
  return handleResponse<T>(await fetch(`${BASE}${path}`, {
    method: 'PUT', headers: headers(), body: JSON.stringify(body),
  }));
}

export async function del(path: string): Promise<void> {
  await handleResponse<void>(await fetch(`${BASE}${path}`, { method: 'DELETE', headers: headers() }));
}

export async function upload<T>(path: string, file: File): Promise<T> {
  const fd = new FormData();
  fd.append('file', file);
  const h: HeadersInit = {};
  if (authToken) (h as Record<string,string>)['Authorization'] = `Bearer ${authToken}`;
  return handleResponse<T>(await fetch(`${BASE}${path}`, { method: 'POST', headers: h, body: fd }));
}
