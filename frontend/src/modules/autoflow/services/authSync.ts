/**
 * Auth sync: multi-tab BroadcastChannel (Layer 3) + WS re-auth hook (Layer 4).
 * Call initAuthSync() once at app startup. After that, token changes in any
 * tab propagate to all other tabs automatically, and WS connections stay
 * aligned with the current token.
 */

import { scheduleRefresh, cancelRefreshSchedule } from './api';
import { useAuthStore } from '../stores/authStore';

// ── Layer 3: Multi-tab sync via BroadcastChannel ─────────────────────────────

const bc = typeof BroadcastChannel !== 'undefined'
  ? new BroadcastChannel('auth')
  : null;

// Prevents re-broadcasting a message that we just received from another tab.
let isSyncingFromBC = false;

if (bc) {
  bc.onmessage = (e: MessageEvent) => {
    isSyncingFromBC = true;
    const store = useAuthStore.getState();
    if (e.data.type === 'TOKEN_UPDATED' && typeof e.data.token === 'string') {
      store.setToken(e.data.token);
    } else if (e.data.type === 'LOGOUT') {
      store.setToken(null);
      store.setUser(null);
    }
    isSyncingFromBC = false;
  };
}

// ── Layer 4: WebSocket re-auth ────────────────────────────────────────────────

type WSFactory = (token: string) => WebSocket;

const managedSockets: Array<{ factory: WSFactory; socket: WebSocket | null }> = [];

/**
 * Register a WebSocket that should be recreated whenever the token changes.
 * Pass a factory function that opens the connection with the given token.
 * Returns a cleanup function to unregister.
 */
export function managedWS(factory: WSFactory): () => void {
  const entry = { factory, socket: null as WebSocket | null };
  const token = useAuthStore.getState().token;
  if (token) entry.socket = factory(token);
  managedSockets.push(entry);
  return () => {
    entry.socket?.close();
    const idx = managedSockets.indexOf(entry);
    if (idx !== -1) managedSockets.splice(idx, 1);
  };
}

function reconnectAllSockets(token: string): void {
  for (const entry of managedSockets) {
    entry.socket?.close();
    entry.socket = entry.factory(token);
    entry.socket.onclose = (e) => {
      if (e.code === 4001) {
        // Server rejected token → trigger a refresh cycle
        import('./api').then(m => m.apiFetch('/auth/me').catch(() => {}));
      }
    };
  }
}

// ── Init ──────────────────────────────────────────────────────────────────────

export function initAuthSync(): void {
  let prevToken = useAuthStore.getState().token;

  // Schedule refresh for the token already in store (restored from localStorage).
  if (prevToken) scheduleRefresh(prevToken);

  useAuthStore.subscribe((state) => {
    const token = state.token;
    if (token === prevToken) return;
    prevToken = token;

    if (token) {
      scheduleRefresh(token);
      reconnectAllSockets(token);
      if (!isSyncingFromBC && bc) {
        bc.postMessage({ type: 'TOKEN_UPDATED', token });
      }
    } else {
      cancelRefreshSchedule();
      if (!isSyncingFromBC && bc) {
        bc.postMessage({ type: 'LOGOUT' });
      }
    }
  });
}
