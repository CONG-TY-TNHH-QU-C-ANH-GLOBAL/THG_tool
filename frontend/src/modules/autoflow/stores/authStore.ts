import { create } from 'zustand';
import * as authService from '../services/authService';
import type { Role, AuthUser } from '../services/authService';

const TOKEN_KEY = 'autoflow_token';

interface AuthState {
  user: AuthUser | null;
  token: string | null;
  isLoading: boolean;
  login(email: string, password: string): Promise<void>;
  logout(): Promise<void>;
  setToken(token: string | null): void;
  setUser(user: AuthUser | null): void;
  setAuth(token: string, user: AuthUser): void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: authService.getStoredToken(),
  isLoading: false,

  async login(email, password) {
    set({ isLoading: true });
    try {
      const { user, token } = await authService.login(email, password);
      set({ user, token, isLoading: false });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  async logout() {
    await authService.logout();
    set({ user: null, token: null });
  },

  setToken(token) {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
    set({ token });
  },

  setUser(user) { set({ user }); },

  setAuth(token, user) {
    localStorage.setItem(TOKEN_KEY, token);
    authService.initToken();
    set({ token, user });
  },
}));

export type { Role, AuthUser };
