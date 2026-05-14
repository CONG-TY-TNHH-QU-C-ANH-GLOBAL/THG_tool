import type { ComponentType } from 'react';
import type { AuthUser } from '../../modules/autoflow/services/authService';

// ────────────────────────────────────────────────────────────────────────
// ServiceDescriptor — metadata layer.
// Static identity + presentation + rollout info. Independent of per-user
// resolved state. Backend admin/analytics/feature-management consume this.
// Mirror this shape on the backend in `internal/platform/services/contracts/`.
// ────────────────────────────────────────────────────────────────────────
export interface ServiceDescriptor {
  slug: string;                          // immutable identity
  internalName: string;                  // engineering name (kebab-case)
  publicLabel: string;                   // user-facing name
  category: string;                      // e.g. "automation", "analytics"
  rolloutStage: 'ga' | 'beta' | 'alpha' | 'internal';
  availability: 'public' | 'private_beta' | 'admin_only';
  version: number;                       // breaking contract version
  displayOrder: number;                  // deterministic listing
}

// ────────────────────────────────────────────────────────────────────────
// State axes — orthogonal. Each axis answers exactly one question.
// ────────────────────────────────────────────────────────────────────────

// Service-level: is the service offered to this user at all?
export type ServiceStatus = 'available' | 'unavailable' | 'suspended';

// Workspace-level: lifecycle of the user's workspace inside the service.
// "Does the workspace exist and is it operational?"
export type WorkspaceState =
  | 'none'           // user has no workspace in this service
  | 'initializing'   // creation in progress
  | 'ready'          // workspace operational
  | 'suspended';     // workspace itself is paused (admin action)

// Access-level: even if a workspace exists, may the user enter it?
// "Does the user have rights to operate?"
export type ServiceAccess =
  | 'granted'
  | 'invite_required'   // workspace exists somewhere, user must accept
  | 'billing_blocked'
  | 'region_locked'
  | 'admin_blocked';

// ────────────────────────────────────────────────────────────────────────
// Capability — what the service CAN do in this user's context.
//
// Capability != Access != Permission. Keep these three distinct:
//   - Capability  — "the system supports this feature for this user's setup"
//                   (e.g. a plan unlocks multiWorkspace). Resolved per user.
//   - Access      — "the user may enter / operate the service right now"
//                   (billing, invite, region, admin block). See ServiceAccess.
//   - Permission  — "within the workspace, what this membership may do" (RBAC).
//
// `browserAutomation: true` means the service supports browser automation —
// NOT that the current user is permitted to run it this instant.
// ────────────────────────────────────────────────────────────────────────
export interface ServiceCapabilities {
  multiWorkspace: boolean;
  browserAutomation: boolean;
  aiAgents: boolean;
}

export interface WorkspaceRBAC {
  role: string;
  permissions: string[];
}

// ResolutionTrace — internal provenance metadata. NEVER exposed to the UI.
// During the legacy-bridge period (PR 2), resolvers synthesise values from
// legacy storage (org_id). The trace records where a value came from so
// migrations, debugging, and telemetry can tell authoritative data apart
// from legacy proxies.
export interface ResolutionTrace {
  source: string;       // e.g. "org_id_proxy", "api/platform/services", "cache"
  resolver: string;     // e.g. "facebook.resolveWorkspace"
  confidence: 'legacy' | 'authoritative' | 'cached' | 'stale';
  // epoch ms — set when confidence === 'authoritative'. Enables cache
  // invalidation, stale-worker detection, async-replication debugging.
  authoritativeAt?: number;
}

export interface WorkspaceResolution {
  state: WorkspaceState;
  workspaceId?: string;   // canonical prefix: "ws_<n>"
  reason?: string;
  rbac?: WorkspaceRBAC;
  trace?: ResolutionTrace;
}

export interface AccessResolution {
  access: ServiceAccess;
  reason?: string;
  trace?: ResolutionTrace;
}

// ────────────────────────────────────────────────────────────────────────
// ServiceModule — what a service exposes to the platform.
// Pure data + resolver functions. Imported as data, registered explicitly.
// See frontend/src/platform/BOUNDARIES.md.
// ────────────────────────────────────────────────────────────────────────
export interface ServiceModule {
  descriptor: ServiceDescriptor;
  icon?: ComponentType<{ size?: number | string }>;
  views: {
    createWorkspace: ComponentType;
    workspace: ComponentType<{ workspaceId: string }>;
  };
  // Resolution layer — every semantic value flows through a resolver.
  // No consumer reads raw storage fields directly.
  resolveStatus?: (user: AuthUser | null) => ServiceStatus;
  resolveWorkspace: (user: AuthUser | null) => WorkspaceResolution;
  resolveCapabilities: (user: AuthUser | null) => ServiceCapabilities;
  resolveAccess: (user: AuthUser | null) => AccessResolution;
}

// ────────────────────────────────────────────────────────────────────────
// PlatformService — the projected, resolved shape consumed by UI.
// This is the cross-boundary contract. PR 2 backend returns this exactly.
// ────────────────────────────────────────────────────────────────────────
export interface PlatformService {
  // unpacked descriptor (renderer convenience)
  slug: string;
  label: string;
  serviceVersion: number;
  // full descriptor (admin / analytics / feature management)
  descriptor: ServiceDescriptor;
  // resolved state
  status: ServiceStatus;
  workspaceState: WorkspaceState;
  workspaceId?: string;
  access: ServiceAccess;
  accessReason?: string;
  reason?: string;            // workspace-level reason (suspension cause, etc.)
  rbac?: WorkspaceRBAC;
  capabilities: ServiceCapabilities;
  // internal provenance — never rendered; for debugging / telemetry / migration
  resolutionTraces?: ResolutionTrace[];
  icon?: ComponentType<{ size?: number | string }>;
}
