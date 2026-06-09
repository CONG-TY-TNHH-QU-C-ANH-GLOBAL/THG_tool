import { get, post } from './api';
import type { AccountReadiness } from '../components/accountHealth/types';

// PR-E: the board renders ONLY what this endpoint reports — it never re-derives
// readiness from old stream_status/online flags.
export async function getAccountReadiness(): Promise<AccountReadiness[]> {
  const res = await get<{ accounts: AccountReadiness[] }>('/accounts/readiness');
  return res.accounts ?? [];
}

// Admin override to lift a Verified-Actor block (P1b) so the account can run again.
export async function clearActorBlock(accountId: number): Promise<void> {
  await post(`/accounts/${accountId}/clear-actor-block`, {});
}
