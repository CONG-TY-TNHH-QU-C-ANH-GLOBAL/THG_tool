import type { LocalConnector, LocalConnectorAction } from '../../types';

export function stateLabel(state?: string): string {
  switch (state) {
    case 'initializing': return 'đang khởi động';
    case 'display_ready': return 'browser ready';
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
  if (state === 'error' || state === 'local_error') return { color: 'var(--hot)', bg: 'var(--hot-bg)', border: 'color-mix(in oklch, var(--hot) 38%, transparent)' };
  if (state === 'checkpoint' || state === 'human_required' || state === 'local_human_required' || state === 'local_login_required') return { color: 'var(--warn)', bg: 'var(--warn-bg)', border: 'color-mix(in oklch, var(--warn) 38%, transparent)' };
  if (state === 'initializing') return { color: 'var(--warn)', bg: 'var(--warn-bg)', border: 'color-mix(in oklch, var(--warn) 34%, transparent)' };
  if (state === 'local_starting' || state === 'local_active' || state === 'local_ready') return { color: 'var(--ok)', bg: 'var(--ok-bg)', border: 'color-mix(in oklch, var(--ok) 34%, transparent)' };
  return { color: 'var(--ok)', bg: 'var(--ok-bg)', border: 'color-mix(in oklch, var(--ok) 34%, transparent)' };
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

// looksLikeJunkName rejects labels that are clearly NOT a person/account name —
// they are Facebook feed text or UI strings that the meta collector sometimes
// scrapes into fb_display_name / account name (e.g. "Unreadsports_saggy is a
// suggested Page for you to follow.1d", "Chỉnh sửa", "Facebook 05/06 15:56").
// A real name is short and free of these markers, so when a label trips this we
// fall through to a stable identifier (@username / FB <id>) instead of showing
// garbage — keeping the session list, connector cards and presence board labels
// consistent and trustworthy.
export function looksLikeJunkName(raw: string): boolean {
  const s = (raw || '').trim();
  if (!s) return true;
  if (s.length > 48) return true; // names are short; feed snippets are long
  const low = s.toLowerCase();
  const markers = [
    'suggested page', 'suggested for you', 'is a suggested', 'gợi ý', 'theo dõi',
    'follow', 'chỉnh sửa', 'edit profile', "what's on your mind", 'bạn đang nghĩ gì',
    'watch', 'reels', 'notification', 'thông báo', 'marketplace',
  ];
  if (markers.some((m) => low.includes(m))) return true;
  // Feed-item time suffix like "· 1d", ".1d", "2 h", "3 ngày"
  if (/[·.]\s*\d+\s*[dhmw]\b/.test(low) || /\b\d+\s*(ngày|giờ|phút|tuần)\b/.test(low)) return true;
  // Auto-generated placeholder "Facebook 05/06 15:56" / "Facebook 6156..."
  if (/^facebook\s+\d/.test(low)) return true;
  return false;
}

export function facebookIdentityLabel(identity: {
  displayName?: string;
  username?: string;
  email?: string;
  fbUserId?: string;
  fallback?: string;
}): string {
  const displayName = (identity.displayName || '').trim();
  if (displayName && !looksLikeJunkName(displayName)) return displayName;
  const username = (identity.username || '').trim().replace(/^@+/, '');
  if (username && !looksLikeJunkName(username)) return `@${username}`;
  const email = (identity.email || '').trim();
  if (email) return email;
  const fbUserId = (identity.fbUserId || '').trim();
  if (fbUserId) return `FB ${fbUserId}`;
  const fallback = (identity.fallback || '').trim();
  return looksLikeJunkName(fallback) ? '' : fallback;
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
      return { label: 'đang chờ Extension', color: 'var(--warn)', border: 'color-mix(in oklch, var(--warn) 34%, transparent)', bg: 'var(--warn-bg)' };
    case 'claimed':
      return { label: 'Extension đang chạy', color: 'var(--info)', border: 'color-mix(in oklch, var(--info) 34%, transparent)', bg: 'var(--info-bg)' };
    case 'done':
      return { label: 'xong', color: 'var(--ok)', border: 'color-mix(in oklch, var(--ok) 34%, transparent)', bg: 'var(--ok-bg)' };
    case 'failed':
      return { label: 'lỗi', color: 'var(--hot)', border: 'color-mix(in oklch, var(--hot) 34%, transparent)', bg: 'var(--hot-bg)' };
    default:
      return { label: status || 'đang xử lý', color: 'var(--text-mute)', border: 'var(--line-strong)', bg: 'var(--tag-mute-bg)' };
  }
}

export function actionTime(action: LocalConnectorAction): string {
  return action.completedAt || action.claimedAt || action.createdAt;
}
