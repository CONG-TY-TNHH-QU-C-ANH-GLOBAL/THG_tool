// Pure, self-contained logic for the Telegram integration UI (NO imports) so it is unit-testable
// with the project's transpile-and-assert .test.mjs harness. All business decisions (status tone,
// countdown, needs-attention reasons, role gating, channel/alert catalogs) live here; the React
// components stay render-only. Channel-neutral: the channel list is whatever the backend returns.

export type ConnState = 'not_connected' | 'connected' | 'needs_attention';
export type Tone = 'ok' | 'warn' | 'off';

// statusTone maps the backend connection state to a UI tone.
export function statusTone(status: ConnState): Tone {
  if (status === 'connected') return 'ok';
  if (status === 'needs_attention') return 'warn';
  return 'off';
}

// The canonical alert types the UI offers (must mirror the backend allow-list). Channel-neutral.
export const ALERT_TYPES = [
  'connector_offline',
  'gate1_failure_spike',
  'submitted_unverified_spike',
  'automation_paused',
  'account_needs_attention',
  'circuit_breaker_triggered',
] as const;

// Fallback channel filters when the backend has not (yet) returned its list.
export const DEFAULT_CHANNEL_FILTERS = ['all', 'facebook', 'taobao', '1688'] as const;

// sanitizeAlertTypes keeps only known alert types (defends the PUT payload).
export function sanitizeAlertTypes(types: string[]): string[] {
  const known = new Set<string>(ALERT_TYPES as readonly string[]);
  return (types || []).filter((t) => known.has(t));
}

// isValidChannelFilter — the PUT must carry a filter the backend accepts.
export function isValidChannelFilter(filter: string, available: string[]): boolean {
  const list = available && available.length ? available : (DEFAULT_CHANNEL_FILTERS as readonly string[]);
  return list.indexOf(filter) !== -1;
}

// secondsLeft / formatCountdown drive the one-time-code expiry countdown.
export function secondsLeft(expiresAtISO: string, nowMs: number): number {
  const exp = Date.parse(expiresAtISO);
  if (Number.isNaN(exp)) return 0;
  return Math.max(0, Math.floor((exp - nowMs) / 1000));
}
export function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${m}:${r < 10 ? '0' : ''}${r}`;
}
export function isCodeExpired(expiresAtISO: string, nowMs: number): boolean {
  return secondsLeft(expiresAtISO, nowMs) <= 0;
}

// needsAttentionReasons derives remediation keys from the status. Order = display priority.
export interface StatusLike {
  status: ConnState;
  enabled: boolean;
  bot_configured: boolean;
  webhook_last_err: string;
  bound_users: number;
  alert_recipients: number;
  flags: { TELEGRAM_BOT_ENABLED: boolean; TELEGRAM_NOTIFY_ENABLED: boolean };
}
export function needsAttentionReasons(s: StatusLike): string[] {
  const out: string[] = [];
  if (!s.flags.TELEGRAM_BOT_ENABLED) out.push('bot_disabled');
  if (!s.bot_configured) out.push('token_missing');
  if (s.webhook_last_err) out.push('webhook_error');
  if (s.enabled && s.bound_users === 0) out.push('no_bound_users');
  if (s.enabled && s.alert_recipients === 0) out.push('no_alert_recipients');
  if (!s.flags.TELEGRAM_NOTIFY_ENABLED) out.push('notify_disabled');
  return out;
}

// ── Role / ownership gating (server still enforces; this only drives what we render) ──

// canManageAllBindings — admins/platform owners see the full org bindings + audit panels.
export function canManageAllBindings(isAdmin: boolean): boolean {
  return isAdmin === true;
}
// canRevokeBinding — admins revoke any; a member only their own.
export function canRevokeBinding(isAdmin: boolean, bindingUserId: number, currentUserId: number): boolean {
  return isAdmin === true || bindingUserId === currentUserId;
}
// canTestNotification — needs notifications on AND at least one active binding.
export function canTestNotification(notifyEnabled: boolean, activeBindings: number): boolean {
  return notifyEnabled === true && activeBindings > 0;
}

// Telegram is control-plane only: this catalog is the EXHAUSTIVE set of control actions the UI may
// offer. There is intentionally no 'comment'/'post'/execution action — asserted by tests.
export const CONTROL_ACTIONS = [
  'enable', 'disable', 'generate_bind_code', 'revoke_binding', 'test_notification', 'update_alerts',
] as const;
export function actionsExecutionEnabled(): boolean {
  return false; // hard constant — Telegram cannot execute outbound actions in this product
}
