import { useEffect } from 'react';
import { useAuthStore } from '../stores/authStore';
import { useRoleStore } from '../stores/roleStore';
import { getMe, refreshToken } from '../services/authService';

export function useAuth() {
  const { user, token, isLoading, login, logout, setUser } = useAuthStore();
  const { setRole } = useRoleStore();

  useEffect(() => {
    if (token && !user) {
      // apiFetch inside getMe() handles 401 → refresh → retry automatically.
      // If it still throws after refresh, the session is dead → force re-login.
      getMe()
        .then(async u => {
          if (u.org_id !== 0) {
            try {
              const next = await refreshToken();
              useAuthStore.getState().setToken(next);
            } catch {
              // Keep the current short-lived token; apiFetch will retry refresh
              // on the next 401 if the refresh cookie is unavailable here.
            }
          }
          setUser(u);
          setRole(u.role);
        })
        .catch(() => setUser(null));
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
