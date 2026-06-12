'use client';
/**
 * ONE accept-invite sequence, shared by every accept surface (bell
 * card, onboarding invite list, /join/<token> page) so no caller can
 * forget a step (PR-1):
 *
 *   accept → setAuth(fresh token) → hydrate /auth/me → remember toast
 *   → route to the invited workspace (no logout/login ever).
 */
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { acceptInviteToken, type AcceptInviteResult } from '../../services/staffService';
import { rememberJoinedWorkspace } from '../../services/membershipService';
import { useAuthStore, type AuthUser } from '../../stores/authStore';
import { facebookWorkspaceIdOf } from '../../service';

export function useAcceptInvite() {
  const router = useRouter();
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState('');

  async function accept(token: string): Promise<AcceptInviteResult | null> {
    setAccepting(true);
    setError('');
    try {
      const data = await acceptInviteToken(token);
      const store = useAuthStore.getState();
      store.setAuth(data.access_token, data.user as AuthUser);
      // Re-hydrate from /auth/me so every membership-derived state
      // (role, org, switcher) reflects the DB, not the cached login.
      await store.hydrate();
      rememberJoinedWorkspace(data.org_name || `Workspace #${data.org_id}`);
      const workspaceId = facebookWorkspaceIdOf(data.org_id);
      router.push(workspaceId ? `/services/facebook/workspaces/${workspaceId}` : '/services');
      return data;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'accept_failed');
      return null;
    } finally {
      setAccepting(false);
    }
  }

  return { accept, accepting, error };
}
