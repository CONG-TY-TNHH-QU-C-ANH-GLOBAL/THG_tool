import { useEffect } from 'react';
import { useAuthStore } from '../stores/authStore';
import { useRoleStore } from '../stores/roleStore';

// Phase 4b: the access token is in an HttpOnly cookie. On mount we
// trigger authStore.hydrate() which checks the non-HttpOnly presence
// cookie and, if it's set, asks /auth/me who we are. Failure modes:
//  - no presence cookie  → hydrate sets user=null, screen=auth
//  - presence cookie but cookie expired → /auth/me 401s → apiFetch
//    triggers /auth/refresh; if that also fails the user is logged
//    out and we land on the auth screen
export function useAuth() {
  const { user, token, isLoading, hydrated, login, logout, hydrate } = useAuthStore();
  const { setRole } = useRoleStore();

  useEffect(() => {
    if (!hydrated) {
      void hydrate();
    }
  }, [hydrated, hydrate]);

  useEffect(() => {
    if (user) setRole(user.role);
  }, [user?.role]);

  async function handleLogin(email: string, password: string) {
    await login(email, password);
    const u = useAuthStore.getState().user;
    if (u) setRole(u.role);
  }

  return { user, token, isLoading, hydrated, login: handleLogin, logout };
}
