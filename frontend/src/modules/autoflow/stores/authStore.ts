import { create } from 'zustand';
import * as authService from '../services/authService';
import type { Role, AuthUser } from '../services/authService';

// Phase 4b: the access token lives in an HttpOnly cookie set by the
// server, not in localStorage. The store keeps a copy in memory for
// defence-in-depth (apiFetch attaches it as Authorization: Bearer
// alongside the cookie). On page reload the cookie restores the
// session via restoreSession(); the in-memory token is null until
// /auth/refresh fires and gives us a fresh JWT.
interface AuthState {
  user: AuthUser | null;
  token: string | null;
  isLoading: boolean;
  hydrated: boolean; // true after the boot-time restoreSession() resolves
  login(email: string, password: string): Promise<void>;
  logout(): Promise<void>;
  setToken(token: string | null): void;
  setUser(user: AuthUser | null): void;
  setAuth(token: string, user: AuthUser): void;
  hydrate(): Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  isLoading: false,
  hydrated: false,

  async login(email, password) {
    set({ isLoading: true });
    try {
      const { user, token } = await authService.login(email, password);
      set({ user, token, isLoading: false, hydrated: true });
    } catch (e) {
      set({ isLoading: false });
      throw e;
    }
  },

  async logout() {
    await authService.logout();
    set({ user: null, token: null, hydrated: true });
  },

  setToken(token) {
    set({ token });
  },

  setUser(user) { set({ user }); },

  setAuth(token, user) {
    set({ token, user, hydrated: true });
  },

  // hydrate is called once on app boot. It checks the SPA presence
  // cookie and, when present, asks the server for the current user.
  // If the cookie is stale, the call 401s and apiFetch will trigger
  // a refresh; if refresh also fails the user is treated as logged
  // out and the UI redirects to login.
  async hydrate() {
    const user = await authService.restoreSession();
    set({ user, hydrated: true });
  },
}));

export type { Role, AuthUser };
