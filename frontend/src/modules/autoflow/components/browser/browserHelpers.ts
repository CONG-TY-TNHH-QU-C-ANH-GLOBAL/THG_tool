import type { SystemInfo } from '../../services/systemService';
import type { LocalConnector, LocalConnectorAction } from '../../types';
export function stateLabel(state?: string): string {
  switch (state) {
    case 'initializing': return 'Ä‘ang khá»Ÿi Ä‘á»™ng';
    case 'display_ready': return 'desktop ready';
    case 'ready': return 'ready';
    case 'idle': return 'idle';
    case 'active': return 'active';
    case 'checkpoint': return 'human required';
    case 'human_required': return 'human required';
    case 'local_starting': return 'Ä‘ang chá» Runtime';
    case 'local_active': return 'Chrome tháº­t Ä‘ang cháº¡y';
    case 'local_login_required': return 'cáº§n Ä‘Äƒng nháº­p Facebook';
    case 'local_human_required': return 'Facebook cáº§n xÃ¡c minh';
    case 'local_ready': return 'Facebook local ready';
    case 'local_error': return 'local error';
    case 'error': return 'error';
    default: return state || '';
  }
}

export function stateTone(state?: string) {
  if (state === 'error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'local_error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'checkpoint' || state === 'human_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'local_human_required' || state === 'local_login_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'initializing') return { color: '#fde68a', bg: '#78350f44', border: '#f59e0b55' };
  if (state === 'local_starting' || state === 'local_active' || state === 'local_ready') return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
  return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
}

export function formatLastSeen(value?: string) {
  if (!value) return 'not connected';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

export type DownloadKey = keyof SystemInfo['agent_builds'];

export const RUNTIME_DOWNLOADS: Array<{ key: DownloadKey; label: string; href: string }> = [
  { key: 'local_kit_windows', label: 'Windows', href: '/downloads/thg-local-kit-windows.zip' },
  { key: 'local_kit_mac_m1', label: 'macOS', href: '/downloads/thg-local-kit-mac-m1.zip' },
  { key: 'local_kit_linux', label: 'Linux', href: '/downloads/thg-local-kit-linux.zip' },
];

export function connectorCapabilities(connector: LocalConnector): Record<string, unknown> {
  try {
    const parsed = JSON.parse(connector.capabilitiesJson || '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

export function isDashboardStreamConnector(connector: LocalConnector): boolean {
  const caps = connectorCapabilities(connector);
  return connector.kind === 'desktop_connector' ||
    connector.transport === 'local_chrome' ||
    caps.native_companion === true ||
    caps.multi_profile === true;
}

export function isUsableConnectorForAccount(connector: LocalConnector, userId: number, accountId?: number | null): boolean {
  if (!connector.online || !isDashboardStreamConnector(connector)) return false;
  if (connector.createdBy === userId) return true;
  return Boolean(accountId && connector.assignedAccountId === accountId);
}

export function connectorStatusLabel(status?: string): string {
  switch ((status || '').toLowerCase()) {
    case 'pairing':
      return 'ÄÃ£ ghÃ©p thiáº¿t bá»‹';
    case 'online':
    case 'connector_online':
      return 'Sáºµn sÃ ng';
    case 'chrome_not_connected':
      return 'ChÆ°a káº¿t ná»‘i Chrome';
    case 'chrome_connected':
      return 'ÄÃ£ tháº¥y Chrome local';
    case 'facebook_login_required':
      return 'ChÆ°a Ä‘Äƒng nháº­p Facebook';
    case 'facebook_human_required':
      return 'Facebook cáº§n xÃ¡c minh';
    case 'facebook_logged_in':
      return 'ÄÃ£ káº¿t ná»‘i Facebook';
    case 'idle':
      return 'Äang chá» lá»‡nh';
    case 'running':
      return 'Äang cháº¡y';
    case 'error':
      return 'Cáº§n kiá»ƒm tra';
    default:
      return status || 'Äang chá» lá»‡nh';
  }
}

export function facebookIdentityLabel(identity: {
  displayName?: string;
  username?: string;
  email?: string;
  fbUserId?: string;
  fallback?: string;
}): string {
  const displayName = (identity.displayName || '').trim();
  if (displayName) return displayName;
  const username = (identity.username || '').trim().replace(/^@+/, '');
  if (username) return `@${username}`;
  const email = (identity.email || '').trim();
  if (email) return email;
  const fbUserId = (identity.fbUserId || '').trim();
  if (fbUserId) return `FB ${fbUserId}`;
  return (identity.fallback || '').trim();
}

export function actionTypeLabel(type?: string): string {
  switch ((type || '').toLowerCase()) {
    case 'crawl':
      return 'Crawl Facebook';
    case 'click':
      return 'Click';
    case 'scroll':
      return 'Scroll';
    case 'text':
      return 'Nháº­p text';
    case 'key':
      return 'PhÃ­m';
    default:
      return type || 'Action';
  }
}

export function actionStatusTone(status?: string) {
  switch ((status || '').toLowerCase()) {
    case 'pending':
      return { label: 'Ä‘ang chá» Runtime', color: '#fde68a', border: '#f59e0b55', bg: '#78350f33' };
    case 'claimed':
      return { label: 'Runtime Ä‘ang cháº¡y', color: '#67e8f9', border: '#06b6d455', bg: '#164e6333' };
    case 'done':
      return { label: 'xong', color: '#86efac', border: '#22c55e55', bg: '#14532d33' };
    case 'failed':
      return { label: 'lá»—i', color: '#fca5a5', border: '#ef444455', bg: '#7f1d1d33' };
    default:
      return { label: status || 'Ä‘ang xá»­ lÃ½', color: '#cbd5e1', border: '#47556966', bg: '#0f172a66' };
  }
}

export function actionTime(action: LocalConnectorAction): string {
  return action.completedAt || action.claimedAt || action.createdAt;
}

export function isRemoteControlKey(key: string): boolean {
  return [
    'Enter', 'Backspace', 'Tab', 'Escape', 'Delete',
    'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown',
    'Home', 'End', 'PageUp', 'PageDown',
  ].includes(key);
}

