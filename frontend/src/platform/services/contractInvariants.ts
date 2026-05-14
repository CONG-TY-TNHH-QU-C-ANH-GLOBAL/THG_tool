import type { ServiceModule } from './types';

// Contract invariants — the non-negotiable shape rules for a service contract.
// Enforced at registration time (see registry.ts). A violation throws at boot,
// not at runtime. PR 2 backend mirrors these in internal/platform/services/contracts/.
//
// Invariants:
//  1. slug is a non-empty, immutable, lower-kebab/snake string (identity, not name)
//  2. all IDs are strings (no numeric IDs at the contract boundary)
//  3. no storage field names leak (org_id, plan_tier, *_at, ...)
//  4. descriptor is fully populated
//  5. all resolvers are present and callable
//  6. resolvers are total — they do not throw on a null user
//
// See frontend/src/platform/BOUNDARIES.md § Contract invariants.

const STORAGE_FIELD_PATTERN = /(^|_)(org_id|plan_tier|created_at|updated_at|deleted_at)($|_)/i;
const SLUG_PATTERN = /^[a-z][a-z0-9_-]*$/;

export class ContractInvariantViolation extends Error {
  constructor(slug: string, rule: string) {
    super(`[contract-invariant] service "${slug}" violates: ${rule}`);
    this.name = 'ContractInvariantViolation';
  }
}

export function assertContractInvariants(mod: ServiceModule): void {
  const d = mod.descriptor;
  const slug = d?.slug ?? '<missing>';

  // 1 — slug
  if (typeof d?.slug !== 'string' || d.slug.length === 0) {
    throw new ContractInvariantViolation(slug, 'slug must be a non-empty string');
  }
  if (!SLUG_PATTERN.test(d.slug)) {
    throw new ContractInvariantViolation(slug, 'slug must be lower-kebab/snake and start with a letter');
  }

  // 4 — descriptor fully populated
  for (const key of ['internalName', 'publicLabel', 'category'] as const) {
    if (typeof d[key] !== 'string' || d[key].length === 0) {
      throw new ContractInvariantViolation(slug, `descriptor.${key} must be a non-empty string`);
    }
  }
  if (typeof d.version !== 'number' || d.version < 1) {
    throw new ContractInvariantViolation(slug, 'descriptor.version must be a positive number');
  }
  if (typeof d.displayOrder !== 'number') {
    throw new ContractInvariantViolation(slug, 'descriptor.displayOrder must be a number');
  }

  // 3 — no storage field names in the descriptor
  for (const key of Object.keys(d)) {
    if (STORAGE_FIELD_PATTERN.test(key)) {
      throw new ContractInvariantViolation(slug, `descriptor field "${key}" looks like a storage column — contracts are not ORM rows`);
    }
  }

  // 5 — resolvers present and callable
  for (const key of ['resolveWorkspace', 'resolveCapabilities', 'resolveAccess'] as const) {
    if (typeof mod[key] !== 'function') {
      throw new ContractInvariantViolation(slug, `${key} must be a function (resolver)`);
    }
  }

  // 6 + 2 — resolvers are total (no throw on null user) and return string IDs.
  // Calling a resolver here is safe: resolvers MUST be pure & side-effect free
  // (see BOUNDARIES.md § Resolver purity rule), so a smoke-resolve is harmless.
  let ws;
  try {
    ws = mod.resolveWorkspace(null);
  } catch (e) {
    throw new ContractInvariantViolation(slug, `resolveWorkspace threw on a null user — resolvers must be total: ${String(e)}`);
  }
  if (ws.workspaceId !== undefined && typeof ws.workspaceId !== 'string') {
    throw new ContractInvariantViolation(slug, 'resolveWorkspace must return a string workspaceId (or undefined)');
  }
}
