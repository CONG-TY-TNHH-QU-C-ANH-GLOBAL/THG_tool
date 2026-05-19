/**
 * Workspace Knowledge OS — sources & sync.
 *
 * Thin TS wrappers over /api/knowledge/* endpoints introduced in PR-3.
 * The shapes mirror the Go contracts in:
 *   - internal/workspace_knowledge/sources
 *   - internal/workspace_knowledge/ingestion (SyncResult / SyncError)
 *
 * Keep field names snake_case so the UI binds straight off the JSON
 * without an intermediate camelCase normaliser; we apply that
 * convention everywhere autoflow surfaces talk to the Go API.
 */
import { del, get, patch, post } from './api';

/** Closed set of source types mirrored from sources.SourceType in Go. */
export type SourceType =
  | 'shopify'
  | 'csv'
  | 'google_sheets'
  | 'notion'
  | 'website'
  | 'catalog'
  | 'rest_json';

export type SyncPolicy = 'manual' | 'realtime' | 'hourly' | 'daily';

export type HealthStatus =
  | 'healthy'
  | 'syncing'
  | 'stale'
  | 'error'
  | 'needs_auth';

export interface KnowledgeSource {
  id: number;
  org_id: number;
  type: SourceType;
  label: string;
  connection_config: unknown; // adapter-specific; opaque on the FE
  sync_policy: SyncPolicy;
  health_status: HealthStatus;
  health_message: string;
  last_sync_at: string | null;
  last_asset_count: number;
  created_at: string;
  updated_at: string;
}

export interface SyncError {
  external_id?: string;
  reason: string;
  detail?: string;
}

export interface SyncResult {
  assets_seen: number;
  assets_created: number;
  assets_updated: number;
  assets_rejected: number;
  errors?: SyncError[];
}

export interface SyncResponse {
  result: SyncResult;
  /** Present on non-2xx adapter outcomes (502 recoverable, 422 permanent). */
  error?: string;
}

export interface CreateKnowledgeSourceInput {
  type: SourceType;
  label: string;
  connection_config: unknown;
  sync_policy?: SyncPolicy;
}

export interface UpdateKnowledgeSourceInput {
  label?: string;
  connection_config?: unknown;
  sync_policy?: SyncPolicy;
}

interface ListResponse {
  sources: KnowledgeSource[];
  count: number;
}

/** List every knowledge source the org has configured. */
export async function listKnowledgeSources(
  filter?: { types?: SourceType[]; health?: HealthStatus[] },
): Promise<KnowledgeSource[]> {
  const params = new URLSearchParams();
  if (filter?.types?.length) params.set('type', filter.types.join(','));
  if (filter?.health?.length) params.set('health', filter.health.join(','));
  const qs = params.toString();
  const res = await get<ListResponse>(`/knowledge/sources${qs ? `?${qs}` : ''}`);
  return res.sources ?? [];
}

export async function createKnowledgeSource(input: CreateKnowledgeSourceInput): Promise<KnowledgeSource> {
  return post<KnowledgeSource>('/knowledge/sources', input);
}

export async function updateKnowledgeSource(
  id: number,
  input: UpdateKnowledgeSourceInput,
): Promise<KnowledgeSource> {
  return patch<KnowledgeSource>(`/knowledge/sources/${id}`, input);
}

export async function deleteKnowledgeSource(id: number): Promise<{ ok: boolean; assets_deleted: number }> {
  return del<{ ok: boolean; assets_deleted: number }>(`/knowledge/sources/${id}`);
}

/**
 * Trigger an immediate sync. Resolves on success (2xx) and rejects on
 * adapter errors (502 recoverable, 422 permanent). The thrown error
 * message carries the human-readable detail surfaced by the
 * dispatcher; callers should display it on the source card.
 */
export async function syncKnowledgeSource(id: number): Promise<SyncResult> {
  const res = await post<SyncResponse>(`/knowledge/sources/${id}/sync`, {});
  if (res.error) throw new Error(res.error);
  return res.result;
}
