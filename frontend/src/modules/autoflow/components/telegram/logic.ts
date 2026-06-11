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
  'connect_channel', 'test_destination', 'update_preferences', 'disconnect_destination',
] as const;
export function actionsExecutionEnabled(): boolean {
  return false; // hard constant — Telegram cannot execute outbound actions in this product
}

// ── Channel destinations (channel-first model) ──

export type DestStatus = 'active' | 'disabled' | 'needs_attention';

// destinationTone maps a destination status to a UI tone.
export function destinationTone(status: DestStatus): Tone {
  if (status === 'active') return 'ok';
  if (status === 'needs_attention') return 'warn';
  return 'off';
}

// The full event-type catalog grouped for the preferences UI (mirrors the backend allow-list).
export const EVENT_GROUPS = [
  { key: 'lead', types: ['lead_created', 'lead_assigned', 'lead_ready_for_review'] },
  { key: 'agent', types: ['comment_submitted', 'comment_verified', 'comment_unverified', 'comment_failed', 'post_submitted', 'post_failed', 'inbox_sent', 'inbox_failed'] },
  { key: 'system', types: ['connector_offline', 'account_attention', 'automation_paused', 'gate1_failure_spike', 'submitted_unverified_spike', 'circuit_breaker_triggered'] },
] as const;

export const EVENT_TYPES = EVENT_GROUPS.flatMap((g) => g.types) as readonly string[];

// sanitizeEventTypes keeps only event types the backend accepts (defends the PUT payload).
export function sanitizeEventTypes(types: string[], available?: string[]): string[] {
  const known = new Set<string>(available && available.length ? available : EVENT_TYPES);
  return (types || []).filter((t) => known.has(t));
}

// destinationReasons derives remediation keys for a needs_attention destination + global gates.
export function destinationReasons(lastError: string, notifyEnabled: boolean, botConfigured: boolean): string[] {
  const out: string[] = [];
  if (!botConfigured) out.push('token_missing');
  if (!notifyEnabled) out.push('notify_disabled');
  if (lastError) out.push('delivery_failed');
  return out;
}

// canManageChannels — connecting/disconnecting/editing workspace channels is admin-only.
export function canManageChannels(isAdmin: boolean): boolean {
  return isAdmin === true;
}

// channelFirstStatus folds destination + binding counts into the headline state. A destination is
// the PRIMARY signal; bindings are secondary.
export function channelFirstStatus(
  enabled: boolean, botConfigured: boolean, activeDestinations: number, anyNeedsAttention: boolean,
): ConnState {
  if (activeDestinations === 0) return 'not_connected';
  if (!botConfigured || anyNeedsAttention) return 'needs_attention';
  return 'connected';
}

// ── Step 1: per-org bot credential ──

export type BotCredState = 'configured' | 'missing' | 'invalid' | 'revoked';

// botCredState derives the Step-1 state from the /bot response.
export function botCredState(bot: { bot_configured: boolean; status?: string } | null): BotCredState {
  if (!bot || (!bot.bot_configured && !bot.status)) return 'missing';
  if (bot.status === 'invalid') return 'invalid';
  if (bot.status === 'revoked') return 'revoked';
  return bot.bot_configured ? 'configured' : 'missing';
}

// botReady — channel connect/delivery requires a configured org bot.
export function botReady(bot: { bot_configured: boolean; status?: string } | null): boolean {
  return botCredState(bot) === 'configured';
}

// canConnectChannel — only an admin, and only once the org bot is configured.
export function canConnectChannel(isAdmin: boolean, bot: { bot_configured: boolean; status?: string } | null): boolean {
  return isAdmin === true && botReady(bot);
}

// publicDeliveryAvailable / privateChannelReady — public works once the bot is configured; private
// requires the per-workspace webhook, which is PENDING, so it must NOT be presented as ready.
export function publicDeliveryAvailable(bot: { bot_configured: boolean; status?: string } | null): boolean {
  return botReady(bot);
}
export const PRIVATE_CHANNEL_READY = false; // per-org webhook pending — keep private connect disabled

// webhookState — this org's bot webhook is not registered yet (per-workspace webhook pending).
export type WebhookState = 'pending' | 'configured' | 'not_configured';
export function webhookState(): WebhookState {
  return 'pending';
}
