/**
 * Types for the Workspace Knowledge OS UI. Mirrors the 7-layer
 * architecture defined in .claude/Architecting the Multi-Tenant
 * Workspace Knowledge OS.md — every entity here is tenant-scoped via
 * org_id and surfaces a piece of the retrieval-first knowledge layer.
 *
 * NOTE: the backend for these endpoints is not yet implemented. These
 * types are the wire shape the frontend expects; when the Go side lands
 * they should match `knowledge_sources` and `knowledge_assets` tables.
 */

export type SourceType = 'catalog' | 'shopify' | 'notion' | 'csv' | 'website' | 'google_sheets';
export type SyncPolicy = 'realtime' | 'hourly' | 'daily' | 'manual';
export type HealthStatus = 'healthy' | 'syncing' | 'stale' | 'error' | 'needs_auth';

export interface KnowledgeSource {
  id: string;
  type: SourceType;
  label: string;
  connection_hint: string;
  sync_policy: SyncPolicy;
  health_status: HealthStatus;
  last_sync_at: string | null;
  asset_count: number;
  error_message?: string;
}

export type AssetType = 'POD_product' | 'faq' | 'shipping_policy' | 'sales_playbook' | 'pricing_rule' | 'banned_claim' | 'cta';
export type AssetState = 'approved' | 'pending' | 'hidden';

export interface KnowledgeAsset {
  id: string;
  source_id: string;
  source_label: string;
  type: AssetType;
  title: string;
  description: string;
  tags: string[];
  variants?: string[];
  price?: string;
  image_url?: string;
  state: AssetState;
  pinned: boolean;
  boost: number; // 0 (default) → 100 (force-to-top)
  retrieval_count_30d: number;
  conversion_count_30d: number;
  updated_at: string;
}

export type ClaimSeverity = 'block' | 'warn';

export interface BannedClaim {
  id: string;
  pattern: string;
  reason: string;
  severity: ClaimSeverity;
  added_by: string;
  added_at: string;
  trigger_count_30d: number;
}

export interface ReplayEvent {
  id: string;
  occurred_at: string;
  lead_context: string;
  action: 'comment_drafted' | 'inbox_drafted' | 'comment_sent' | 'inbox_sent';
  outcome: 'queued' | 'approved' | 'rejected' | 'sent' | 'failed';
  retrieved_assets: Array<{
    asset_id: string;
    asset_title: string;
    score: number;
  }>;
  generated_text: string;
  operator?: string;
}
