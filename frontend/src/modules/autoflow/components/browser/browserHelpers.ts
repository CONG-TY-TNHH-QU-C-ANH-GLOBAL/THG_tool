import type { LocalConnector, LocalConnectorAction } from '../../types';

export function stateLabel(state?: string): string {
  switch (state) {
    case 'initializing': return 'đang khởi động';
    case 'display_ready': return 'Extension ready';
    case 'ready': return 'ready';
    case 'idle': return 'idle';
    case 'active': return 'active';
    case 'checkpoint': return 'human required';
    case 'human_required': return 'human required';
    case 'local_starting': return 'đang chờ Extension';
    case 'local_active': return 'Extension đang stream';
    case 'local_login_required': return 'cần đăng nhập Facebook';
    case 'local_human_required': return 'Facebook cần xác minh';
    case 'local_ready': return 'Facebook ready';
    case 'local_error': return 'cần kiểm tra';
    case 'error': return 'error';
    default: return state || '';
  }
}

export function stateTone(state?: string) {
  if (state === 'error' || state === 'local_error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'checkpoint' || state === 'human_required' || state === 'local_human_required' || state === 'local_login_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'initializing') return { color: '#fde68a', bg: '#78350f44', border: '#f59e0b55' };
  if (state === 'local_starting' || state === 'local_active' || state === 'local_ready') return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
  return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
}

export function formatLastSeen(value?: string) {
  if (!value) return 'chưa kết nối';
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
  return connector.kind === 'extension_connector' ||
    connector.transport === 'chrome_extension' ||
    caps.chrome_extension === true ||
    caps.extension_bridge === 'supported' ||
    caps.dashboard_stream === true ||
    caps.dom_metadata === true ||
    caps.screen_capture === 'active_facebook_tab_only';
}

export function isUsableConnectorForAccount(connector: LocalConnector, userId: number, accountId?: number | null): boolean {
  if (!connector.online || !isDashboardStreamConnector(connector)) return false;
  if (connector.createdBy === userId) return true;
  return Boolean(accountId && connector.assignedAccountId === accountId);
}

export function connectorStatusLabel(status?: string): string {
  switch ((status || '').toLowerCase()) {
    case 'pairing':
      return 'Đã ghép thiết bị';
    case 'online':
    case 'connector_online':
      return 'Sẵn sàng';
    case 'chrome_not_connected':
      return 'Chưa thấy tab Facebook';
    case 'chrome_connected':
      return 'Đã thấy Chrome';
    case 'facebook_login_required':
      return 'Cần đăng nhập Facebook';
    case 'facebook_human_required':
      return 'Facebook cần xác minh';
    case 'facebook_logged_in':
      return 'Đã kết nối Facebook';
    case 'idle':
      return 'Đang chờ lệnh';
    case 'running':
      return 'Đang chạy';
    case 'error':
      return 'Cần kiểm tra';
    default:
      return status || 'Đang chờ lệnh';
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
      return 'Nhập text';
    case 'key':
      return 'Phím';
    default:
      return type || 'Action';
  }
}

export function actionStatusTone(status?: string) {
  switch ((status || '').toLowerCase()) {
    case 'pending':
      return { label: 'đang chờ Extension', color: '#fde68a', border: '#f59e0b55', bg: '#78350f33' };
    case 'claimed':
      return { label: 'Extension đang chạy', color: '#67e8f9', border: '#06b6d455', bg: '#164e6333' };
    case 'done':
      return { label: 'xong', color: '#86efac', border: '#22c55e55', bg: '#14532d33' };
    case 'failed':
      return { label: 'lỗi', color: '#fca5a5', border: '#ef444455', bg: '#7f1d1d33' };
    default:
      return { label: status || 'đang xử lý', color: '#cbd5e1', border: '#47556966', bg: '#0f172a66' };
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
