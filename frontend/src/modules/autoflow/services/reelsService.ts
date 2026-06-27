// Reel API client — mirrors the outboxService pattern (thin wrappers over services/api).
// Backend: internal/server/reels (POST/GET /reels, GET /reels/:id, PATCH /script,
// POST /approve, POST /publish). There is intentionally NO delete endpoint.
import { apiFetch, get, patch, post } from './api';

export interface ReelShot {
  id: number;
  reel_id: number;
  org_id: number;
  scene: number;
  kind: string;
  render_state: string;
  provider: string;
  provider_job_id: string;
  output_key: string;
  cost_usd: number;
  attempts: number;
}

export interface ReelScript {
  id: number;
  reel_id: number;
  org_id: number;
  version: number;
  dialogue: string;
  shot_list: string; // JSON array string
  caption: string;
  verify_flags: string; // JSON array string
  approved: boolean;
}

export interface Reel {
  id: number;
  org_id: number;
  mission_id: string;
  created_by: number;
  source: string;
  status: string;
  brief_style: string;
  keywords: string; // JSON array string
  product_refs: string;
  target_duration_sec: number;
  render_idempotency_key: string;
  final_output_key: string;
  total_cost_usd: number;
}

export interface ReelResult {
  reel: Reel;
  script: ReelScript | null;
  shots?: ReelShot[];
}

export interface ReelDetail extends ReelResult {
  status: string;
  shots_total: number;
  shots_done: number;
  total_cost_usd: number;
}

export interface PublishResult {
  outbound_id: number;
  allowed: boolean;
  reason: string;
}

export interface CreateReelInput {
  brief_style: string;
  keywords: string[];
  target_duration_sec: number;
}

export async function listReels(limit = 50): Promise<Reel[]> {
  const r = await get<{ reels: Reel[]; count: number }>(`/reels?limit=${limit}`);
  return r.reels ?? [];
}

export async function getReel(id: number): Promise<ReelDetail> {
  return get<ReelDetail>(`/reels/${id}`);
}

export async function createReel(input: CreateReelInput): Promise<ReelResult> {
  return post<ReelResult>('/reels', input);
}

export async function updateScriptCaption(id: number, caption: string): Promise<ReelResult> {
  return patch<ReelResult>(`/reels/${id}/script`, { caption });
}

export async function approveReel(id: number): Promise<ReelResult> {
  return post<ReelResult>(`/reels/${id}/approve`, {});
}

export async function publishReel(id: number, accountId: number, targetUrl: string): Promise<PublishResult> {
  return post<PublishResult>(`/reels/${id}/publish`, { account_id: accountId, target_url: targetUrl });
}

// fetchReelVideo streams the finished .mp4 as a Blob. The endpoint requires the JWT
// auth header (so a plain <video src> would 401) — go through apiFetch, which attaches
// the token and handles refresh, then read the body as a blob for an object URL.
export async function fetchReelVideo(id: number): Promise<Blob> {
  const res = await apiFetch(`/reels/${id}/video`);
  if (!res.ok) {
    throw new Error(res.status === 404 ? 'Video chưa sẵn sàng' : `HTTP ${res.status}`);
  }
  return res.blob();
}

// parseShots safely decodes a script's shot_list JSON string.
export function parseShots(shotListJSON: string): Array<{ scene: number; kind: string; prompt: string; dur_sec: number; voiceover: string }> {
  try {
    const v = JSON.parse(shotListJSON || '[]');
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}

// parseFlags safely decodes a verify_flags JSON string.
export function parseFlags(flagsJSON: string): string[] {
  try {
    const v = JSON.parse(flagsJSON || '[]');
    return Array.isArray(v) ? v.map(String) : [];
  } catch {
    return [];
  }
}
