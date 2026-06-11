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

export interface TelegramStatus {
  status: TelegramConnState;
  enabled: boolean;
  bot_username: string;
  bot_configured: boolean;
  webhook_last_at: string | null;
  webhook_last_err: string;
  bound_users: number;
  alert_recipients: number;
  actions_enabled: boolean;
  flags: TelegramFlags;
  channels: TelegramChannel[];
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
