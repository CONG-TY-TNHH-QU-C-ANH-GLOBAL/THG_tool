import { create } from 'zustand';
import * as authService from '../services/authService';
import type { Role, AuthUser } from '../services/authService';

interface AuthState {
  user: AuthUser | null;
  token: string | null;
  isLoading: boolean;
  login(email: string, password: string): Promise<void>;
  logout(): Promise<void>;
  refresh(): Promise<void>;
  setUser(user: AuthUser | null): void;
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

  async refresh() {
    try {
      const token = await authService.refreshToken();
      set({ token });
    } catch {
      set({ user: null, token: null });
    }
  },

  setUser(user) { set({ user }); },
}));

export type { Role, AuthUser };
