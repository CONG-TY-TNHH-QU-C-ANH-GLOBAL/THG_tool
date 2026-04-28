import { useEffect } from 'react';
import { useAuthStore } from '../stores/authStore';
import { useRoleStore } from '../stores/roleStore';
import { initToken, getMe } from '../services/authService';

export function useAuth() {
  const { user, token, isLoading, login, logout, setUser, refresh } = useAuthStore();
  const { setRole } = useRoleStore();

  useEffect(() => {
    initToken();
    if (token && !user) {
      getMe()
        .then(u => {
          setUser(u);
          setRole(u.role);
        })
        .catch(async () => {
          // Access token expired (15-min TTL).
          // refresh() rotates the httpOnly refresh_token cookie (same-origin — sent
          // automatically), stores the new access token, and updates authStore.token.
          // Updating the store re-triggers this effect with the new token, which then
          // calls getMe() cleanly. No need to call getMe() here again.
          try {
            await refresh();
          } catch {
            // Refresh failed (cookie gone / token revoked). Force re-login.
            setUser(null);
          }
        });
    }
  }, [token]);

  useEffect(() => {
    if (user) setRole(user.role);
  }, [user?.role]);

  async function handleLogin(email: string, password: string) {
    await login(email, password);
    const u = useAuthStore.getState().user;
    if (u) setRole(u.role);
  }

  return { user, token, isLoading, login: handleLogin, logout };
}
