/**
 * Execution context service — the member's Default Account.
 *
 * This is the UI side of the deterministic ExecutionContext (PR4). When a
 * member owns ≥2 accounts and has not picked a default, outbound actions fail
 * with `execution_context_required`. Setting a default here resolves that:
 *   explicit account → default_account_id → (exactly 1 owned) → error.
 *
 * Per-user, per-org. The backend enforces that a member may only set a default
 * to an account they own (models.IsAccountOwnerAllowed).
 *
 * GET /api/execution-context  → { default_account_id }
 * PUT /api/execution-context  → { ok, default_account_id }
 */
import { get, put } from './api';

export async function getDefaultAccountId(): Promise<number> {
  const r = await get<{ default_account_id?: number }>('/execution-context');
  return r.default_account_id ?? 0;
}

export async function setDefaultAccountId(accountId: number): Promise<number> {
  const r = await put<{ ok?: boolean; default_account_id?: number }>(
    '/execution-context',
    { default_account_id: accountId },
  );
  return r.default_account_id ?? accountId;
}
