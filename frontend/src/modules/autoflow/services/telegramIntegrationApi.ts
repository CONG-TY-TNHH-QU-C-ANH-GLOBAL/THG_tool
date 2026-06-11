// Typed client for the Telegram integration control-plane (backend PR-1:
// specs/TELEGRAM_INTEGRATION_UI.md). Tenant scope + role come from the auth context server-side;
// the client never sends org/user ids. Read-only / control-plane only — there is deliberately NO
// action-execution method here (Telegram cannot post comments).
import * as api from './api';

const BASE = '/settings/integrations/telegram';

export type TelegramConnState = 'not_connected' | 'connected' | 'needs_attention';

export interface TelegramChannel {
  channel: string; // facebook | taobao | 1688 | ...
  label: string;
  active: boolean;
}

export interface TelegramFlags {
  TELEGRAM_BOT_ENABLED: boolean;
  TELEGRAM_NOTIFY_ENABLED: boolean;
  TELEGRAM_ACTIONS_ENABLED: boolean;
  bot_token_configured: boolean;
}

export type TelegramConnStateFull = TelegramConnState | 'disabled';

export interface TelegramStatus {
  status: TelegramConnState;
  enabled: boolean;
  bot_username: string;
  bot_configured: boolean;
  webhook_last_at: string | null;
  webhook_last_err: string;
  bound_users: number;
  alert_recipients: number;
  active_destinations: number;
  actions_enabled: boolean;
  flags: TelegramFlags;
  channels: TelegramChannel[];
}

// A notification destination — primarily a Telegram CHANNEL. chat_id is never sent by the backend.
export interface TelegramDestination {
  id: number;
  destination_type: 'channel' | 'group' | 'personal_dm';
  title: string;
  username: string;
  invite_link: string;
  status: 'active' | 'disabled' | 'needs_attention';
  event_types: string[];
  channel_filter: string;
  delivery_mode: string;
  connected_by_user_id: number;
  last_delivery_at: string | null;
  last_error: string;
  created_at: string;
}

export interface DestinationsResponse {
  destinations: TelegramDestination[];
  available_event_types: string[];
  available_filters: string[];
}

export interface ConnectCodeResponse {
  connect_code: string;
  instructions: string;
  ttl_seconds: number;
}

export interface TelegramBinding {
  id: number;
  user_id: number;
  telegram_user_id: number;
  telegram_username: string;
  display_name: string;
  role: string;
  alert_recipient: boolean;
  status: 'active' | 'revoked';
  bound_at: string;
  last_command_at: string | null;
}

export interface BindCodeResponse {
  code: string;
  expires_at: string;
  ttl_seconds: number;
  bot_username: string;
  deep_link: string;
}

export interface TelegramAlertPrefs {
  alerts_enabled: boolean;
  channel_filter: string;
  alert_types: string[];
  available_types: string[];
  available_filters: string[];
}

export interface TelegramAuditEvent {
  id: number;
  user_id: number;
  telegram_user_id: number;
  action: string;
  result: string;
  metadata: string;
  created_at: string;
}

export const getStatus = () => api.get<TelegramStatus>(`${BASE}/status`);
export const enableIntegration = () => api.post<TelegramStatus>(`${BASE}/enable`, {});
export const disableIntegration = () => api.post<TelegramStatus>(`${BASE}/disable`, {});

// ── Per-ORG bot credential (Step 1). The token is never returned — only safe fields. ──
export interface TelegramBotStatus {
  bot_configured: boolean;
  bot_username?: string;
  bot_display_name?: string;
  token_last4?: string;
  status?: 'active' | 'invalid' | 'revoked' | 'needs_attention';
  last_verified_at?: string | null;
  last_error?: string;
  // false = internal platform/runtime config issue (not the customer's Telegram setup). Admin-safe.
  platform_ready?: boolean;
}
export const getBot = () => api.get<TelegramBotStatus>(`${BASE}/bot`);
export const saveBot = (token: string) => api.post<TelegramBotStatus>(`${BASE}/bot`, { token });
export const verifyBot = () => api.post<TelegramBotStatus>(`${BASE}/bot/verify`, {});
export const deleteBot = () => api.del(`${BASE}/bot`);

// ── Notification destinations (PRIMARY: Telegram channels) ──
export const getDestinations = () => api.get<DestinationsResponse>(`${BASE}/destinations`);
export const connectPublicChannel = (username: string) =>
  api.post<{ destination: TelegramDestination }>(`${BASE}/destinations`, { type: 'public', username });
export const createPrivateChannelConnectCode = () =>
  api.post<ConnectCodeResponse>(`${BASE}/destinations`, { type: 'private' });
export const testDestination = (id: number) =>
  api.post<{ sent: boolean }>(`${BASE}/destinations/${id}/test`, {});
export const updateDestinationPreferences = (id: number, body: { event_types: string[]; channel_filter: string }) =>
  api.put<{ updated: boolean }>(`${BASE}/destinations/${id}/preferences`, body);
// Disconnect soft-disables; reconnecting (re-running setup) re-enables it.
export const disconnectDestination = (id: number) => api.del(`${BASE}/destinations/${id}`);

// ── Personal DM bindings (SECONDARY) ──
export const createBindCode = () => api.post<BindCodeResponse>(`${BASE}/bind-codes`, {});

export async function getBindings(): Promise<{ bindings: TelegramBinding[]; can_manage_all: boolean }> {
  return api.get(`${BASE}/bindings`);
}
export const revokeBinding = (id: number) => api.del(`${BASE}/bindings/${id}`);

export const sendTestNotification = () =>
  api.post<{ queued: boolean; note?: string }>(`${BASE}/test-notification`, {});

export const getAlerts = () => api.get<TelegramAlertPrefs>(`${BASE}/alerts`);
export const updateAlerts = (body: {
  alerts_enabled: boolean;
  channel_filter: string;
  alert_types: string[];
}) => api.put<TelegramAlertPrefs>(`${BASE}/alerts`, body);

export async function getAudit(limit = 100): Promise<TelegramAuditEvent[]> {
  const res = await api.get<{ events: TelegramAuditEvent[] }>(`${BASE}/audit?limit=${limit}`);
  return res.events ?? [];
}
