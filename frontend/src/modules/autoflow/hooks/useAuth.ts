import { useEffect } from 'react';
import { useAuthStore } from '../stores/authStore';
import { useRoleStore } from '../stores/roleStore';
import { initToken, getMe } from '../services/authService';

export function useAuth() {
  const { user, token, isLoading, login, logout, setUser } = useAuthStore();
  const { setRole } = useRoleStore();

  useEffect(() => {
    initToken();
    if (token && !user) {
      getMe()
        .then(u => {
          setUser(u);
          setRole(u.role);
        })
        .catch(() => {
          setUser(null);
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
