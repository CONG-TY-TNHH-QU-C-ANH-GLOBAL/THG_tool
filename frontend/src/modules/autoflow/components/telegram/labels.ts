// Key -> human label maps for Telegram channel-first UI (vi primary; en humanizes the key when no
// explicit translation exists). Self-contained (no imports) — testable + tiny.
export type Lang = 'vi' | 'en';

const EVENT_VI: Record<string, string> = {
  lead_created: 'Lead mới', lead_assigned: 'Lead được giao', lead_ready_for_review: 'Lead sẵn sàng xử lý',
  comment_submitted: 'Comment đã gửi', comment_verified: 'Comment đã xác minh',
  comment_unverified: 'Comment chưa xác minh', comment_failed: 'Comment lỗi',
  post_submitted: 'Post đã gửi', post_failed: 'Post lỗi', inbox_sent: 'Inbox đã gửi', inbox_failed: 'Inbox lỗi',
  connector_offline: 'Connector offline', account_attention: 'Tài khoản cần chú ý',
  automation_paused: 'Tự động hoá tạm dừng', gate1_failure_spike: 'Tăng đột biến lỗi gate1',
  submitted_unverified_spike: 'Tăng đột biến chưa xác minh', circuit_breaker_triggered: 'Ngắt mạch kích hoạt',
};

const GROUP_VI: Record<string, string> = { lead: 'Sự kiện Lead', agent: 'Hành động Agent', system: 'Hệ thống / Cảnh báo' };
const GROUP_EN: Record<string, string> = { lead: 'Lead events', agent: 'Agent actions', system: 'System / Health' };
const TYPE_VI: Record<string, string> = { channel: 'Channel', group: 'Group', personal_dm: 'DM cá nhân' };

function humanize(key: string): string {
  return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

export function eventLabel(lang: Lang, key: string): string {
  return lang === 'vi' ? EVENT_VI[key] ?? humanize(key) : humanize(key);
}
export function groupLabel(lang: Lang, key: string): string {
  return (lang === 'vi' ? GROUP_VI : GROUP_EN)[key] ?? key;
}
export function destTypeLabel(lang: Lang, type: string): string {
  if (lang === 'en') return humanize(type);
  return TYPE_VI[type] ?? type;
}
